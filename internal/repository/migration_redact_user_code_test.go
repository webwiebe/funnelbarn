package repository

import (
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMigration00042_RedactsStoredUserCode(t *testing.T) {
	db := openAtVersion(t, 41)
	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p1', 'P1', 'p1')`)
	mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, url, ingest_id, occurred_at)
		VALUES ('e1', 'p1', 's1', 'page_view', 'https://iam.test/device?user_code=ABCD-EFGH&x=1', 'i1', '2026-08-17T00:00:00Z')`)
	if err := goose.UpTo(db, "migrations", 42); err != nil {
		t.Fatalf("goose up to 42: %v", err)
	}
	var got string
	if err := db.QueryRow(`SELECT url FROM events WHERE id='e1'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if want := "https://iam.test/device?user_code=redacted&x=1"; got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
}
