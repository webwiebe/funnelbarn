package worker

// PersistEvent tests use an in-memory fake that satisfies EventPersister.
// This validates the idempotency logic and session upsert without SQLite.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/geoip"
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

func (f *fakeEventStore) UpsertSessionSignals(_ context.Context, _, _ string, _ repository.SessionSignals) error {
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

	if err := PersistEvent(context.Background(), store, event, nil); err != nil {
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
	if err := PersistEvent(context.Background(), store, event, nil); err != nil {
		t.Fatalf("first PersistEvent: %v", err)
	}
	// Second call: same ingest_id → must be skipped.
	if err := PersistEvent(context.Background(), store, event, nil); err != nil {
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

// An event with no project ID can only fail the events.project_id foreign key,
// which SQLite reports as an unattributable "FOREIGN KEY constraint failed
// (787)". PersistEvent names the cause before the insert is attempted.
func TestPersistEvent_RejectsEventWithoutProject(t *testing.T) {
	store := &fakeEventStore{}
	event := repository.Event{
		ID:         "evt-1",
		SessionID:  "sess-1",
		Name:       "signup",
		IngestID:   "ingest-1",
		OccurredAt: time.Now(),
	}

	err := PersistEvent(context.Background(), store, event, nil)
	if err == nil {
		t.Fatal("expected an error for an event with no project ID")
	}
	if !errors.Is(err, ErrNoProject) {
		t.Errorf("expected ErrNoProject, got %v", err)
	}
	if !strings.Contains(err.Error(), "ingest-1") {
		t.Errorf("error should name the ingest ID so it is traceable, got %q", err)
	}
	if len(store.events) != 0 {
		t.Errorf("expected no insert attempt, got %d events", len(store.events))
	}
}

// The geo lookup used to land only on the session row, leaving
// events.country_code empty. The country widgets group events, so they read
// empty on every project until the event carries the code too.
func TestPersistEvent_CopiesGeoCountryOntoEvent(t *testing.T) {
	store := &fakeEventStore{}
	event := repository.Event{
		ID:         "evt-geo",
		ProjectID:  "proj-geo",
		SessionID:  "sess-geo",
		Name:       "pageview",
		IngestID:   "ingest-geo",
		OccurredAt: time.Now().UTC(),
	}

	err := PersistEvent(context.Background(), store, event, &geoip.GeoResult{CountryCode: "NL", City: "Nijmegen"})
	if err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	if len(store.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(store.events))
	}
	if store.events[0].CountryCode != "NL" {
		t.Errorf("event country_code: want NL, got %q", store.events[0].CountryCode)
	}
	if store.sessions[0].CountryCode != "NL" {
		t.Errorf("session country_code: want NL, got %q", store.sessions[0].CountryCode)
	}
}

// A lookup that resolved nothing must not blank a country the payload already
// carried.
func TestPersistEvent_EmptyGeoLookupKeepsExistingCountry(t *testing.T) {
	store := &fakeEventStore{}
	event := repository.Event{
		ID:          "evt-geo-keep",
		ProjectID:   "proj-geo",
		SessionID:   "sess-geo-keep",
		Name:        "pageview",
		IngestID:    "ingest-geo-keep",
		CountryCode: "DE",
		OccurredAt:  time.Now().UTC(),
	}

	if err := PersistEvent(context.Background(), store, event, &geoip.GeoResult{}); err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	if store.events[0].CountryCode != "DE" {
		t.Errorf("event country_code: want DE, got %q", store.events[0].CountryCode)
	}
}

// No geo lookup at all (collection disabled) leaves the event untouched.
func TestPersistEvent_NilGeoLeavesCountryUnset(t *testing.T) {
	store := &fakeEventStore{}
	event := repository.Event{
		ID:         "evt-geo-nil",
		ProjectID:  "proj-geo",
		SessionID:  "sess-geo-nil",
		Name:       "pageview",
		IngestID:   "ingest-geo-nil",
		OccurredAt: time.Now().UTC(),
	}

	if err := PersistEvent(context.Background(), store, event, nil); err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	if store.events[0].CountryCode != "" {
		t.Errorf("event country_code: want empty, got %q", store.events[0].CountryCode)
	}
}
