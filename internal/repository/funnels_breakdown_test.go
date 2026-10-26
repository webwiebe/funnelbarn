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

// requireGroupsAddUp checks that the groups sum to the unbroken funnel run with
// the same filters, so every entry landed in exactly one group.
func requireGroupsAddUp(t *testing.T, s *repository.Store, f repository.Funnel, from, to time.Time, seg *repository.SegmentFilter, groups []repository.FunnelBreakdownGroup, rules ...repository.SegmentRule) {
	t.Helper()
	whole, err := s.AnalyzeFunnel(context.Background(), f, from, to, seg, rules...)
	require.NoError(t, err)
	sum := make([]int64, len(f.Steps))
	for _, g := range groups {
		for i, r := range g.Results {
			sum[i] += r.Count
		}
	}
	require.Equal(t, stepCounts(whole), sum)
}

func TestAnalyzeFunnelBreakdown_PageViewScope(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Page views", "page-views")
	require.NoError(t, err)
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	from, to := at.Add(-time.Hour), at.Add(time.Hour)

	// One session, four page views. Each page view is its own entry, grouped
	// by the surface on its visit. The visit without a page view id is outside
	// a page view funnel.
	insert := func(pageView, name, props string) {
		require.NoError(t, s.InsertEvent(ctx, repository.Event{
			ID: randomHex(t), ProjectID: p.ID, SessionID: "s1", PageViewID: pageView,
			Name: name, Environment: "production", Properties: props,
			IngestID: "ingest-" + randomHex(t), OccurredAt: at,
		}))
	}
	insert("pv1", "visit", `{"surface":"wordpress"}`)
	insert("pv1", "signup", "{}")
	insert("pv2", "visit", `{"surface":"wordpress"}`)
	insert("pv3", "visit", `{"surface":"lightroom"}`)
	insert("pv3", "signup", "{}")
	insert("pv4", "visit", "{}")
	insert("", "visit", `{"surface":"wordpress"}`)

	f, err := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID, Name: "F", Scope: repository.FunnelScopePageView,
		Steps: []repository.FunnelStep{{EventName: "visit"}, {EventName: "signup"}},
	})
	require.NoError(t, err)

	groups, err := s.AnalyzeFunnelBreakdown(ctx, f, from, to, nil, "surface")
	require.NoError(t, err)
	require.Equal(t, map[string][]int64{
		"wordpress": {2, 1}, "lightroom": {1, 1}, "(none)": {1, 0},
	}, groupCounts(groups))
	requireGroupsAddUp(t, s, f, from, to, nil, groups)
}

func TestAnalyzeFunnelBreakdown_WithSegments(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Segments", "segments")
	require.NoError(t, err)
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	from, to := at.Add(-time.Hour), at.Add(time.Hour)

	visit := func(session, env, device, surface string, signup bool) {
		insert := func(name, props string, when time.Time) {
			require.NoError(t, s.InsertEvent(ctx, repository.Event{
				ID: randomHex(t), ProjectID: p.ID, SessionID: session, UserIDHash: "u-" + session,
				Name: name, Environment: env, DeviceType: device, Properties: props,
				IngestID: "ingest-" + randomHex(t), OccurredAt: when,
			}))
		}
		insert("visit", fmt.Sprintf(`{"surface":%q}`, surface), at)
		if signup {
			insert("signup", "{}", at.Add(time.Minute))
		}
	}
	visit("s1", "production", "mobile", "wordpress", true)
	visit("s2", "production", "desktop", "wordpress", true)
	visit("s3", "production", "mobile", "lightroom", false)
	visit("s4", "staging", "mobile", "wordpress", true)
	visit("s5", "production", "mobile", "wordpress", false)

	mobile := &repository.SegmentFilter{Field: "device_type", Op: "eq", Value: "mobile"}
	desktopRule := repository.SegmentRule{Field: "device_type", Operator: "eq", Value: "desktop"}
	production := repository.EnvironmentRule("production")

	cases := []struct {
		name  string
		seg   *repository.SegmentFilter
		rules []repository.SegmentRule
		want  map[string][]int64
	}{
		{"preset segment", mobile, nil, map[string][]int64{"wordpress": {3, 2}, "lightroom": {1, 0}}},
		{"preset segment and environment", mobile, production, map[string][]int64{"wordpress": {2, 1}, "lightroom": {1, 0}}},
		{"stored segment and environment", nil, append([]repository.SegmentRule{desktopRule}, production...), map[string][]int64{"wordpress": {1, 1}}},
	}
	for _, scope := range []string{repository.FunnelScopeSession, repository.FunnelScopeUser} {
		f, err := s.CreateFunnel(ctx, repository.Funnel{
			ProjectID: p.ID, Name: "F " + scope, Scope: scope,
			Steps: []repository.FunnelStep{{EventName: "visit"}, {EventName: "signup"}},
		})
		require.NoError(t, err)
		for _, tc := range cases {
			t.Run(scope+"/"+tc.name, func(t *testing.T) {
				groups, err := s.AnalyzeFunnelBreakdown(ctx, f, from, to, tc.seg, "surface", tc.rules...)
				require.NoError(t, err)
				require.Equal(t, tc.want, groupCounts(groups))
				requireGroupsAddUp(t, s, f, from, to, tc.seg, groups, tc.rules...)
			})
		}
	}
}
