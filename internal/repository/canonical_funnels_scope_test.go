package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func canonEvent(t *testing.T, s *repository.Store, projectID, session, pageView, env, name string, at time.Time) {
	t.Helper()
	require.NoError(t, s.InsertEvent(context.Background(), domain.Event{
		ID: randomHex(t), ProjectID: projectID, SessionID: session, PageViewID: pageView,
		Name: name, Environment: env, Properties: "{}", IngestID: "ingest-" + randomHex(t), OccurredAt: at,
	}))
}

// Pins what the overview test leaves open before AnalyzeCanonicalFunnel is
// split: the page_view scope, a project subset, the environment rule (which
// forces the sessions join), the exclusion reason and an empty funnel.
func TestAnalyzeCanonicalFunnel_ScopeSubsetEnvironmentAndExclusion(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	at := time.Now().UTC()
	from, to := at.Add(-time.Hour), at.Add(time.Hour)

	a, err := s.CreateProject(ctx, "A", "scope-a")
	require.NoError(t, err)
	b, err := s.CreateProject(ctx, "B", "scope-b")
	require.NoError(t, err)
	c, err := s.CreateProject(ctx, "C", "scope-c")
	require.NoError(t, err)
	for _, p := range []string{a.ID, b.ID} {
		require.NoError(t, s.UpsertMapping(ctx, p, "view", "page_view"))
		require.NoError(t, s.UpsertMapping(ctx, p, "buy", "purchase"))
	}
	require.NoError(t, s.UpsertMapping(ctx, c.ID, "view", "page_view")) // no purchase mapping

	// A: session s1 has two page views (pv1, pv2), one purchase on pv1; the
	// staging session s2 views once. Events without a page_view_id are ignored
	// by the page_view scope.
	canonEvent(t, s, a.ID, "s1", "pv1", "production", "view", at)
	canonEvent(t, s, a.ID, "s1", "pv2", "production", "view", at)
	canonEvent(t, s, a.ID, "s1", "pv1", "production", "buy", at)
	canonEvent(t, s, a.ID, "s2", "pv3", "staging", "view", at)
	canonEvent(t, s, a.ID, "s3", "", "production", "view", at)
	// B: one session, one view, no purchase.
	canonEvent(t, s, b.ID, "b1", "bpv1", "production", "view", at)
	canonEvent(t, s, c.ID, "c1", "cpv1", "production", "view", at)

	steps := []domain.CanonicalFunnelStep{
		{StepOrder: 1, CanonicalKey: "page_view"},
		{StepOrder: 2, CanonicalKey: "purchase"},
	}
	counts := func(res domain.CanonicalFunnelResult) []int64 {
		out := make([]int64, len(res.Steps))
		for i, st := range res.Steps {
			out[i] = st.Count
		}
		return out
	}

	sess, err := s.AnalyzeCanonicalFunnel(ctx, domain.CanonicalFunnel{Scope: "session", Steps: steps}, nil, from, to, nil)
	require.NoError(t, err)
	require.Equal(t, []int64{4, 1}, counts(sess), "session scope: s1,s2,s3 in A plus b1; one purchase")

	pv, err := s.AnalyzeCanonicalFunnel(ctx, domain.CanonicalFunnel{Scope: "page_view", Steps: steps}, nil, from, to, nil)
	require.NoError(t, err)
	require.Equal(t, []int64{4, 1}, counts(pv), "page_view scope: pv1,pv2,pv3 in A plus bpv1; s3 has no page view id")

	empty, err := s.AnalyzeCanonicalFunnel(ctx, domain.CanonicalFunnel{Scope: ""}, nil, from, to, nil)
	require.NoError(t, err)
	require.Empty(t, empty.Steps)
	require.Empty(t, empty.ByProject)

	// Empty scope defaults to session.
	def, err := s.AnalyzeCanonicalFunnel(ctx, domain.CanonicalFunnel{Steps: steps}, nil, from, to, nil)
	require.NoError(t, err)
	require.Equal(t, counts(sess), counts(def))

	// A project subset keeps only the named projects.
	only, err := s.AnalyzeCanonicalFunnel(ctx, domain.CanonicalFunnel{Scope: "session", Steps: steps}, []string{b.ID}, from, to, nil)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 0}, counts(only))
	require.Len(t, only.ByProject, 1)
	require.Equal(t, "B", only.ByProject[0].ProjectName)

	// The environment rule narrows every step (and forces the sessions join).
	prod, err := s.AnalyzeCanonicalFunnel(ctx, domain.CanonicalFunnel{Scope: "session", Steps: steps}, nil, from, to, nil,
		domain.EnvironmentRule("production")...)
	require.NoError(t, err)
	require.Equal(t, []int64{3, 1}, counts(prod), "staging session s2 is out")

	// C lacks the purchase mapping: excluded, with the key in the reason.
	require.Len(t, sess.ExcludedProjects, 1)
	require.Equal(t, c.ID, sess.ExcludedProjects[0].ProjectID)
	require.Equal(t, "C", sess.ExcludedProjects[0].ProjectName)
	require.Equal(t, `no event mapped to "purchase"`, sess.ExcludedProjects[0].Reason)
}
