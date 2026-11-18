package pipeline

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

func newTestApplier(t *testing.T) (*Applier, *repository.Store) {
	t.Helper()
	store, err := repository.Open(repotest.Path(t, "apply.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return &Applier{Store: store, Health: NewMonitor(MonitorOptions{})}, store
}

func applyRecord(ingestID, slug, body string) spool.Record {
	return spool.Record{
		IngestID:    ingestID,
		ReceivedAt:  time.Now().UTC(),
		ContentType: "application/json",
		BodyBase64:  base64.StdEncoding.EncodeToString([]byte(body)),
		ProjectSlug: slug,
	}
}

func countEvents(t *testing.T, store *repository.Store, ingestID string) int {
	t.Helper()
	var n int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE ingest_id = ?`, ingestID).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// The queue consumer and the spool worker both call apply, and a queue can
// deliver a record twice. The second apply must store nothing.
func TestIngestApply_StoresOnceOnRedelivery(t *testing.T) {
	a, store := newTestApplier(t)
	rec := applyRecord("ing-apply-1", "apply-site", `{"name":"pageview","url":"https://example.com/","session_id":"s1"}`)

	for i := 0; i < 2; i++ {
		if err := a.apply(context.Background(), rec, false); err != nil {
			t.Fatalf("apply #%d: %v", i+1, err)
		}
	}
	if n := countEvents(t, store, "ing-apply-1"); n != 1 {
		t.Errorf("events with ingest_id = %d, want 1", n)
	}
}

func TestIngestApply_NoSlugIsUnresolvable(t *testing.T) {
	a, store := newTestApplier(t)
	rec := applyRecord("ing-apply-2", "", `{"name":"pageview","url":"https://example.com/"}`)

	err := a.apply(context.Background(), rec, false)
	if !errors.Is(err, domain.ErrProjectUnresolvable) {
		t.Fatalf("err = %v, want ErrProjectUnresolvable", err)
	}
	if n := countEvents(t, store, "ing-apply-2"); n != 0 {
		t.Errorf("events with ingest_id = %d, want 0", n)
	}
}

func TestIngestApply_BadPayloadIsAProcessError(t *testing.T) {
	a, _ := newTestApplier(t)
	rec := applyRecord("ing-apply-3", "apply-site", "")
	rec.BodyBase64 = "not base64 !!"

	err := a.apply(context.Background(), rec, false)
	if !errors.Is(err, errProcessRecord) {
		t.Fatalf("err = %v, want errProcessRecord", err)
	}
	if errors.Is(err, domain.ErrProjectUnresolvable) {
		t.Error("a process error is not an unresolvable project")
	}
	if !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("err = %v, want ErrInvalidPayload", err)
	}
}

// A nameless event can never become an event (#554): the queue consumer must
// mark it permanent, so the bus parks it without retrying.
func TestApplyQueued_InvalidPayloadIsPermanent(t *testing.T) {
	a, _ := newTestApplier(t)
	rec := applyRecord("ing-apply-4", "apply-site", `{"name":"","url":"https://example.com/"}`)

	err := a.applyQueued(context.Background(), rec)
	if !errors.Is(err, command.ErrPermanent) || !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("err = %v, want ErrPermanent wrapping ErrInvalidPayload", err)
	}
}

// The spool worker parks an invalid payload at once instead of retrying it
// maxRetries times.
func TestApplyEntries_InvalidPayloadIsParkedAtOnce(t *testing.T) {
	a, store := newTestApplier(t)
	dir := t.TempDir()
	rec := applyRecord("ing-apply-5", "apply-site", `{"name":"","url":"https://example.com/"}`)
	counts := map[string]int{}

	offset := a.applyEntries(context.Background(), dir, []spool.RecordAtOffset{{Record: rec, EndOffset: 42}}, 0, counts)
	if offset != 42 {
		t.Errorf("offset = %d, want 42 (record skipped)", offset)
	}
	if len(counts) != 0 {
		t.Errorf("retryCounts = %v, want empty", counts)
	}
	if n := countEvents(t, store, "ing-apply-5"); n != 0 {
		t.Errorf("events = %d, want 0", n)
	}
	if _, err := os.Stat(spool.DeadLetterPath(dir)); err != nil {
		t.Errorf("record not parked: %v", err)
	}
}

func TestResolveEventProject_EmptySlugIsUnresolvable(t *testing.T) {
	_, err := resolveEventProject(context.Background(), nil, "")
	if !errors.Is(err, domain.ErrProjectUnresolvable) {
		t.Fatalf("got %v, want ErrProjectUnresolvable", err)
	}
}
