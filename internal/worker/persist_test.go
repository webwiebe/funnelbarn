package worker

// PersistEvent tests use an in-memory fake that satisfies EventPersister.
// This validates the idempotency logic and session upsert without SQLite.

import (
	"context"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// fakeEventStore is an in-memory implementation of EventPersister.
type fakeEventStore struct {
	events   []repository.Event
	sessions []repository.Session
}

func (f *fakeEventStore) GetEventByIngestID(_ context.Context, ingestID string) (*repository.Event, error) {
	for i := range f.events {
		if f.events[i].IngestID == ingestID {
			return &f.events[i], nil
		}
	}
	return nil, nil
}

func (f *fakeEventStore) InsertEvent(_ context.Context, e repository.Event) error {
	f.events = append(f.events, e)
	return nil
}

func (f *fakeEventStore) UpsertSession(_ context.Context, sess repository.Session) error {
	f.sessions = append(f.sessions, sess)
	return nil
}

func TestPersistEvent_InsertsEventAndUpsertSession(t *testing.T) {
	store := &fakeEventStore{}
	event := repository.Event{
		ID:         "evt-1",
		ProjectID:  "proj-1",
		SessionID:  "sess-1",
		Name:       "signup",
		IngestID:   "ingest-1",
		OccurredAt: time.Now().UTC(),
	}

	if err := PersistEvent(context.Background(), store, event); err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	if len(store.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(store.events))
	}
	if len(store.sessions) != 1 {
		t.Fatalf("want 1 session upsert, got %d", len(store.sessions))
	}
	if store.sessions[0].ID != "sess-1" {
		t.Errorf("session ID: want sess-1, got %q", store.sessions[0].ID)
	}
}

func TestPersistEvent_Idempotency(t *testing.T) {
	store := &fakeEventStore{}
	event := repository.Event{
		ID:         "evt-dup",
		ProjectID:  "proj-dup",
		SessionID:  "sess-dup",
		Name:       "purchase",
		IngestID:   "ingest-dup",
		OccurredAt: time.Now().UTC(),
	}

	// First call: inserts.
	if err := PersistEvent(context.Background(), store, event); err != nil {
		t.Fatalf("first PersistEvent: %v", err)
	}
	// Second call: same ingest_id → must be skipped.
	if err := PersistEvent(context.Background(), store, event); err != nil {
		t.Fatalf("second PersistEvent: %v", err)
	}

	if len(store.events) != 1 {
		t.Errorf("idempotency: want 1 event, got %d", len(store.events))
	}
	// Session upsert should still happen on the first call only.
	if len(store.sessions) != 1 {
		t.Errorf("idempotency: want 1 session upsert, got %d", len(store.sessions))
	}
}
