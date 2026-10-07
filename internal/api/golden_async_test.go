package api

// Deterministic waits for the writes the server performs off the request
// goroutine. A case that reads what such a write produces names the wait in
// goldenCase.Await; nothing here sleeps for a fixed time.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// Names of the awaits a case can request.
const (
	// awaitEvaluate: every flag the SDK evaluate cases touched has its
	// last_evaluated_at, both projects have flags_evaluated set, and the
	// evaluation rows are all recorded.
	awaitEvaluate = "evaluate"
	// awaitIngest: the ingest and recording-chunk requests have flipped
	// events_received and recordings_received on project A.
	awaitIngest = "ingest"
	// awaitAPIKeys: every API key the cases used so far has last_used_at set.
	// Every await asserts this; the name is for cases that wait on nothing else.
	awaitAPIKeys = "apikeys"
	// awaitSetup: the setup guide request has flipped setup_called on project A.
	// Every await asserts this once the setup guide case has run.
	awaitSetup = "setup"
	// awaitAutoFlag: the flags named in goldenCase.AwaitFlags exist. It follows
	// each auto-registering evaluate case on purpose: the auto-registration cap
	// counts auto flags, so the next registering case must see the previous
	// row for the cap check to run against deterministic state.
	awaitAutoFlag = "autoflag"
)

// goldenAsyncTimeout bounds every wait. It is a failure deadline, not a delay:
// each wait returns the moment its condition holds.
const goldenAsyncTimeout = 10 * time.Second

// Number of flags the evaluate cases touch (SDK path, existing and
// auto-registered): project A checkout_redesign, pricing_banner, max_items,
// legacy_search, sdk_discovered, brand_new_flag, new_limit; project B
// beta_banner and the auto-registered checkout_redesign.
const (
	goldenTouchedA = 7
	goldenTouchedB = 2
)

// goldenRecordedEvaluations is how many flag_evaluations rows the evaluate
// cases add on top of the seed: active, non-config flags resolved by split or
// targeting. SDK: checkout_redesign NL and DE, pricing_banner match and no
// match, project B beta_banner. Playground: checkout_redesign and
// pricing_banner.
const goldenRecordedEvaluations = 7

// goldenFlagRef names one flag row by project and key.
type goldenFlagRef struct{ Project, Key string }

// goldenWait is everything one await point asserts. The fields beyond What are
// checked for every await, so an await never passes on less than the cases
// before it produced.
type goldenWait struct {
	What  string
	Flags []goldenFlagRef
	// Keys is how many api_keys rows must have last_used_at set.
	Keys int
	// SetupA requires project_health.setup_called=1 for project A.
	SetupA bool
	// EvalBase is the flag_evaluations row count before any case ran.
	EvalBase int
}

// goldenAnnotate fills the derived expectations of each case, in execution
// order: how many API keys the cases before it used, and whether the setup
// guide has been requested. Call it on the sorted case list.
func goldenAnnotate(cs []goldenCase, s *goldenSeed) []goldenCase {
	seeded := map[string]bool{s.KeyAFull: true, s.KeyAIngest: true, s.KeyAAnalytics: true, s.KeyAFlags: true, s.KeyBFull: true}
	used := map[string]bool{}
	setup := false
	for i := range cs {
		cs[i].wantKeys, cs[i].wantSetupA = len(used), setup
		c := cs[i]
		if c.Auth == authKey && (seeded[c.Key] || (c.Key == "$newKey" && c.WantStatus != 401)) {
			used[c.Key] = true
		}
		switch c.Name {
		case "apikey_delete":
			delete(used, "$newKey") // the row is gone, so it no longer counts
		case "setup_guide":
			setup = true
		case "project_health_a_reset":
			setup = false
		}
	}
	return cs
}

// waitGoldenAsync blocks until the async writes of one await point have
// landed.
//
// When tgt.Flush is set (a server whose bookkeeping runs behind a command
// dispatcher) it is called first, so queued commands are applied. The database
// conditions are then polled regardless, so what the next case reads is always
// asserted, with a deadline and no fixed sleeps. The default server leaves
// Flush nil: its writes are goroutines and polling alone covers them.
func waitGoldenAsync(t *testing.T, tgt goldenTarget, store *repository.Store, seed *goldenSeed, w goldenWait) {
	t.Helper()
	var check func(db *sql.DB) (bool, string)
	switch w.What {
	case awaitEvaluate:
		check = func(db *sql.DB) (bool, string) { return evaluateLanded(db, seed, w.EvalBase+goldenRecordedEvaluations) }
	case awaitIngest:
		check = func(db *sql.DB) (bool, string) { return ingestLanded(db, seed) }
	case awaitAPIKeys, awaitSetup, awaitAutoFlag:
		check = func(*sql.DB) (bool, string) { return true, "" }
	default:
		t.Fatalf("waitGoldenAsync: unknown await %q", w.What)
	}
	if tgt.Flush != nil {
		ctx, cancel := context.WithTimeout(context.Background(), goldenAsyncTimeout)
		err := tgt.Flush(ctx)
		cancel()
		if err != nil {
			t.Fatalf("waitGoldenAsync(%s): Flush: %v", w.What, err)
		}
	}
	deadline := time.Now().Add(goldenAsyncTimeout)
	for {
		ok, state := check(store.DB())
		if ok {
			ok, state = commonLanded(store.DB(), seed, w)
		}
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waitGoldenAsync(%s): async writes did not land within %s (%s)\n%s", w.What, goldenAsyncTimeout, state, dumpGoldenState(store.DB()))
		}
		time.Sleep(2 * time.Millisecond) // poll interval only; the deadline is the bound
	}
}

// commonLanded checks the conditions every await carries.
func commonLanded(db *sql.DB, seed *goldenSeed, w goldenWait) (bool, string) {
	var keys int
	if err := db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE last_used_at IS NOT NULL`).Scan(&keys); err != nil {
		return false, "api_keys: " + err.Error()
	}
	if keys != w.Keys {
		return false, fmt.Sprintf("api_keys with last_used_at=%d want %d", keys, w.Keys)
	}
	if w.SetupA {
		v, err := healthFlag(db, "setup_called", seed.ProjectA)
		if err != nil {
			return false, "setup_called: " + err.Error()
		}
		if v != 1 {
			return false, "project A setup_called=0"
		}
	}
	for _, f := range w.Flags {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM feature_flags WHERE project_id = ? AND flag_key = ?`, f.Project, f.Key).Scan(&n); err != nil {
			return false, "flag exists: " + err.Error()
		}
		if n == 0 {
			return false, fmt.Sprintf("flag %s/%s does not exist yet", f.Project, f.Key)
		}
	}
	return true, ""
}

// dumpGoldenState renders the tables the awaits watch, for a timeout message.
func dumpGoldenState(db *sql.DB) string {
	var b strings.Builder
	if rows, err := db.Query(`SELECT name, last_used_at IS NOT NULL FROM api_keys ORDER BY name`); err == nil {
		b.WriteString("api_keys (name=used):")
		for rows.Next() {
			var n string
			var u bool
			_ = rows.Scan(&n, &u)
			fmt.Fprintf(&b, " %s=%v", n, u)
		}
		rows.Close()
		b.WriteString("\n")
	}
	if rows, err := db.Query(`SELECT project_id, flag_key, COALESCE(origin,''), last_evaluated_at IS NOT NULL FROM feature_flags ORDER BY project_id, flag_key`); err == nil {
		b.WriteString("feature_flags (project key origin evaluated):\n")
		for rows.Next() {
			var p, k, o string
			var e bool
			_ = rows.Scan(&p, &k, &o, &e)
			fmt.Fprintf(&b, "  %s %s %s %v\n", p, k, o, e)
		}
		rows.Close()
	}
	if n, err := countEvaluationRows(db); err == nil {
		fmt.Fprintf(&b, "flag_evaluations rows=%d\n", n)
	}
	if rows, err := db.Query(`SELECT project_id, setup_called, flags_evaluated, events_received, recordings_received FROM project_health ORDER BY project_id`); err == nil {
		b.WriteString("project_health (project setup flags_evaluated events recordings):\n")
		for rows.Next() {
			var p string
			var a, f, e, r int
			_ = rows.Scan(&p, &a, &f, &e, &r)
			fmt.Fprintf(&b, "  %s %d %d %d %d\n", p, a, f, e, r)
		}
		rows.Close()
	}
	return b.String()
}

func countEvaluationRows(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM flag_evaluations`).Scan(&n)
	return n, err
}

func countTouched(db *sql.DB, projectID string) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM feature_flags WHERE project_id = ? AND last_evaluated_at IS NOT NULL`, projectID).Scan(&n)
	return n, err
}

func healthFlag(db *sql.DB, column, projectID string) (int, error) {
	var v int
	err := db.QueryRow(`SELECT `+column+` FROM project_health WHERE project_id = ?`, projectID).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}

func evaluateLanded(db *sql.DB, seed *goldenSeed, wantEvals int) (bool, string) {
	ta, err := countTouched(db, seed.ProjectA)
	if err != nil {
		return false, "touched A: " + err.Error()
	}
	tb, err := countTouched(db, seed.ProjectB)
	if err != nil {
		return false, "touched B: " + err.Error()
	}
	fa, err := healthFlag(db, "flags_evaluated", seed.ProjectA)
	if err != nil {
		return false, "health A: " + err.Error()
	}
	fb, err := healthFlag(db, "flags_evaluated", seed.ProjectB)
	if err != nil {
		return false, "health B: " + err.Error()
	}
	ev, err := countEvaluationRows(db)
	if err != nil {
		return false, "flag_evaluations: " + err.Error()
	}
	state := fmt.Sprintf("touched A=%d/%d B=%d/%d flags_evaluated A=%d B=%d flag_evaluations=%d/%d", ta, goldenTouchedA, tb, goldenTouchedB, fa, fb, ev, wantEvals)
	// >= for the touches: a flag touched again still counts once. The
	// evaluation rows are exact, since an extra row would change the analysis.
	return ta >= goldenTouchedA && tb >= goldenTouchedB && fa == 1 && fb == 1 && ev == wantEvals, state
}

func ingestLanded(db *sql.DB, seed *goldenSeed) (bool, string) {
	ev, err := healthFlag(db, "events_received", seed.ProjectA)
	if err != nil {
		return false, "events_received: " + err.Error()
	}
	rec, err := healthFlag(db, "recordings_received", seed.ProjectA)
	if err != nil {
		return false, "recordings_received: " + err.Error()
	}
	return ev == 1 && rec == 1, fmt.Sprintf("events_received=%d recordings_received=%d", ev, rec)
}
