package main

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/workerhealth"
)

func newTestApplier(t *testing.T) (*ingestApplier, *repository.Store) {
	t.Helper()
	store, err := repository.Open(filepath.Join(t.TempDir(), "apply.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return &ingestApplier{store: store, health: workerhealth.New(workerhealth.Options{})}, store
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
	if !errors.Is(err, repository.ErrProjectUnresolvable) {
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
	if errors.Is(err, repository.ErrProjectUnresolvable) {
		t.Error("a process error must be retried, not dead-lettered as unresolvable")
	}
}
