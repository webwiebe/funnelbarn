package api

// Canonical form of a response, and the masker that removes volatile values.
//
// A golden file holds:
//
//	{"status": 200, "headers": {"Cache-Control": ..., "Content-Type": ...}, "body": <json | string>}
//
// JSON bodies are re-encoded with sorted keys (map encoding) and indented, so
// field order and whitespace never produce a diff while field names, types and
// values do. Only values that genuinely differ between two identical runs are
// masked, each by an explicit rule below. Everything else is compared exactly:
// widening a mask hides regressions, so add a rule only for a value that is
// proven volatile and document why.

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"time"
)

// goldenHeaders are the response headers recorded in a golden file. The rest
// (Date, X-Request-Id, security headers, CORS) are either volatile or covered
// by their own tests.
var goldenHeaders = []string{"Content-Type", "Cache-Control"}

// Masked placeholders.
const (
	maskUUID      = "<uuid>"
	maskNow       = "<now>"
	maskRequestID = "<request-id>"
	maskSecret    = "<secret>"
	maskIngestID  = "<ingest-id>"
)

var (
	uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	// rfc3339Re finds timestamps embedded in text bodies (the setup guide).
	rfc3339Re = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
)

// goldenWindow is the wall-clock span of one suite run: from just before the
// seed started to the moment the response being masked was recorded. A
// timestamp inside it was produced by this run (a touch, an evaluation, a
// created row, time.Now() in a payload); one outside it, such as the seed's
// fixed March 2026 dates, is data and stays literal.
type goldenWindow struct {
	start, end time.Time
}

// goldenClockSlack covers the one-second resolution of the SQL clock.
const goldenClockSlack = time.Second

// contains reports whether t equals some now in [start,end] minus shift.
func (w goldenWindow) contains(t time.Time, shift time.Duration) bool {
	if w.start.IsZero() {
		return false
	}
	return !t.Before(w.start.Add(-shift-goldenClockSlack)) && !t.After(w.end.Add(-shift+goldenClockSlack))
}

// analysisShifts are the look-back distances of the flag analysis route:
// "from" is now minus one of these, "to" is now.
var analysisShifts = []time.Duration{0, 24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour}

func parseGoldenTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// goldenMasker documents and applies the volatile-value rules.
//
//  1. UUIDs that do not start with goldenIDPrefix. Every seeded row has a
//     fixed id; a UUID without the prefix was generated while serving the
//     request (a created flag, an auto-registered flag, an ingest id) and
//     differs per run.
//  2. request_id keys. Generated per request.
//  3. Timestamps inside the run window (goldenWindow). They hold the wall
//     clock of a touch, an evaluation, a created row or time.Now() in a
//     payload (last_used_at, last_evaluated_at, created_at of a row created
//     by a case, the health "time", project health updated_at). A timestamp
//     outside the window keeps its literal value, so a null stays null, the
//     seed's fixed dates stay compared, and a zero time "0001-01-01..." is
//     still distinguishable from a stamped one.
//  4. "from" on a flag analysis. That route has no from/to parameters and
//     reports "now minus range" (30 days by default, 24h, 7d or 30d with
//     ?range=), so it is masked when it falls on any of those look-backs.
//  5. RFC3339 timestamps inside text (non-JSON) bodies, same window rule, for
//     the setup guide's "generated at" line and its curl example.
//  6. "key" on the API key create response: the plaintext secret, random per
//     call.
//  7. "ingestId" on the POST /api/v1/events response: 12 random bytes in hex
//     (not a UUID), generated per accepted event.
//  8. Order of rows created during the run. Lists sort by created_at (the
//     recording flag timeline by evaluated_at, the same column), which
//     has one-second resolution, so rows created within the same second tie
//     and their relative order depends on whether a second boundary fell
//     between the inserts. Consecutive elements whose created_at was masked
//     as <now> are sorted by their JSON encoding; seeded rows (fixed,
//     distinct created_at) keep the order the server gave them.
//  9. Element order of session_ids (any route) and of links/nodes on the flows
//     route. Today's handlers build these from Go maps, so equal-weight
//     entries come out in a different order on every call (found by
//     TestGoldenDeterministic). The arrays are sorted by their JSON encoding,
//     so the set of entries is still compared exactly. If the code makes the
//     order deterministic, drop this rule and re-record.
//
// TestGoldenDeterministic backs these rules: it runs the whole suite twice a
// second apart and fails on any difference a rule missed.
type goldenMasker struct {
	path   string // request path, for path-scoped rules
	window goldenWindow
}

func (m goldenMasker) maskString(s string) string {
	return uuidRe.ReplaceAllStringFunc(s, func(u string) string {
		if strings.HasPrefix(strings.ToLower(u), goldenIDPrefix) {
			return u
		}
		return maskUUID
	})
}

// maskTimestamp returns <now> for a timestamp inside the run window.
func (m goldenMasker) maskTimestamp(s string, shifts ...time.Duration) (string, bool) {
	t, ok := parseGoldenTime(s)
	if !ok {
		return s, false
	}
	if len(shifts) == 0 {
		shifts = []time.Duration{0}
	}
	for _, sh := range shifts {
		if m.window.contains(t, sh) {
			return maskNow, true
		}
	}
	return s, false
}

// unordered reports whether the array under key has no stable element order (rule 9).
func (m goldenMasker) unordered(key string) bool {
	switch key {
	case "session_ids":
		return true
	case "links", "nodes":
		return strings.HasSuffix(m.path, "/flows")
	}
	return false
}

func sortByJSON(items []any) {
	keyed := make([]struct {
		k string
		v any
	}, len(items))
	for i, it := range items {
		b, _ := json.Marshal(it)
		keyed[i].k, keyed[i].v = string(b), it
	}
	sort.SliceStable(keyed, func(i, j int) bool { return keyed[i].k < keyed[j].k })
	for i := range keyed {
		items[i] = keyed[i].v
	}
}

// sortCreatedRuns sorts each run of consecutive objects whose created_at (or
// evaluated_at, the recording flag timeline's sort key) was masked as <now>
// (rule 8).
func sortCreatedRuns(items []any) {
	isNew := func(v any) bool {
		m, ok := v.(map[string]any)
		return ok && (m["created_at"] == maskNow || m["evaluated_at"] == maskNow)
	}
	for i := 0; i < len(items); {
		if !isNew(items[i]) {
			i++
			continue
		}
		j := i
		for j < len(items) && isNew(items[j]) {
			j++
		}
		sortByJSON(items[i:j])
		i = j
	}
}

func (m goldenMasker) maskValue(key string, v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			x[k] = m.maskValue(k, child)
		}
		return x
	case []any:
		for i, child := range x {
			x[i] = m.maskValue("", child)
		}
		if m.unordered(key) {
			sortByJSON(x)
		}
		sortCreatedRuns(x)
		return x
	case string:
		if key == "request_id" {
			return maskRequestID
		}
		if key == "ingestId" && m.path == "/api/v1/events" {
			return maskIngestID
		}
		if key == "key" && m.path == "/api/v1/apikeys" {
			return maskSecret
		}
		if key == "from" && strings.HasSuffix(m.path, "/analysis") && strings.Contains(m.path, "/flags/") {
			if masked, ok := m.maskTimestamp(x, analysisShifts...); ok {
				return masked
			}
		}
		if masked, ok := m.maskTimestamp(x); ok {
			return masked
		}
		return m.maskString(x)
	}
	return v
}

// maskText masks a text body and splits it into lines, so a change to a long
// document shows up as a line diff instead of one unreadable string.
func (m goldenMasker) maskText(s string) []string {
	s = rfc3339Re.ReplaceAllStringFunc(m.maskString(s), func(ts string) string {
		if masked, ok := m.maskTimestamp(ts); ok {
			return masked
		}
		return ts
	})
	return strings.Split(s, "\n")
}

// goldenCanonicalize renders a recorded response as the golden file content.
func goldenCanonicalize(rec *httptest.ResponseRecorder, path string, window goldenWindow) ([]byte, error) {
	m := goldenMasker{path: path, window: window}
	headers := map[string]string{}
	for _, h := range goldenHeaders {
		if v := rec.Header().Get(h); v != "" {
			headers[h] = v
		}
	}
	out := map[string]any{"status": rec.Code, "headers": headers}

	raw := rec.Body.Bytes()
	mt, _, _ := mime.ParseMediaType(rec.Header().Get("Content-Type"))
	switch {
	case len(bytes.TrimSpace(raw)) == 0:
		out["body"] = []string{}
	case mt == "application/json" || json.Valid(raw):
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber() // keep 1.0 and 1 distinct, never round large ints
		var body any
		if err := dec.Decode(&body); err != nil {
			out["body"] = m.maskText(string(raw))
			break
		}
		out["body"] = m.maskValue("", body)
	default:
		out["body"] = m.maskText(string(raw))
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
