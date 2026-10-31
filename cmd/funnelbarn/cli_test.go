package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

func replayableRecord(ingestID, slug string) spool.Record {
	return spool.Record{
		IngestID:    ingestID,
		ReceivedAt:  time.Now().UTC(),
		ProjectSlug: slug,
		BodyBase64:  base64.StdEncoding.EncodeToString([]byte(`{"name":"pageview","url":"https://example.com/x"}`)),
	}
}

func storedEventCount(t *testing.T, store *repository.Store, ingestID string) int {
	t.Helper()
	var n int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE ingest_id = ?`, ingestID).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// Each way a record can fail returns false, which keeps the dead-letter file,
// and logs its own line. Every case asserts that line, so a record refused for
// the wrong reason fails the test. Only Error reaches BugBarn, so the level is
// asserted too: a record waiting on --project is an operator input and stays
// at Warn.
func TestReplayRecord_Failures(t *testing.T) {
	cases := []struct {
		name    string
		record  spool.Record
		project string
		setup   func(t *testing.T, store *repository.Store)
		wantLog string
	}{
		{
			name:    "no slug and no project",
			record:  replayableRecord("rr-no-slug", ""),
			wantLog: `"level":"WARN","msg":"replay: record has no project slug and no --project given"`,
		},
		{
			name: "body does not decode",
			record: spool.Record{
				IngestID:    "rr-bad-body",
				ReceivedAt:  time.Now().UTC(),
				ProjectSlug: "recovered",
				BodyBase64:  "not base64!",
			},
			wantLog: `"level":"ERROR","msg":"replay: process record"`,
		},
		{
			name:    "slug cannot name a project",
			record:  replayableRecord("rr-bad-slug", "Bad Slug"),
			wantLog: `"level":"ERROR","msg":"replay: resolve project"`,
		},
		{
			name:   "store refuses the event",
			record: replayableRecord("rr-refused", "recovered"),
			setup: func(t *testing.T, store *repository.Store) {
				t.Helper()
				_, err := store.DB().Exec(`CREATE TRIGGER refuse_events BEFORE INSERT ON events
					BEGIN SELECT RAISE(ABORT, 'refused by test'); END`)
				if err != nil {
					t.Fatalf("create trigger: %v", err)
				}
			},
			wantLog: `"level":"ERROR","msg":"replay: persist event"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			store := repotest.Open(t)
			if tc.setup != nil {
				tc.setup(t, store)
			}

			if replayRecord(context.Background(), store, tc.record, tc.project, false) {
				t.Fatal("replayRecord = true, want false")
			}
			if !strings.Contains(logs.String(), tc.wantLog) {
				t.Errorf("logs = %s, want a line with %s", logs.String(), tc.wantLog)
			}
			if got := storedEventCount(t, store, tc.record.IngestID); got != 0 {
				t.Errorf("stored %d events, want 0", got)
			}
		})
	}
}

func TestReplayRecord_DryRunStoresNoEvent(t *testing.T) {
	store := repotest.Open(t)
	record := replayableRecord("rr-dry", "recovered")

	if !replayRecord(context.Background(), store, record, "", true) {
		t.Fatal("replayRecord = false, want true for a record that could be applied")
	}
	if got := storedEventCount(t, store, record.IngestID); got != 0 {
		t.Errorf("dry run stored %d events, want 0", got)
	}
}

// A dry run must not leave an empty project behind for a slug it only previewed.
func TestReplayRecord_DryRunCreatesNoProject(t *testing.T) {
	store := repotest.Open(t)
	record := replayableRecord("rr-dry-project", "not-created-yet")

	if !replayRecord(context.Background(), store, record, "", true) {
		t.Fatal("replayRecord = false, want true for a slug a real run could create")
	}
	var n int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM projects WHERE slug = ?`, record.ProjectSlug).Scan(&n); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if n != 0 {
		t.Errorf("dry run created %d projects, want 0", n)
	}
}

// The preview has to match the real run: a slug a real run refuses is skipped,
// which keeps the dead-letter file on a real run.
func TestReplayRecord_DryRunRefusesAnUnusableSlug(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	store := repotest.Open(t)
	if replayRecord(context.Background(), store, replayableRecord("rr-dry-bad", "Bad Slug"), "", true) {
		t.Fatal("replayRecord = true, want false")
	}
	want := `"level":"ERROR","msg":"replay: resolve project"`
	if !strings.Contains(logs.String(), want) {
		t.Errorf("logs = %s, want a line with %s", logs.String(), want)
	}
}

// --project names the project for records that carry none. A record that names
// its own keeps it.
func TestReplayRecord_RecordSlugWinsOverProjectFlag(t *testing.T) {
	store := repotest.Open(t)
	record := replayableRecord("rr-own-slug", "own-project")

	if !replayRecord(context.Background(), store, record, "flag-project", false) {
		t.Fatal("replayRecord = false, want true")
	}
	var slug string
	err := store.DB().QueryRow(`SELECT p.slug FROM events e JOIN projects p ON p.id = e.project_id
		WHERE e.ingest_id = ?`, record.IngestID).Scan(&slug)
	if err != nil {
		t.Fatalf("read stored event: %v", err)
	}
	if slug != "own-project" {
		t.Errorf("event filed under %q, want own-project", slug)
	}
}

// One failed record keeps the whole file, so the failure can be retried. The
// record that did apply is stored, and a retry skips it by ingest id.
func TestRunReplayDeadLetter_KeepsFileWhenARecordFails(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DBPath: filepath.Join(dir, "test.db"), SpoolDir: dir}

	good := replayableRecord("rr-good", "recovered")
	bad := replayableRecord("rr-bad", "Bad Slug")
	for _, record := range []spool.Record{good, bad} {
		if err := spool.AppendDeadLetter(dir, record); err != nil {
			t.Fatalf("AppendDeadLetter: %v", err)
		}
	}

	if err := runReplayDeadLetter(cfg, nil); err != nil {
		t.Fatalf("replay: %v", err)
	}

	left, err := spool.ReadRecords(spool.DeadLetterPath(dir))
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(left) != 2 {
		t.Errorf("dead-letter file holds %d records, want 2", len(left))
	}

	if err := runReplayDeadLetter(cfg, nil); err != nil {
		t.Fatalf("retry: %v", err)
	}

	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if got := storedEventCount(t, store, good.IngestID); got != 1 {
		t.Errorf("stored %d of the good record, want 1", got)
	}
}
