package repository

import (
	"database/sql"
	"testing"
)

func openPlainSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// A read that fails partway through PRAGMA table_info used to end the loop
// like a table without the column, so ensureColumns ran an ALTER TABLE for a
// column that might exist and failed startup on the wrong error.
func TestScanForColumn_ReadErrorIsNotAMissingColumn(t *testing.T) {
	db := openPlainSQLite(t)
	// The second row overflows abs() while SQLite steps to it, which is an
	// iteration error after one good row.
	rows, err := db.Query(`SELECT 0, 'id', 'TEXT', 0, NULL, 0
		UNION ALL SELECT abs(-9223372036854775808), 'status', 'TEXT', 0, NULL, 0`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	found, err := scanForColumn(rows, "status")
	if err == nil {
		t.Fatalf("scanForColumn = %v, nil; want the read error", found)
	}
}

func TestTableHasColumn(t *testing.T) {
	db := openPlainSQLite(t)
	if _, err := db.Exec(`CREATE TABLE projects (id TEXT, status TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	for column, want := range map[string]bool{"status": true, "missing": false} {
		got, err := tableHasColumn(db, "projects", column)
		if err != nil || got != want {
			t.Errorf("tableHasColumn(%q) = %v, %v; want %v, nil", column, got, err, want)
		}
	}
}
