package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func envEvent(t *testing.T, s *repository.Store, projectID, session, user, env, name, browser string, at time.Time) {
	t.Helper()
	require.NoError(t, s.InsertEvent(context.Background(), repository.Event{
		ID:          randomHex(t),
		ProjectID:   projectID,
		SessionID:   session,
		UserIDHash:  user,
		Name:        name,
		Browser:     browser,
		Environment: env,
		Properties:  "{}",
		IngestID:    "ingest-" + randomHex(t),
		OccurredAt:  at,
	}))
}

func TestEnvironmentRule(t *testing.T) {
	require.Nil(t, repository.EnvironmentRule(""))
	require.Nil(t, repository.EnvironmentRule("  "))
	require.Equal(t,
		[]repository.SegmentRule{{Field: "environment", Operator: "eq", Value: "production"}},
		repository.EnvironmentRule("prod"))
	require.Equal(t, "test", repository.EnvironmentRule("QA")[0].Value)
}

// The environment argument must not become a stored-segment field.
func TestEnvironmentIsNotAnAllowedSegmentField(t *testing.T) {
	_, ok := repository.AllowedSegmentFields["environment"]
	require.False(t, ok)
}

func TestAnalyzeFunnel_EnvironmentArgument(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Env", "env-funnel")
	require.NoError(t, err)

	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	// Real visitors: 3 sessions view, 2 book. One of them uses Firefox.
	envEvent(t, s, p.ID, "prod-1", "u-prod-1", "production", "page_view", "Chrome", at)
	envEvent(t, s, p.ID, "prod-1", "u-prod-1", "production", "booking", "Chrome", at.Add(time.Minute))
	envEvent(t, s, p.ID, "prod-2", "u-prod-2", "production", "page_view", "Firefox", at)
	envEvent(t, s, p.ID, "prod-2", "u-prod-2", "production", "booking", "Firefox", at.Add(time.Minute))
	envEvent(t, s, p.ID, "prod-3", "u-prod-3", "production", "page_view", "Chrome", at)
	// E2E and staging traffic that used to pollute the funnel.
	for _, sess := range []string{"stg-1", "stg-2", "stg-3", "stg-4"} {
		envEvent(t, s, p.ID, sess, "u-"+sess, "staging", "page_view", "Chrome", at)
		envEvent(t, s, p.ID, sess, "u-"+sess, "staging", "booking", "Chrome", at.Add(time.Minute))
	}
	envEvent(t, s, p.ID, "test-1", "u-test-1", "test", "page_view", "Chrome", at)
	// Rows recorded before environments existed.
	envEvent(t, s, p.ID, "old-1", "u-old-1", "", "page_view", "Chrome", at)

	steps := []repository.FunnelStep{{EventName: "page_view"}, {EventName: "booking"}}
	from, to := at.Add(-time.Hour), at.Add(time.Hour)
	for _, scope := range []string{repository.FunnelScopeSession, repository.FunnelScopeUser} {
		f, err := s.CreateFunnel(ctx, repository.Funnel{ProjectID: p.ID, Name: "Booking " + scope, Scope: scope, Steps: steps})
		require.NoError(t, err)

		all, err := s.AnalyzeFunnel(ctx, f, from, to, nil)
		require.NoError(t, err)
		require.Equal(t, []int64{9, 6}, stepCounts(all), "%s: no environment keeps every environment", scope)

		prod, err := s.AnalyzeFunnel(ctx, f, from, to, nil, repository.EnvironmentRule("production")...)
		require.NoError(t, err)
		require.Equal(t, []int64{3, 2}, stepCounts(prod), "%s: production only", scope)

		alias, err := s.AnalyzeFunnel(ctx, f, from, to, nil, repository.EnvironmentRule("prod")...)
		require.NoError(t, err)
		require.Equal(t, []int64{3, 2}, stepCounts(alias), "%s: alias is normalised", scope)

		stg, err := s.AnalyzeFunnel(ctx, f, from, to, nil, repository.EnvironmentRule("staging")...)
		require.NoError(t, err)
		require.Equal(t, []int64{4, 4}, stepCounts(stg), "%s: staging only", scope)

		// Combined with a stored segment rule, both conditions apply.
		rules := append([]repository.SegmentRule{{Field: "browser", Operator: "eq", Value: "Chrome"}},
			repository.EnvironmentRule("production")...)
		chromeProd, err := s.AnalyzeFunnel(ctx, f, from, to, nil, rules...)
		require.NoError(t, err)
		require.Equal(t, []int64{2, 1}, stepCounts(chromeProd), "%s: segment and environment are ANDed", scope)
	}
}
