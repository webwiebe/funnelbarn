package repository_test

import (
	"context"
	"fmt"
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

// The rowid bound must count what a filter on created_at alone counts, for any
// since: before every row, after every row, on a row and between rows. Retention
// deletes the oldest rows and a few in the middle leave gaps in rowid.
func TestEnvironmentEventsByHour_BoundMatchesTheFullRead(t *testing.T) {
	ctx := context.Background()
	s := repotest.Open(t)
	p, err := s.CreateProject(ctx, "Shop", "shop")
	require.NoError(t, err)

	base := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	for i := range 300 {
		_, err := s.DB().ExecContext(ctx,
			`INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, created_at, environment) VALUES (?, ?, 's1', 'page_view', ?, ?, ?, 'production')`,
			fmt.Sprint("e", i), p.ID, fmt.Sprint("i", i), base, base.Add(time.Duration(i)*17*time.Minute).Format(time.DateTime))
		require.NoError(t, err)
	}
	_, err = s.DB().ExecContext(ctx, `DELETE FROM events WHERE rowid <= 40 OR rowid BETWEEN 150 AND 170`)
	require.NoError(t, err)

	full := func(since time.Time) map[string]int64 {
		rows, err := s.DB().QueryContext(ctx,
			`SELECT substr(created_at, 1, 13), COUNT(*) FROM events WHERE created_at >= ? AND environment = 'production' GROUP BY 1`,
			since.Format(time.DateTime))
		require.NoError(t, err)
		defer rows.Close()
		out := map[string]int64{}
		for rows.Next() {
			var h string
			var n int64
			require.NoError(t, rows.Scan(&h, &n))
			out[h] = n
		}
		require.NoError(t, rows.Err())
		return out
	}
	for _, since := range []time.Time{
		base.Add(-time.Hour),
		base,
		base.Add(40 * 17 * time.Minute),
		base.Add(41*17*time.Minute - time.Second),
		base.Add(160 * 17 * time.Minute),
		base.Add(200*17*time.Minute + time.Minute),
		base.Add(299 * 17 * time.Minute),
		base.Add(300 * 17 * time.Minute),
	} {
		got, err := s.Reader().EnvironmentEventsByHour(ctx, "production", since)
		require.NoError(t, err)
		counted := map[string]int64{}
		for _, c := range got {
			counted[c.Hour] = c.Count
		}
		require.Equal(t, full(since), counted, "since %s", since)
	}
}

func TestEnvironmentEventsByHour_EmptyTable(t *testing.T) {
	got, err := repotest.Open(t).Reader().EnvironmentEventsByHour(context.Background(), "production", time.Now())
	require.NoError(t, err)
	require.Empty(t, got)
}
