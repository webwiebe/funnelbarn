package api

// Top-level golden tests. See golden_harness_test.go for the overview.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

const goldenDir = "testdata/golden"

// goldenAllCases returns every case in execution order.
func goldenAllCases(s *goldenSeed) []goldenCase {
	cs := append(goldenGETCases(s), goldenPOSTCases(s)...)
	cs = append(cs, goldenMutationCases(s)...)
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Phase < cs[j].Phase })
	return goldenAnnotate(cs, s)
}

func goldenPath(name string) string { return filepath.Join(goldenDir, name+".golden.json") }

// goldenExpand substitutes $var placeholders. Longer names go first so $keyID
// is never clobbered by $key.
func goldenExpand(s string, vars map[string]string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	names := make([]string, 0, len(vars))
	for n := range vars {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		s = strings.ReplaceAll(s, "$"+n, vars[n])
	}
	return s
}

// goldenDo sends one case to the handler with the credentials real callers use.
func goldenDo(t *testing.T, tgt goldenTarget, c goldenCase, vars map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Reader
	if c.Body != nil {
		b, err := json.Marshal(c.Body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		body = bytes.NewReader([]byte(goldenExpand(string(b), vars)))
	} else {
		body = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(c.Method, goldenExpand(c.Path, vars), body)
	if c.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	switch c.Auth {
	case authSession:
		req.AddCookie(tgt.Session)
		if c.Method != http.MethodGet {
			req.Header.Set("X-FunnelBarn-CSRF", tgt.CSRF)
		}
	case authSpare:
		req.AddCookie(tgt.Spare)
		if c.Method != http.MethodGet {
			req.Header.Set("X-FunnelBarn-CSRF", tgt.SpareCSRF)
		}
	case authKey:
		req.Header.Set(auth.HeaderAPIKey, goldenExpand(c.Key, vars))
	}
	rec := httptest.NewRecorder()
	tgt.Handler.ServeHTTP(rec, req)
	return rec
}

// goldenSave stores the response values a case asks to keep.
func goldenSave(t *testing.T, c goldenCase, rec *httptest.ResponseRecorder, vars map[string]string) {
	t.Helper()
	for name, path := range c.Save {
		var cur any
		if err := json.Unmarshal(rec.Body.Bytes(), &cur); err != nil {
			t.Fatalf("%s: save %s: response is not JSON: %v", c.Name, name, err)
		}
		for _, part := range strings.Split(path, ".") {
			m, ok := cur.(map[string]any)
			if !ok {
				t.Fatalf("%s: save %s: %q is not an object path in %s", c.Name, name, path, rec.Body.String())
			}
			cur = m[part]
		}
		v, ok := cur.(string)
		if !ok || v == "" {
			t.Fatalf("%s: save %s: %q is not a non-empty string in %s", c.Name, name, path, rec.Body.String())
		}
		vars[name] = v
	}
}

// goldenResult is one case's canonical response.
type goldenResult struct {
	Case   goldenCase
	Status int
	Golden []byte
}

// goldenCollect seeds a fresh environment, runs every case against the server
// ctor builds, and returns the canonical responses in execution order. Each
// case's status is checked against WantStatus here, so it holds with and
// without -update.
func goldenCollect(t *testing.T, ctor goldenConstructor) []goldenResult {
	t.Helper()
	start := time.Now()
	tgt, seed, deps := newGoldenEnv(t, ctor)
	evalBase, err := countEvaluationRows(deps.Store.DB())
	if err != nil {
		t.Fatalf("count seeded flag_evaluations: %v", err)
	}
	vars := map[string]string{}
	var out []goldenResult
	for _, c := range goldenAllCases(seed) {
		if c.Await != "" {
			waitGoldenAsync(t, tgt, deps.Store, seed, goldenWait{
				What: c.Await, Flags: c.AwaitFlags, Keys: c.wantKeys, SetupA: c.wantSetupA, EvalBase: evalBase,
			})
		}
		rec := goldenDo(t, tgt, c, vars)
		if rec.Code != c.wantStatus() {
			t.Fatalf("%s: %s %s returned %d, want %d\n%s", c.Name, c.Method, goldenExpand(c.Path, vars), rec.Code, c.wantStatus(), rec.Body.String())
		}
		goldenSave(t, c, rec, vars)
		path := strings.SplitN(goldenExpand(c.Path, vars), "?", 2)[0]
		got, err := goldenCanonicalize(rec, path, goldenWindow{start: start, end: time.Now()})
		if err != nil {
			t.Fatalf("%s: canonicalize: %v", c.Name, err)
		}
		out = append(out, goldenResult{Case: c, Status: rec.Code, Golden: got})
	}
	return out
}

// runGoldenCases runs every case against a server built by ctor and compares
// (or, with -update, rewrites) the golden files. Phase 2 of the CQRS work calls
// this with its own constructor to prove the new setup answers identically.
func runGoldenCases(t *testing.T, ctor goldenConstructor) {
	t.Helper()
	results := goldenCollect(t, ctor)

	ok2xx := 0
	var non2xx []string
	for _, r := range results {
		c := r.Case
		if r.Status >= 200 && r.Status < 300 {
			ok2xx++
		} else {
			non2xx = append(non2xx, c.Name+" -> "+http.StatusText(r.Status))
		}
		file := goldenPath(c.Name)
		if *update {
			if err := os.MkdirAll(goldenDir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(file, r.Golden, 0o644); err != nil {
				t.Errorf("%s: write: %v", c.Name, err)
			}
			continue
		}
		want, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("%s: missing golden file %s (run with -update): %v", c.Name, file, err)
			continue
		}
		if !bytes.Equal(want, r.Golden) {
			t.Errorf("%s: %s %s differs from %s\n--- want\n%s\n--- got\n%s", c.Name, c.Method, c.Path, file, want, r.Golden)
		}
	}

	// Every case already matched its WantStatus in goldenCollect. This ratio is
	// an extra guard: a suite full of 401/404 bodies proves nothing, so most
	// cases must be successes.
	total := len(results)
	if total == 0 || ok2xx*100 < total*90 {
		t.Errorf("only %d of %d golden cases returned 2xx (need 90%%); non-2xx: %v", ok2xx, total, non2xx)
	}
	for _, n := range non2xx {
		t.Logf("non-2xx golden case: %s", n)
	}
	t.Logf("golden cases: %d total, %d 2xx", total, ok2xx)
}

// TestGoldenDeterministic runs the whole suite twice, more than a second apart
// (the SQL clock has one-second resolution), and fails on any difference. It
// proves the seed and the masker leave nothing time- or random-dependent in the
// golden files, which a single run compared with a file written seconds earlier
// can miss.
func TestGoldenDeterministic(t *testing.T) {
	first := goldenCollect(t, defaultGoldenServer)
	time.Sleep(1100 * time.Millisecond)
	second := goldenCollect(t, defaultGoldenServer)
	if len(first) != len(second) {
		t.Fatalf("case count changed between runs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if !bytes.Equal(first[i].Golden, second[i].Golden) {
			t.Errorf("%s is not deterministic between two runs\n--- first\n%s\n--- second\n%s", first[i].Case.Name, first[i].Golden, second[i].Golden)
		}
	}
}

// TestGoldenAPI is the regression check: every case against today's server.
func TestGoldenAPI(t *testing.T) {
	runGoldenCases(t, defaultGoldenServer)
	if *update {
		goldenRemoveOrphans(t)
	}
}

// goldenRemoveOrphans deletes golden files no case refers to any more.
func goldenRemoveOrphans(t *testing.T) {
	t.Helper()
	want := map[string]bool{}
	for _, c := range goldenAllCases(goldenPlaceholderSeed()) {
		want[goldenPath(c.Name)] = true
	}
	files, _ := filepath.Glob(filepath.Join(goldenDir, "*.golden.json"))
	for _, f := range files {
		if !want[f] {
			if err := os.Remove(f); err != nil {
				t.Errorf("remove orphan %s: %v", f, err)
			}
		}
	}
}

// TestGoldenCaseNamesUnique guards the one-file-per-case layout.
func TestGoldenCaseNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range goldenAllCases(goldenPlaceholderSeed()) {
		if seen[c.Name] {
			t.Errorf("duplicate golden case name %q", c.Name)
		}
		seen[c.Name] = true
	}
}

// TestGoldenNoOrphanFiles fails when a golden file has no case, which would
// mean a case was removed or renamed and its file silently stopped guarding
// anything.
func TestGoldenNoOrphanFiles(t *testing.T) {
	want := map[string]bool{}
	for _, c := range goldenAllCases(goldenPlaceholderSeed()) {
		want[goldenPath(c.Name)] = true
	}
	files, err := filepath.Glob(filepath.Join(goldenDir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("golden file %s has no case (run -update to prune)", f)
		}
	}
}
