package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
	"github.com/wiebe-xyz/funnelbarn/internal/volumedrop"
)

func TestEnvironmentEventsByHour(t *testing.T) {
	ctx := context.Background()
	s := repotest.Open(t)
	p, err := s.CreateProject(ctx, "Shop", "shop")
	require.NoError(t, err)

	// The client's occurred_at sits a day off, and the count must not follow it.
	insert := func(id, env string, stored time.Time) {
		t.Helper()
		_, err := s.DB().ExecContext(ctx,
			`INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, created_at, environment) VALUES (?, ?, 's1', 'page_view', ?, ?, ?, ?)`,
			id, p.ID, id, stored.Add(-24*time.Hour), stored.Format(time.DateTime), env)
		require.NoError(t, err)
	}
	since := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	insert("before-since", "production", since.Add(-time.Minute))
	insert("first-a", "production", since)
	insert("first-b", "production", since.Add(59*time.Minute))
	insert("second", "production", since.Add(time.Hour))
	insert("other-env", "test", since)

	got, err := s.Reader().EnvironmentEventsByHour(ctx, "production", since)

	require.NoError(t, err)
	require.ElementsMatch(t, []repository.ProjectHourCount{
		{Project: "shop", Hour: "2026-10-30 10", Count: 2},
		{Project: "shop", Hour: "2026-10-30 11", Count: 1},
	}, got)
}

// A row through the production insert gets created_at from the column default.
// The hour key must match volumedrop.HourFormat, or every project reads as empty
// and looks like a drop.
func TestEnvironmentEventsByHour_ReadsTheProductionInsert(t *testing.T) {
	ctx := context.Background()
	s := repotest.Open(t)
	p, err := s.CreateProject(ctx, "Shop", "shop")
	require.NoError(t, err)

	before := time.Now().UTC()
	require.NoError(t, s.InsertEvent(ctx, repository.Event{
		ID: "e1", ProjectID: p.ID, SessionID: "s1", Name: "page_view",
		IngestID: "ing1", OccurredAt: before.Add(-24 * time.Hour), Environment: "production",
	}))
	after := time.Now().UTC()

	got, err := s.Reader().EnvironmentEventsByHour(ctx, "production", before.Add(-time.Hour))

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Contains(t, []string{before.Format(volumedrop.HourFormat), after.Format(volumedrop.HourFormat)}, got[0].Hour)
	require.EqualValues(t, 1, got[0].Count)
}
