package repository

import (
	"database/sql"
	"path/filepath"
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
