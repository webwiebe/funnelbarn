package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// userEvent inserts one event for a user at a fixed time.
func userEvent(t *testing.T, s *repository.Store, projectID, user, name string, at time.Time, props string) {
	t.Helper()
	if props == "" {
		props = "{}"
	}
	require.NoError(t, s.InsertEvent(context.Background(), repository.Event{
		ID:         randomHex(t),
		ProjectID:  projectID,
		SessionID:  randomHex(t),
		UserIDHash: user,
		Name:       name,
		Properties: props,
		IngestID:   "ingest-" + randomHex(t),
		OccurredAt: at,
	}))
}

func stepCounts(results []repository.FunnelStepResult) []int64 {
	out := make([]int64, len(results))
	for i, r := range results {
		out[i] = r.Count
	}
	return out
}

func TestAnalyzeFunnel_UserScopeOrderedWithinWindow(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Agents", "agents-user-scope")
	require.NoError(t, err)

	day := 24 * time.Hour
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	// complete: all three steps in order, days apart, each in its own session.
	userEvent(t, s, p.ID, "complete", "issue_opened", base, "")
	userEvent(t, s, p.ID, "complete", "pr_opened", base.Add(2*day), "")
	userEvent(t, s, p.ID, "complete", "pr_merged", base.Add(3*day), "")
	// reversed: step 2 fired before step 1, so it reaches only step 1.
	userEvent(t, s, p.ID, "reversed", "pr_opened", base, "")
	userEvent(t, s, p.ID, "reversed", "issue_opened", base.Add(day), "")
	// late: step 2 falls outside the 5-day window of step 1.
	userEvent(t, s, p.ID, "late", "issue_opened", base, "")
	userEvent(t, s, p.ID, "late", "pr_opened", base.Add(6*day), "")
	// restart: the first step 1 is too early, the second one completes.
	userEvent(t, s, p.ID, "restart", "issue_opened", base, "")
	userEvent(t, s, p.ID, "restart", "issue_opened", base.Add(10*day), "")
	userEvent(t, s, p.ID, "restart", "pr_opened", base.Add(11*day), "")
	userEvent(t, s, p.ID, "restart", "pr_merged", base.Add(12*day), "")
	// sametime: steps 1 and 2 share a timestamp, which counts as in order.
	userEvent(t, s, p.ID, "sametime", "issue_opened", base, "")
	userEvent(t, s, p.ID, "sametime", "pr_opened", base, "")
	// anonymous events never enter a user funnel.
	userEvent(t, s, p.ID, "", "issue_opened", base, "")
	userEvent(t, s, p.ID, "", "pr_opened", base.Add(day), "")

	window := int64((5 * day).Seconds())
	f, err := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID:               p.ID,
		Name:                    "Issue to merge",
		Scope:                   repository.FunnelScopeUser,
		ConversionWindowSeconds: &window,
		Steps: []repository.FunnelStep{
			{EventName: "issue_opened"},
			{EventName: "pr_opened"},
			{EventName: "pr_merged"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, repository.FunnelScopeUser, f.Scope)
	require.NotNil(t, f.ConversionWindowSeconds)
	require.Equal(t, window, *f.ConversionWindowSeconds)

	results, err := s.AnalyzeFunnel(ctx, f, base.Add(-day), base.Add(30*day), nil)
	require.NoError(t, err)
	// Step 1: complete, reversed, late, restart, sametime.
	// Step 2: complete, restart, sametime. Step 3: complete, restart.
	require.Equal(t, []int64{5, 3, 2}, stepCounts(results))
	require.InDelta(t, 0.4, results[2].Conversion, 1e-9)

	// The same events as a session funnel count each step on its own.
	sessionFunnel := f
	sessionFunnel.Scope = repository.FunnelScopeSession
	results, err = s.AnalyzeFunnel(ctx, sessionFunnel, base.Add(-day), base.Add(30*day), nil)
	require.NoError(t, err)
	require.Equal(t, []int64{7, 6, 2}, stepCounts(results))
}

func TestAnalyzeFunnel_UserScopeDefaultWindowAndFilters(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Agents", "agents-user-window")
	require.NoError(t, err)

	day := 24 * time.Hour
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	// Within the 30-day default, step 2 matching the filter.
	userEvent(t, s, p.ID, "a", "issue_opened", base, "")
	userEvent(t, s, p.ID, "a", "deployment_status", base.Add(29*day), `{"environment":"production"}`)
	// Past the default window.
	userEvent(t, s, p.ID, "b", "issue_opened", base, "")
	userEvent(t, s, p.ID, "b", "deployment_status", base.Add(31*day), `{"environment":"production"}`)
	// In time, but step 2 fails the property filter.
	userEvent(t, s, p.ID, "c", "issue_opened", base, "")
	userEvent(t, s, p.ID, "c", "deployment_status", base.Add(day), `{"environment":"staging"}`)

	f, err := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Issue to production",
		Scope:     repository.FunnelScopeUser,
		Steps: []repository.FunnelStep{
			{EventName: "issue_opened"},
			{EventName: "deployment_status", Filters: []repository.FunnelFilter{{Property: "environment", Value: "production"}}},
		},
	})
	require.NoError(t, err)
	require.Nil(t, f.ConversionWindowSeconds)
	require.Equal(t, repository.DefaultConversionWindow, f.ConversionWindow())

	results, err := s.AnalyzeFunnel(ctx, f, base, base.Add(60*day), nil)
	require.NoError(t, err)
	require.Equal(t, []int64{3, 1}, stepCounts(results))
}

func TestFunnelConversionWindowRoundTrips(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Agents", "agents-window-roundtrip")
	require.NoError(t, err)

	window := int64(3600)
	f, err := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID:               p.ID,
		Name:                    "Hourly",
		Scope:                   repository.FunnelScopeUser,
		ConversionWindowSeconds: &window,
		Steps:                   []repository.FunnelStep{{EventName: "a"}, {EventName: "b"}},
	})
	require.NoError(t, err)

	listed, err := s.ListFunnels(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, window, *listed[0].ConversionWindowSeconds)

	f.ConversionWindowSeconds = nil
	f.Scope = repository.FunnelScopeSession
	updated, err := s.UpdateFunnel(ctx, f)
	require.NoError(t, err)
	require.Nil(t, updated.ConversionWindowSeconds)
	require.Equal(t, repository.FunnelScopeSession, updated.Scope)
}
