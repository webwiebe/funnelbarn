package repository

import (
	"testing"

	"github.com/pressly/goose/v3"
)

// 00038 deletes api_keys rows whose project no longer exists. They cannot
// authenticate anything since #265, but CountOrphanedRows still counted them,
// and that count is logged at Error on every maintenance pass — a BugBarn issue
// (FUN-18) describing a fault that was already fixed.
func TestMigration00038_DeletesOrphanedAPIKeys(t *testing.T) {
	db := openAtVersion(t, 37)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p-live', 'Live', 'live')`)
	mustExec(t, db, `INSERT INTO api_keys (id, project_id, name, key_hash)
		VALUES ('k-live', 'p-live', 'setup', 'hash-live')`)

	mustExec(t, db, `PRAGMA foreign_keys = OFF`)
	mustExec(t, db, `INSERT INTO api_keys (id, project_id, name, key_hash)
		VALUES ('k-orphan', 'p-deleted', 'setup', 'hash-orphan')`)
	mustExec(t, db, `INSERT INTO api_keys (id, project_id, name, key_hash)
		VALUES ('k-orphan-2', 'bad8320c-8aef-408b-a0bf-df4b0adc3877', 'setup', 'hash-orphan-2')`)
	mustExec(t, db, `PRAGMA foreign_keys = ON`)

	if got := countRows(t, db, `SELECT COUNT(*) FROM api_keys`); got != 3 {
		t.Fatalf("setup: want 3 keys before the migration, got %d", got)
	}

	if err := goose.UpTo(db, "migrations", 38); err != nil {
		t.Fatalf("goose up to 38: %v", err)
	}

	for _, c := range []struct {
		name  string
		query string
		want  int
	}{
		{"orphaned key", `SELECT COUNT(*) FROM api_keys WHERE id = 'k-orphan'`, 0},
		{"orphaned key with a UUID project", `SELECT COUNT(*) FROM api_keys WHERE id = 'k-orphan-2'`, 0},
		{"key on a live project", `SELECT COUNT(*) FROM api_keys WHERE id = 'k-live'`, 1},
		{"live project itself", `SELECT COUNT(*) FROM projects WHERE id = 'p-live'`, 1},
	} {
		if got := countRows(t, db, c.query); got != c.want {
			t.Errorf("%s: want %d, got %d", c.name, c.want, got)
		}
	}
}

// A database with nothing to repair — testing, staging, and every fresh
// install — must come through the migration untouched.
func TestMigration00038_LeavesACleanDatabaseAlone(t *testing.T) {
	db := openAtVersion(t, 37)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p1', 'P1', 'p1')`)
	mustExec(t, db, `INSERT INTO api_keys (id, project_id, name, key_hash)
		VALUES ('k1', 'p1', 'setup', 'hash-1')`)
	mustExec(t, db, `INSERT INTO api_keys (id, project_id, name, key_hash)
		VALUES ('k2', 'p1', 'ingest', 'hash-2')`)

	if err := goose.UpTo(db, "migrations", 38); err != nil {
		t.Fatalf("goose up to 38: %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM api_keys`); got != 2 {
		t.Errorf("want both keys kept, got %d", got)
	}
}

// The deletion is written as the condition that selects the rows, so replaying
// it matches nothing. Down is a no-op, so Down then Up must not lose a live key.
func TestMigration00038_IsIdempotent(t *testing.T) {
	db := openAtVersion(t, 37)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p-live', 'Live', 'live')`)
	mustExec(t, db, `INSERT INTO api_keys (id, project_id, name, key_hash)
		VALUES ('k-live', 'p-live', 'setup', 'hash-live')`)
	mustExec(t, db, `PRAGMA foreign_keys = OFF`)
	mustExec(t, db, `INSERT INTO api_keys (id, project_id, name, key_hash)
		VALUES ('k-orphan', 'p-deleted', 'setup', 'hash-orphan')`)
	mustExec(t, db, `PRAGMA foreign_keys = ON`)

	if err := goose.UpTo(db, "migrations", 38); err != nil {
		t.Fatalf("goose up to 38: %v", err)
	}
	if err := goose.Down(db, "migrations"); err != nil {
		t.Fatalf("goose down: %v", err)
	}
	if err := goose.UpTo(db, "migrations", 38); err != nil {
		t.Fatalf("goose up to 38 again: %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM api_keys WHERE id = 'k-live'`); got != 1 {
		t.Errorf("live key did not survive a replay: want 1, got %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM api_keys WHERE id = 'k-orphan'`); got != 0 {
		t.Errorf("orphan came back: want 0, got %d", got)
	}
}

// CountOrphanedRows is what turns a leftover key into a BugBarn issue, so the
// migration has to bring its api_keys count to zero — that is the signal the
// maintenance pass reports.
func TestMigration00038_ClearsTheOrphanCountThatFiledTheIssue(t *testing.T) {
	db := openAtVersion(t, 37)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p-live', 'Live', 'live')`)
	mustExec(t, db, `PRAGMA foreign_keys = OFF`)
	mustExec(t, db, `INSERT INTO api_keys (id, project_id, name, key_hash)
		VALUES ('k-orphan', 'p-deleted', 'setup', 'hash-orphan')`)
	mustExec(t, db, `PRAGMA foreign_keys = ON`)

	orphaned := `SELECT COUNT(*) FROM api_keys k WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = k.project_id)`
	if got := countRows(t, db, orphaned); got != 1 {
		t.Fatalf("setup: want 1 orphaned key, got %d", got)
	}

	if err := goose.UpTo(db, "migrations", 38); err != nil {
		t.Fatalf("goose up to 38: %v", err)
	}

	if got := countRows(t, db, orphaned); got != 0 {
		t.Errorf("orphaned api_keys after the migration: want 0, got %d", got)
	}
}
