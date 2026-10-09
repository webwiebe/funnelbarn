package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func persistFixture(projectID, ingestID, sessionID string) (repository.Event, repository.Session) {
	now := time.Now().UTC().Truncate(time.Second)
	e := repository.Event{
		ID:         "evt-" + ingestID,
		ProjectID:  projectID,
		SessionID:  sessionID,
		Name:       "pageview",
		URL:        "https://example.com/",
		IngestID:   ingestID,
		OccurredAt: now,
	}
	sess := repository.Session{
		ID:          sessionID,
		ProjectID:   projectID,
		FirstSeenAt: now,
		LastSeenAt:  now,
		EntryURL:    e.URL,
		ExitURL:     e.URL,
	}
	return e, sess
}

func countEventsByIngestID(t *testing.T, s *repository.Store, ingestID string) int {
	t.Helper()
	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE ingest_id = ?`, ingestID).Scan(&n))
	return n
}

// A queue redelivers on a crash between apply and ack. The replay must store
// nothing: one event row, and the session counted once.
func TestPersistEvent_ReplayIsANoOp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Replay", "replay")
	require.NoError(t, err)

	e, sess := persistFixture(p.ID, "ingest-replay", "sess-replay")
	width := 1440
	signals := &repository.SessionSignals{ScreenWidth: &width}

	inserted, err := s.PersistEvent(ctx, e, sess, signals)
	require.NoError(t, err)
	require.True(t, inserted)

	e.ID = "evt-replay-second-id" // a replay may carry a fresh event id
	inserted, err = s.PersistEvent(ctx, e, sess, signals)
	require.NoError(t, err)
	require.False(t, inserted)

	require.Equal(t, 1, countEventsByIngestID(t, s, "ingest-replay"))
	got, err := s.SessionByID(ctx, p.ID, "sess-replay")
	require.NoError(t, err)
	require.Equal(t, 1, got.EventCount)

	var storedWidth int
	require.NoError(t, s.DB().QueryRow(`SELECT screen_width FROM sessions WHERE project_id = ? AND id = ?`, p.ID, "sess-replay").Scan(&storedWidth))
	require.Equal(t, 1440, storedWidth)

	dups, err := s.CountDuplicateIngestIDs(ctx)
	require.NoError(t, err)
	require.Zero(t, dups)
}

// Two distinct events in one session both count.
func TestPersistEvent_SecondEventCountsSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Two", "two-events")
	require.NoError(t, err)

	e1, sess := persistFixture(p.ID, "ingest-1", "sess-1")
	e2, _ := persistFixture(p.ID, "ingest-2", "sess-1")
	for _, e := range []repository.Event{e1, e2} {
		inserted, err := s.PersistEvent(ctx, e, sess, nil)
		require.NoError(t, err)
		require.True(t, inserted)
	}

	got, err := s.SessionByID(ctx, p.ID, "sess-1")
	require.NoError(t, err)
	require.Equal(t, 2, got.EventCount)
}

// A failure after the event insert rolls the insert back, so the retry stores
// the event once. Before, the event stayed and the session update was lost.
func TestPersistEvent_FailureStoresNothing(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Rollback", "rollback")
	require.NoError(t, err)

	e, sess := persistFixture(p.ID, "ingest-rollback", "sess-rollback")
	bad := sess
	bad.ProjectID = "no-such-project" // fails the sessions.project_id foreign key

	_, err = s.PersistEvent(ctx, e, bad, nil)
	require.Error(t, err)
	require.Equal(t, 0, countEventsByIngestID(t, s, "ingest-rollback"))

	inserted, err := s.PersistEvent(ctx, e, sess, nil)
	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, 1, countEventsByIngestID(t, s, "ingest-rollback"))
}

// Rows duplicated before the guard existed are counted and left in place.
func TestCountDuplicateIngestIDs(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Dups", "dups")
	require.NoError(t, err)

	for _, id := range []string{"a", "b"} {
		_, err := s.DB().Exec(`INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at) VALUES (?, ?, 's', 'pv', 'legacy-dup', ?)`,
			"evt-legacy-"+id, p.ID, time.Now().UTC())
		require.NoError(t, err)
	}

	dups, err := s.CountDuplicateIngestIDs(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, dups)

	e, sess := persistFixture(p.ID, "legacy-dup", "s")
	inserted, err := s.PersistEvent(ctx, e, sess, nil)
	require.NoError(t, err)
	require.False(t, inserted)
	require.Equal(t, 2, countEventsByIngestID(t, s, "legacy-dup"))
}
