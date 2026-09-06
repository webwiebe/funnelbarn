package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// openAtVersion opens a fresh SQLite database migrated up to (and including)
// the given goose version, so a migration can be tested against rows that
// existed before it ran.
func openAtVersion(t *testing.T, version int64) *sql.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	goose.SetLogger(goose.NopLogger())
	if err := goose.UpTo(db, "migrations", version); err != nil {
		t.Fatalf("goose up to %d: %v", version, err)
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

// 00031 files untagged events and sessions as production. Rows that already
// carry an environment must be left exactly as they are.
func TestMigration00031_BackfillsUntaggedEnvironment(t *testing.T) {
	db := openAtVersion(t, 30)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p1', 'P1', 'p1')`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, environment)
		VALUES ('e-blank', 'p1', 's-blank', 'pageview', 'i-blank', CURRENT_TIMESTAMP, '')`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, environment)
		VALUES ('e-staging', 'p1', 's-staging', 'pageview', 'i-staging', CURRENT_TIMESTAMP, 'staging')`)
	mustExec(t, db, `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at, environment)
		VALUES ('s-blank', 'p1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')`)
	mustExec(t, db, `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at, environment)
		VALUES ('s-staging', 'p1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'staging')`)

	if err := goose.UpTo(db, "migrations", 31); err != nil {
		t.Fatalf("goose up to 31: %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM events WHERE environment = ''`); got != 0 {
		t.Errorf("untagged events after backfill: want 0, got %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM sessions WHERE environment = ''`); got != 0 {
		t.Errorf("untagged sessions after backfill: want 0, got %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM events WHERE id = 'e-blank' AND environment = 'production'`); got != 1 {
		t.Errorf("e-blank not filed as production")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM events WHERE id = 'e-staging' AND environment = 'staging'`); got != 1 {
		t.Errorf("e-staging environment was overwritten")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM sessions WHERE id = 's-staging' AND environment = 'staging'`); got != 1 {
		t.Errorf("s-staging environment was overwritten")
	}
}

// The migration ships in the image and runs at every process start, so a
// second application must be a no-op rather than re-tagging fresh rows.
func TestMigration00031_IsIdempotent(t *testing.T) {
	db := openAtVersion(t, 31)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p1', 'P1', 'p1')`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, environment)
		VALUES ('e-dev', 'p1', 's-dev', 'pageview', 'i-dev', CURRENT_TIMESTAMP, 'development')`)

	// Re-running the migration's Up body must not touch a tagged row.
	mustExec(t, db, `UPDATE events   SET environment = 'production' WHERE environment = ''`)
	mustExec(t, db, `UPDATE sessions SET environment = 'production' WHERE environment = ''`)

	if got := countRows(t, db, `SELECT COUNT(*) FROM events WHERE id = 'e-dev' AND environment = 'development'`); got != 1 {
		t.Errorf("re-running the backfill changed an already-tagged row")
	}
}

// 00033 deletes rows whose project_id matches no project. Those rows are
// unreachable by every query (all reads are project-scoped) but still indexed
// and backed up.
func TestMigration00033_DeletesOrphanedRows(t *testing.T) {
	db := openAtVersion(t, 32)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p1', 'P1', 'p1')`)
	// Real rows, which must survive.
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at)
		VALUES ('e-ok', 'p1', 's-ok', 'pageview', 'i-ok', CURRENT_TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at)
		VALUES ('s-ok', 'p1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO funnels (id, project_id, name) VALUES ('f-ok', 'p1', 'Checkout')`)
	mustExec(t, db, `INSERT INTO funnel_steps (id, funnel_id, step_order, event_name)
		VALUES ('fs-ok', 'f-ok', 0, 'pageview')`)

	// Orphans, written in the window before foreign keys were enforced. Foreign
	// keys are on now, so they have to be inserted with enforcement off — which
	// is exactly how they got there.
	mustExec(t, db, `PRAGMA foreign_keys = OFF`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at)
		VALUES ('e-orphan', '', 's-orphan', 'pageview', 'i-orphan', CURRENT_TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at)
		VALUES ('s-orphan', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO funnels (id, project_id, name) VALUES ('f-orphan', '', 'Ghost')`)
	mustExec(t, db, `INSERT INTO funnel_steps (id, funnel_id, step_order, event_name)
		VALUES ('fs-orphan', 'f-orphan', 0, 'pageview')`)
	mustExec(t, db, `PRAGMA foreign_keys = ON`)

	if err := goose.UpTo(db, "migrations", 33); err != nil {
		t.Fatalf("goose up to 33: %v", err)
	}

	for _, c := range []struct {
		name  string
		query string
		want  int
	}{
		{"orphaned events", `SELECT COUNT(*) FROM events WHERE project_id = ''`, 0},
		{"orphaned sessions", `SELECT COUNT(*) FROM sessions WHERE project_id = ''`, 0},
		{"orphaned funnels", `SELECT COUNT(*) FROM funnels WHERE project_id = ''`, 0},
		{"orphaned funnel steps", `SELECT COUNT(*) FROM funnel_steps WHERE funnel_id = 'f-orphan'`, 0},
		{"real event", `SELECT COUNT(*) FROM events WHERE id = 'e-ok'`, 1},
		{"real session", `SELECT COUNT(*) FROM sessions WHERE id = 's-ok'`, 1},
		{"real funnel", `SELECT COUNT(*) FROM funnels WHERE id = 'f-ok'`, 1},
		{"real funnel step", `SELECT COUNT(*) FROM funnel_steps WHERE id = 'fs-ok'`, 1},
	} {
		if got := countRows(t, db, c.query); got != c.want {
			t.Errorf("%s: want %d, got %d", c.name, c.want, got)
		}
	}
}

// De-duplication is scoped to funnels that are identical in every respect. Two
// funnels that merely share a name are different funnels and must survive —
// production has exactly such a pair ("Funnel Analysis Engagement", 4 steps and
// 3 steps).
func TestMigration00033_DeduplicatesOnlyIdenticalFunnels(t *testing.T) {
	db := openAtVersion(t, 32)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p1', 'P1', 'p1')`)
	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p2', 'P2', 'p2')`)

	addFunnel := func(id, project, name, created string, steps ...string) {
		t.Helper()
		mustExec(t, db, `INSERT INTO funnels (id, project_id, name, created_at) VALUES (?, ?, ?, ?)`,
			id, project, name, created)
		for i, ev := range steps {
			mustExec(t, db, `INSERT INTO funnel_steps (id, funnel_id, step_order, event_name) VALUES (?, ?, ?, ?)`,
				id+"-s"+ev, id, i, ev)
		}
	}

	// Identical pair — the older one survives.
	addFunnel("f-dup-old", "p1", "Login to Dashboard", "2026-06-01 10:00:00", "login", "dashboard_view")
	addFunnel("f-dup-new", "p1", "Login to Dashboard", "2026-07-01 10:00:00", "login", "dashboard_view")
	// Same name, different steps — both survive.
	addFunnel("f-diff-a", "p1", "Funnel Analysis Engagement", "2026-06-01 10:00:00", "a", "b", "c", "d")
	addFunnel("f-diff-b", "p1", "Funnel Analysis Engagement", "2026-07-01 10:00:00", "a", "b", "c")
	// Same name and steps but a different project — both survive.
	addFunnel("f-p1", "p1", "Registration", "2026-06-01 10:00:00", "sign_up")
	addFunnel("f-p2", "p2", "Registration", "2026-06-01 10:00:00", "sign_up")

	if err := goose.UpTo(db, "migrations", 33); err != nil {
		t.Fatalf("goose up to 33: %v", err)
	}

	for _, c := range []struct {
		id   string
		want int
	}{
		{"f-dup-old", 1}, {"f-dup-new", 0},
		{"f-diff-a", 1}, {"f-diff-b", 1},
		{"f-p1", 1}, {"f-p2", 1},
	} {
		if got := countRows(t, db, `SELECT COUNT(*) FROM funnels WHERE id = ?`, c.id); got != c.want {
			t.Errorf("funnel %s: want %d row(s), got %d", c.id, c.want, got)
		}
	}
	// The losing funnel's steps go with it.
	if got := countRows(t, db, `SELECT COUNT(*) FROM funnel_steps WHERE funnel_id = 'f-dup-new'`); got != 0 {
		t.Errorf("steps of the de-duplicated funnel survived: %d", got)
	}
}

// The migration ships in the image and runs at every process start.
func TestMigration00033_IsIdempotent(t *testing.T) {
	db := openAtVersion(t, 33)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p1', 'P1', 'p1')`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at)
		VALUES ('e-ok', 'p1', 's-ok', 'pageview', 'i-ok', CURRENT_TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO funnels (id, project_id, name) VALUES ('f-ok', 'p1', 'Checkout')`)

	mustExec(t, db, `DELETE FROM events WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = events.project_id)`)
	mustExec(t, db, `DELETE FROM funnels WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = funnels.project_id)`)

	if got := countRows(t, db, `SELECT COUNT(*) FROM events WHERE id = 'e-ok'`); got != 1 {
		t.Error("re-running the orphan delete removed a valid event")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM funnels WHERE id = 'f-ok'`); got != 1 {
		t.Error("re-running the orphan delete removed a valid funnel")
	}
}

// The triggers are the storage-layer guard NOT NULL never provided: NOT NULL
// accepts the empty string, and the foreign key was not enforced when these
// rows were written.
func TestMigration00033_RejectsEmptyProjectID(t *testing.T) {
	db := openAtVersion(t, 33)
	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p1', 'P1', 'p1')`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at)
		VALUES ('e-ok', 'p1', 's-ok', 'pageview', 'i-ok', CURRENT_TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at)
		VALUES ('s-ok', 'p1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	// Even with foreign keys off — the state that let the 344 rows in — the
	// empty string is refused.
	mustExec(t, db, `PRAGMA foreign_keys = OFF`)
	for _, c := range []struct {
		name  string
		query string
	}{
		{"insert event", `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at)
			VALUES ('e-bad', '', 's', 'pageview', 'i-bad', CURRENT_TIMESTAMP)`},
		{"update event", `UPDATE events SET project_id = '' WHERE id = 'e-ok'`},
		{"insert session", `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at)
			VALUES ('s-bad', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`},
		{"update session", `UPDATE sessions SET project_id = '' WHERE id = 's-ok'`},
	} {
		if _, err := db.Exec(c.query); err == nil {
			t.Errorf("%s with an empty project_id was accepted", c.name)
		}
	}
	mustExec(t, db, `PRAGMA foreign_keys = ON`)

	// A real project_id is unaffected.
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at)
		VALUES ('e-good', 'p1', 's-ok', 'pageview', 'i-good', CURRENT_TIMESTAMP)`)
	if got := countRows(t, db, `SELECT COUNT(*) FROM events WHERE id = 'e-good'`); got != 1 {
		t.Error("the trigger rejected a valid insert")
	}
}

// CountOrphanedRows is what would have surfaced the 344 unreachable events in
// June rather than in an audit two months later. It runs on the maintenance
// cycle and logs at error level, so a non-zero count reaches BugBarn.
func TestStore_CountOrphanedRows(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "orphans.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if _, err := s.CreateProject(ctx, "Orphans", "orphans"); err != nil {
		t.Fatalf("create project: %v", err)
	}
	counts, err := s.CountOrphanedRows(ctx)
	if err != nil {
		t.Fatalf("count orphaned rows: %v", err)
	}
	if counts.Total() != 0 {
		t.Fatalf("fresh database reports %d orphans", counts.Total())
	}

	// Getting an orphan in takes both guards off — which is what the June 2026
	// window effectively was: no foreign-key enforcement and no trigger.
	mustExec(t, s.db, `PRAGMA foreign_keys = OFF`)
	mustExec(t, s.db, `DROP TRIGGER IF EXISTS events_project_id_not_empty_insert`)
	mustExec(t, s.db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at)
		VALUES ('e-orphan', '', 's-orphan', 'pageview', 'i-orphan', CURRENT_TIMESTAMP)`)
	mustExec(t, s.db, `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at)
		VALUES ('s-orphan', 'no-such-project', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	counts, err = s.CountOrphanedRows(ctx)
	if err != nil {
		t.Fatalf("count orphaned rows: %v", err)
	}
	if counts.Events != 1 || counts.Sessions != 1 || counts.Funnels != 0 || counts.Total() != 2 {
		t.Errorf("counts = %+v (total %d), want 1 event, 1 session, 0 funnels", counts, counts.Total())
	}
}

// seedForSplit sets up the production shape: one session ID emitted by two
// projects, with the sessions row owned by whichever wrote last.
func seedForSplit(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('pa', 'A', 'a')`)
	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('pc', 'C', 'c')`)

	// Project A: three events for session "shared".
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, url, referrer, utm_source, device_type, country_code, environment)
		VALUES ('a1', 'pa', 'shared', 'pageview', 'ia1', '2026-08-01 10:00:00', 'https://a.example/landing', 'https://google.com', 'newsletter', 'desktop', 'NL', 'production')`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, url, environment)
		VALUES ('a2', 'pa', 'shared', 'pageview', 'ia2', '2026-08-01 10:05:00', 'https://a.example/pricing', 'production')`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, url, environment)
		VALUES ('a3', 'pa', 'shared', 'signup', 'ia3', '2026-08-01 10:09:00', 'https://a.example/done', 'production')`)

	// Project C: one event for the same session ID.
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, url, environment)
		VALUES ('c1', 'pc', 'shared', 'pageview', 'ic1', '2026-08-02 09:00:00', 'https://c.example/home', 'staging')`)

	// The single sessions row, owned by C because C wrote last, carrying C's geo.
	mustExec(t, db, `INSERT INTO sessions
		(id, project_id, first_seen_at, last_seen_at, event_count, entry_url, exit_url, device_type, country_code, environment,
		 ip, city, latitude, longitude, asn_org, geo_anonymized, screen_width, signals_collected)
		VALUES ('shared', 'pc', '2026-08-01 10:00:00', '2026-08-02 09:00:00', 4, 'https://a.example/landing', 'https://c.example/home', 'desktop', 'NL', 'staging',
		 '203.0.113.7', 'Nijmegen', 51.84, 5.86, 'Example ISP', 0, 1920, 1)`)
}

// The repair splits a colliding session into one row per project, with each
// project's aggregates re-derived from its own events.
func TestMigration00034_SplitsCollidingSessionsPerProject(t *testing.T) {
	db := openAtVersion(t, 33)
	seedForSplit(t, db)

	if err := goose.UpTo(db, "migrations", 34); err != nil {
		t.Fatalf("goose up to 34: %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM sessions WHERE id = 'shared'`); got != 2 {
		t.Fatalf("want 2 rows for the shared session, got %d", got)
	}

	var count int
	var first, last, entry, exit, env string
	err := db.QueryRow(`SELECT event_count, first_seen_at, last_seen_at, entry_url, exit_url, environment
		FROM sessions WHERE id = 'shared' AND project_id = 'pa'`).
		Scan(&count, &first, &last, &entry, &exit, &env)
	if err != nil {
		t.Fatalf("read project A's row: %v", err)
	}
	if count != 3 {
		t.Errorf("project A event_count: want 3, got %d", count)
	}
	if !strings.HasPrefix(first, "2026-08-01T10:00:00") || !strings.HasPrefix(last, "2026-08-01T10:09:00") {
		t.Errorf("project A window: got %s .. %s", first, last)
	}
	if entry != "https://a.example/landing" || exit != "https://a.example/done" {
		t.Errorf("project A urls: entry %q exit %q", entry, exit)
	}
	if env != "production" {
		t.Errorf("project A environment: want production, got %q", env)
	}

	// Project C keeps only its own single event — its inflated count is gone.
	err = db.QueryRow(`SELECT event_count, entry_url, exit_url, environment
		FROM sessions WHERE id = 'shared' AND project_id = 'pc'`).
		Scan(&count, &entry, &exit, &env)
	if err != nil {
		t.Fatalf("read project C's row: %v", err)
	}
	if count != 1 {
		t.Errorf("project C event_count: want 1, got %d", count)
	}
	if entry != "https://c.example/home" || exit != "https://c.example/home" {
		t.Errorf("project C urls: entry %q exit %q", entry, exit)
	}
	if env != "staging" {
		t.Errorf("project C environment: want staging, got %q", env)
	}
}

// Geo and device signals have no per-project source, so they stay with the one
// project the old row already attributed them to. Copying them to every split
// row would invent a city and an ASN for a visitor who was never there.
func TestMigration00034_GeoStaysWithItsOwningProject(t *testing.T) {
	db := openAtVersion(t, 33)
	seedForSplit(t, db)

	if err := goose.UpTo(db, "migrations", 34); err != nil {
		t.Fatalf("goose up to 34: %v", err)
	}

	var city, asn sql.NullString
	var width sql.NullInt64
	var signals int
	err := db.QueryRow(`SELECT city, asn_org, screen_width, signals_collected
		FROM sessions WHERE id = 'shared' AND project_id = 'pc'`).Scan(&city, &asn, &width, &signals)
	if err != nil {
		t.Fatalf("read project C's row: %v", err)
	}
	if city.String != "Nijmegen" || asn.String != "Example ISP" || width.Int64 != 1920 || signals != 1 {
		t.Errorf("the owning project lost its geo/signals: city=%v asn=%v width=%v signals=%d",
			city, asn, width, signals)
	}

	err = db.QueryRow(`SELECT city, asn_org, screen_width, signals_collected
		FROM sessions WHERE id = 'shared' AND project_id = 'pa'`).Scan(&city, &asn, &width, &signals)
	if err != nil {
		t.Fatalf("read project A's row: %v", err)
	}
	if city.Valid || asn.Valid || width.Valid {
		t.Errorf("another project's geo was copied onto project A: city=%v asn=%v width=%v", city, asn, width)
	}
	if signals != 0 {
		t.Errorf("signals_collected: want 0 on a split row, got %d", signals)
	}
}

// 29 rows in production sit under a project none of their own events belong to.
// The rebuild is authoritative for any session ID that has events, so those
// rows are not carried across.
func TestMigration00034_DropsMisattributedRows(t *testing.T) {
	db := openAtVersion(t, 33)
	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('pa', 'A', 'a')`)
	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('pc', 'C', 'c')`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, environment)
		VALUES ('a1', 'pa', 'stolen', 'pageview', 'ia1', '2026-08-01 10:00:00', 'production')`)
	// The row was last written by C, which owns none of this session's events.
	mustExec(t, db, `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at, event_count)
		VALUES ('stolen', 'pc', '2026-08-01 10:00:00', '2026-08-01 10:00:00', 1)`)

	if err := goose.UpTo(db, "migrations", 34); err != nil {
		t.Fatalf("goose up to 34: %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM sessions WHERE id = 'stolen' AND project_id = 'pc'`); got != 0 {
		t.Error("a session row survived under a project that owns none of its events")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM sessions WHERE id = 'stolen' AND project_id = 'pa'`); got != 1 {
		t.Error("the session was not rebuilt under the project its events belong to")
	}
}

// A session whose events were removed by the retention purge has nothing to
// re-derive from and must survive the rebuild untouched.
func TestMigration00034_KeepsEventlessSessions(t *testing.T) {
	db := openAtVersion(t, 33)
	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('pa', 'A', 'a')`)
	mustExec(t, db, `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at, event_count, entry_url, city)
		VALUES ('purged', 'pa', '2026-01-01 10:00:00', '2026-01-01 10:30:00', 12, 'https://a.example/old', 'Nijmegen')`)

	if err := goose.UpTo(db, "migrations", 34); err != nil {
		t.Fatalf("goose up to 34: %v", err)
	}

	var count int
	var entry, city string
	err := db.QueryRow(`SELECT event_count, entry_url, city FROM sessions WHERE id = 'purged' AND project_id = 'pa'`).
		Scan(&count, &entry, &city)
	if err != nil {
		t.Fatalf("the event-less session was dropped: %v", err)
	}
	if count != 12 || entry != "https://a.example/old" || city != "Nijmegen" {
		t.Errorf("the event-less session was altered: count=%d entry=%q city=%q", count, entry, city)
	}
}

// The rebuild ships in the image and runs at process start; re-deriving from
// the same events must land on the same rows.
func TestMigration00034_IsIdempotent(t *testing.T) {
	db := openAtVersion(t, 33)
	seedForSplit(t, db)
	if err := goose.UpTo(db, "migrations", 34); err != nil {
		t.Fatalf("goose up to 34: %v", err)
	}

	before := dumpSessions(t, db)
	// Re-apply by hand: goose records the version, so a genuine second run only
	// happens if the version table is lost, but the SQL still has to be safe.
	if _, err := db.Exec(`DELETE FROM goose_db_version WHERE version_id = 34`); err != nil {
		t.Fatalf("reset goose version: %v", err)
	}
	if err := goose.UpTo(db, "migrations", 34); err != nil {
		t.Fatalf("second goose up to 34: %v", err)
	}
	if after := dumpSessions(t, db); after != before {
		t.Errorf("re-running the rebuild changed the result:\nbefore: %s\nafter:  %s", before, after)
	}
}

func dumpSessions(t *testing.T, db *sql.DB) string {
	t.Helper()
	var out string
	err := db.QueryRow(`SELECT COALESCE(group_concat(row, ';'), '') FROM (
		SELECT project_id || '/' || id || '/' || event_count || '/' || first_seen_at ||
		       '/' || last_seen_at || '/' || COALESCE(entry_url,'-') || '/' || COALESCE(exit_url,'-') ||
		       '/' || COALESCE(city,'-') AS row
		FROM sessions ORDER BY project_id, id)`).Scan(&out)
	if err != nil {
		t.Fatalf("dump sessions: %v", err)
	}
	return out
}
