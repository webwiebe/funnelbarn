package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func breakdownEvent(t *testing.T, s *repository.Store, projectID, session, user, env, name, props string, at time.Time) {
	t.Helper()
	require.NoError(t, s.InsertEvent(context.Background(), repository.Event{
		ID: randomHex(t), ProjectID: projectID, SessionID: session, UserIDHash: user,
		Name: name, Environment: env, Properties: props,
		IngestID: "ingest-" + randomHex(t), OccurredAt: at,
	}))
}

func groupCounts(groups []repository.FunnelBreakdownGroup) map[string][]int64 {
	out := map[string][]int64{}
	for _, g := range groups {
		out[g.Label] = stepCounts(g.Results)
	}
	return out
}

func TestAnalyzeFunnelBreakdown(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Surface", "surface")
	require.NoError(t, err)
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	from, to := at.Add(-time.Hour), at.Add(time.Hour)

	// Three surfaces start a visit; two of the wordpress visits and one
	// lightroom visit sign up. One visit has no surface, one is staging.
	visit := func(session, user, env, surface string, signup bool) {
		props := "{}"
		if surface != "" {
			props = fmt.Sprintf(`{"surface":%q}`, surface)
		}
		breakdownEvent(t, s, p.ID, session, user, env, "visit", props, at)
		if signup {
			breakdownEvent(t, s, p.ID, session, user, env, "signup", "{}", at.Add(time.Minute))
		}
	}
	visit("s1", "u1", "production", "wordpress", true)
	visit("s2", "u2", "production", "wordpress", true)
	visit("s3", "u3", "production", "wordpress", false)
	visit("s4", "u4", "production", "lightroom", true)
	visit("s5", "u5", "production", "", false)
	visit("s6", "u6", "staging", "wordpress", true)

	steps := []repository.FunnelStep{{EventName: "visit"}, {EventName: "signup"}}
	for _, scope := range []string{repository.FunnelScopeSession, repository.FunnelScopeUser} {
		f, err := s.CreateFunnel(ctx, repository.Funnel{ProjectID: p.ID, Name: "F " + scope, Scope: scope, Steps: steps})
		require.NoError(t, err)

		groups, err := s.AnalyzeFunnelBreakdown(ctx, f, from, to, nil, "surface")
		require.NoError(t, err)
		require.Equal(t, map[string][]int64{
			"wordpress": {4, 3}, "lightroom": {1, 1}, "(none)": {1, 0},
		}, groupCounts(groups), scope)
		require.Equal(t, "wordpress", groups[0].Value, "largest group first")
		require.Equal(t, int64(4), groups[0].Entries)
		require.InDelta(t, 0.75, groups[0].Results[1].Conversion, 1e-9)

		// The groups add up to the unbroken funnel.
		whole, err := s.AnalyzeFunnel(ctx, f, from, to, nil)
		require.NoError(t, err)
		var sum [2]int64
		for _, g := range groups {
			for i, r := range g.Results {
				sum[i] += r.Count
			}
		}
		require.Equal(t, stepCounts(whole), sum[:], scope)

		// Environment narrows the groups.
		prod, err := s.AnalyzeFunnelBreakdown(ctx, f, from, to, nil, "surface", repository.EnvironmentRule("production")...)
		require.NoError(t, err)
		require.Equal(t, map[string][]int64{
			"wordpress": {3, 2}, "lightroom": {1, 1}, "(none)": {1, 0},
		}, groupCounts(prod), scope)
	}
}

func TestAnalyzeFunnelBreakdown_CapsGroups(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Cap", "cap")
	require.NoError(t, err)
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	total := repository.MaxBreakdownGroups + 5
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("s%02d", i)
		breakdownEvent(t, s, p.ID, id, "u"+id, "production", "visit", fmt.Sprintf(`{"surface":"v%02d"}`, i), at)
	}
	f, err := s.CreateFunnel(ctx, repository.Funnel{ProjectID: p.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "visit"}}})
	require.NoError(t, err)

	groups, err := s.AnalyzeFunnelBreakdown(ctx, f, at.Add(-time.Hour), at.Add(time.Hour), nil, "surface")
	require.NoError(t, err)
	require.Len(t, groups, repository.MaxBreakdownGroups+1)
	last := groups[len(groups)-1]
	require.True(t, last.Other)
	require.Equal(t, "(other)", last.Label)
	require.Equal(t, int64(5), last.Entries)
}

func TestAnalyzeFunnelBreakdown_RejectsBadProperty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Bad", "bad")
	require.NoError(t, err)
	f, err := s.CreateFunnel(ctx, repository.Funnel{ProjectID: p.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "visit"}}})
	require.NoError(t, err)
	_, err = s.AnalyzeFunnelBreakdown(ctx, f, time.Now().Add(-time.Hour), time.Now(), nil, "a') OR 1=1 --")
	require.Error(t, err)
}
