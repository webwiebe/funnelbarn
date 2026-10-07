package api

// State-snapshot regression harness (issue #302, spec 012).
//
// TestStateSnapshot* drives a scripted scenario through the full stack and
// compares a canonical dump of the bookkeeping tables against golden files in
// testdata/statesnap. Later PRs of the CQRS split must leave the goldens
// untouched: the same requests have to produce the same rows.
//
// Regenerate with: go test ./internal/api -run TestStateSnapshot -update-state

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

var updateState = flag.Bool("update-state", false, "rewrite internal/api/testdata/statesnap/*.golden")

const (
	snapDir      = "testdata/statesnap"
	maskedValue  = "<masked>"
	setValue     = "<set>"
	unsetValue   = "<unset>"
	asyncTimeout = 10 * time.Second

	// End state of the scenario: distinct flag_evaluations rows and feature
	// flag rows (4 seeded + snapAutoMax auto-registered).
	snapWantEvaluations = 5
	snapWantFlags       = 4 + snapAutoMax
	// api_keys rows with last_used_at set: only the SDK key is used.
	snapWantKeysUsed = 1
)

// snapStacks maps a store to the snapStack that owns it, so flushAsync keeps
// its call-site signature while still reaching snapStack.Flush.
var snapStacks sync.Map // *repository.Store -> *snapStack

// flushAsync blocks until the writes that the evaluate path performs off the
// request goroutine have landed: FlagService.touchEvaluated (bblog.Go), the
// MarkFlagsEvaluated goroutine in handleEvaluateFlag, the TouchAPIKey call in
// API key auth, and the OnEventsReceived goroutine in the ingest handler.
//
// It calls snapStack.Flush when set (PR 2: the async command dispatcher), then
// polls the database for the expected end state with a deadline, so it waits
// for the real transition instead of sleeping a fixed time. The signature and
// every call site stay as they are.
func flushAsync(t *testing.T, store *repository.Store, projectID string, evaluatedFlagKeys []string) {
	t.Helper()
	if v, ok := snapStacks.Load(store); ok {
		if err := v.(*snapStack).flush(context.Background()); err != nil {
			t.Fatalf("flushAsync: Flush: %v", err)
		}
	}
	db := store.DB()
	deadline := time.Now().Add(asyncTimeout)
	for {
		ok, state := asyncStateLanded(db, projectID, len(evaluatedFlagKeys))
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("flushAsync: async writes did not land within %s (%s)\n%s",
				asyncTimeout, state, dumpSnapshotTables(t, db))
		}
		time.Sleep(2 * time.Millisecond) // poll interval only; the deadline is the bound
	}
}

// dumpSnapshotTables renders every snapshot table, so a timeout shows which
// rows are present and which are missing.
func dumpSnapshotTables(t *testing.T, db *sql.DB) string {
	t.Helper()
	labels := idLabels(t, db)
	var b strings.Builder
	for _, spec := range snapTables {
		fmt.Fprintf(&b, "== %s\n%s", spec.table, dumpTable(t, db, spec, labels))
	}
	return b.String()
}

func asyncStateLanded(db *sql.DB, projectID string, wantTouched int) (bool, string) {
	var touched, flagsEval, eventsRecv, evals, flagRows, keysUsed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM feature_flags WHERE project_id = ? AND last_evaluated_at IS NOT NULL`, projectID).Scan(&touched); err != nil {
		return false, "touched query: " + err.Error()
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM flag_evaluations WHERE project_id = ?`, projectID).Scan(&evals); err != nil {
		return false, "evaluations query: " + err.Error()
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM feature_flags WHERE project_id = ?`, projectID).Scan(&flagRows); err != nil {
		return false, "flags query: " + err.Error()
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE project_id = ? AND last_used_at IS NOT NULL`, projectID).Scan(&keysUsed); err != nil {
		return false, "api keys query: " + err.Error()
	}
	err := db.QueryRow(`SELECT flags_evaluated, events_received FROM project_health WHERE project_id = ?`, projectID).Scan(&flagsEval, &eventsRecv)
	if err != nil && err != sql.ErrNoRows {
		return false, "health query: " + err.Error()
	}
	state := fmt.Sprintf("touched=%d/%d evaluations=%d/%d flags=%d/%d flags_evaluated=%d events_received=%d keys_used=%d/%d",
		touched, wantTouched, evals, snapWantEvaluations, flagRows, snapWantFlags, flagsEval, eventsRecv, keysUsed, snapWantKeysUsed)
	// >= so an extra touch or row shows up as a golden diff, not a timeout.
	return touched >= wantTouched && evals >= snapWantEvaluations && flagRows >= snapWantFlags &&
		flagsEval == 1 && eventsRecv == 1 && keysUsed >= snapWantKeysUsed, state
}

// tableSpec says how one table is canonicalised.
type tableSpec struct {
	table string
	// drop lists columns left out entirely (random ids, wall-clock stamps).
	drop []string
	// mask lists columns replaced by a constant (value is random but present).
	mask []string
	// presence lists columns reduced to <set>/<unset>.
	presence []string
	// sortJSONArray lists text columns holding a JSON string array whose order
	// is not deterministic in production (map iteration); they are sorted.
	sortJSONArray []string
}

// sortedJSONArray re-marshals a JSON string array with its elements sorted.
// Values that are not a JSON string array are returned unchanged.
func sortedJSONArray(v any) any {
	str, ok := v.(string)
	if !ok {
		return v
	}
	var arr []string
	if err := json.Unmarshal([]byte(str), &arr); err != nil {
		return v
	}
	sort.Strings(arr)
	return marshalSorted(arr)
}

var snapTables = []tableSpec{
	{table: "flag_evaluations", drop: []string{"id", "created_at"}, sortJSONArray: []string{"context_keys"}},
	{table: "feature_flags", drop: []string{"id", "created_at"}, presence: []string{"last_evaluated_at"}},
	{table: "api_keys", drop: []string{"id", "key_hash", "created_at"}, presence: []string{"last_used_at"}},
	{table: "project_health", drop: []string{"updated_at"}},
	{table: "events", drop: []string{"created_at"}, mask: []string{"id", "ingest_id"}},
	{table: "sessions", drop: []string{"created_at", "updated_at"}},
}

func contains(set []string, c string) bool {
	for _, s := range set {
		if s == c {
			return true
		}
	}
	return false
}

// dumpTable renders one table as sorted, compact JSON lines (one row each).
// labels maps known random ids (project, flag) to stable names.
func dumpTable(t *testing.T, db *sql.DB, spec tableSpec, labels map[string]string) string {
	t.Helper()
	rows, err := db.Query("SELECT * FROM " + spec.table)
	if err != nil {
		t.Fatalf("dump %s: %v", spec.table, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("dump %s columns: %v", spec.table, err)
	}
	var lines []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("dump %s scan: %v", spec.table, err)
		}
		row := map[string]any{}
		for i, c := range cols {
			v := normalise(vals[i], labels)
			switch {
			case contains(spec.drop, c):
				continue
			case contains(spec.mask, c):
				v = maskedValue
			case contains(spec.sortJSONArray, c):
				v = sortedJSONArray(v)
			case contains(spec.presence, c):
				v = unsetValue
				if vals[i] != nil {
					v = setValue
				}
			}
			row[c] = v
		}
		lines = append(lines, marshalSorted(row))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("dump %s rows: %v", spec.table, err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// marshalSorted renders v as compact JSON with sorted map keys and no HTML
// escaping, so the goldens stay readable.
func marshalSorted(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSuffix(buf.String(), "\n")
}

func normalise(v any, labels map[string]string) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return normalise(string(x), labels)
	case string:
		if l, ok := labels[x]; ok {
			return "<" + l + ">"
		}
		return x
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	default:
		return x
	}
}

// idLabels maps every project and flag id in the database to a stable label.
func idLabels(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	labels := map[string]string{}
	collect := func(q, prefix string) {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatalf("labels %q: %v", q, err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				t.Fatalf("labels scan: %v", err)
			}
			labels[id] = prefix + name
		}
	}
	collect(`SELECT id, slug FROM projects`, "project:")
	// Events without a client session id get a fingerprint that changes per
	// process run; only the fixed ids chosen by the scenario stay literal.
	collect(`SELECT DISTINCT id, 'fingerprint' FROM sessions WHERE id NOT IN ('`+snapSession1+`','`+snapSession2+`')`, "session:")
	collect(`SELECT f.id, p.slug || '/' || f.flag_key FROM feature_flags f JOIN projects p ON p.id = f.project_id`, "flag:")
	return labels
}

// compareGolden checks got against testdata/statesnap/<name>.golden, or
// rewrites the file under -update-state.
func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(snapDir, name+".golden")
	if *updateState {
		if err := os.MkdirAll(snapDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update-state to create): %v", path, err)
	}
	if string(want) != got {
		t.Errorf("state snapshot %s differs from %s\n--- want\n%s--- got\n%s", name, path, want, got)
	}
}

// snapshotAll dumps and compares every table in snapTables.
func snapshotAll(t *testing.T, store *repository.Store) {
	t.Helper()
	db := store.DB()
	labels := idLabels(t, db)
	for _, spec := range snapTables {
		compareGolden(t, spec.table, dumpTable(t, db, spec, labels))
	}
}
