package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

// 00041 rewrites the values of sensitive URL parameters in rows stored before
// ingest redacted them. Rows stay, clean rows stay as they are, and a second
// run changes nothing.
func TestMigration00041_RedactsStoredURLs(t *testing.T) {
	db := openAtVersion(t, 40)

	mustExec(t, db, `INSERT INTO projects (id, name, slug) VALUES ('p1', 'P1', 'p1')`)
	insertEvent := func(id, url, referrer, props any) {
		mustExec(t, db, `INSERT INTO events (id, project_id, session_id, name, url, referrer, properties, ingest_id, occurred_at)
			VALUES (?, 'p1', 's1', 'page_view', ?, ?, ?, ?, '2026-10-01T00:00:00Z')`, id, url, referrer, props, "i-"+id.(string))
	}
	insertEvent("e-token", "https://app.test/login?token=abc123&x=1", "https://idp.test/?invite=inv9", `{"next":"https://app.test/a?access_token=zzz","n":12345678901234567890}`)
	insertEvent("e-clean", "https://app.test/list?page=2#pricing", "https://news.test/", `{"label":"free text with ? mark"}`)
	insertEvent("e-null", nil, nil, nil)
	mustExec(t, db, `INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at, entry_url, exit_url, referrer)
		VALUES ('s1', 'p1', '2026-10-01T00:00:00Z', '2026-10-01T00:01:00Z',
		'https://app.test/in?token=t1', 'https://app.test/out?code=c2&ok=1', 'https://idp.test/?state=s3')`)
	mustExec(t, db, `INSERT INTO recordings (id, project_id, session_id, started_at, page_url)
		VALUES ('r1', 'p1', 's1', '2026-10-01T00:00:00Z', 'https://app.test/r?token=t4')`)
	mustExec(t, db, `INSERT INTO recording_traces (project_id, session_id, recording_id, trace_id, url, occurred_at)
		VALUES ('p1', 's1', 'r1', 'tr1', 'https://app.test/api?client_secret=q', '2026-10-01T00:00:01Z')`)

	if err := goose.UpTo(db, "migrations", 41); err != nil {
		t.Fatalf("goose up to 41: %v", err)
	}

	str := func(q string, args ...any) string {
		t.Helper()
		var s string
		if err := db.QueryRow(q, args...).Scan(&s); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return s
	}
	for _, c := range []struct{ name, got, want string }{
		{"event url", str(`SELECT url FROM events WHERE id='e-token'`), "https://app.test/login?token=redacted&x=1"},
		{"event referrer", str(`SELECT referrer FROM events WHERE id='e-token'`), "https://idp.test/?invite=redacted"},
		{"event properties", str(`SELECT properties FROM events WHERE id='e-token'`), `{"n":12345678901234567890,"next":"https://app.test/a?access_token=redacted"}`},
		{"clean event url", str(`SELECT url FROM events WHERE id='e-clean'`), "https://app.test/list?page=2#pricing"},
		{"clean event properties", str(`SELECT properties FROM events WHERE id='e-clean'`), `{"label":"free text with ? mark"}`},
		{"session entry", str(`SELECT entry_url FROM sessions WHERE id='s1'`), "https://app.test/in?token=redacted"},
		{"session exit", str(`SELECT exit_url FROM sessions WHERE id='s1'`), "https://app.test/out?code=redacted&ok=1"},
		{"session referrer", str(`SELECT referrer FROM sessions WHERE id='s1'`), "https://idp.test/?state=redacted"},
		{"recording page", str(`SELECT page_url FROM recordings WHERE id='r1'`), "https://app.test/r?token=redacted"},
		{"trace url", str(`SELECT url FROM recording_traces WHERE trace_id='tr1'`), "https://app.test/api?client_secret=redacted"},
	} {
		if c.got != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.name, c.got, c.want)
		}
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM events`); n != 3 {
		t.Errorf("events kept: want 3, got %d", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM events WHERE url IS NULL AND referrer IS NULL AND properties IS NULL`); n != 1 {
		t.Errorf("null row kept as it was: want 1, got %d", n)
	}

	// A second run finds nothing to change.
	before := str(`SELECT group_concat(url || referrer || properties, '|') FROM (SELECT * FROM events WHERE url IS NOT NULL ORDER BY id)`)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := up00041RedactStoredURLs(context.Background(), tx); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	after := str(`SELECT group_concat(url || referrer || properties, '|') FROM (SELECT * FROM events WHERE url IS NOT NULL ORDER BY id)`)
	if before != after {
		t.Errorf("second run changed rows:\n before %s\n after  %s", before, after)
	}
	if strings.Contains(after, "abc123") {
		t.Errorf("token value still stored: %s", after)
	}
}
