package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// insertPageView inserts a page_view event at a specific time so the flow
// window functions (which order by occurred_at) see a deterministic sequence.
func insertPageView(t *testing.T, s *repository.Store, projectID, sessionID, url, referrerDomain string, at time.Time) {
	t.Helper()
	e := repository.Event{
		ID:             randomHex(t) + sessionID + url,
		ProjectID:      projectID,
		SessionID:      sessionID,
		Name:           "page_view",
		URL:            url,
		ReferrerDomain: referrerDomain,
		IngestID:       "ingest-" + randomHex(t) + sessionID + url,
		OccurredAt:     at,
	}
	require.NoError(t, s.InsertEvent(context.Background(), e))
}

func TestPageFlows_EmptyProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "FlowsEmpty", "flows-empty")

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	// No events, no page given → resolves to top page (none) → empty result.
	res, err := s.PageFlows(ctx, p.ID, "", 5, from, to, "")
	require.NoError(t, err)
	assert.Empty(t, res.Nodes)
	assert.Empty(t, res.Links)
	assert.EqualValues(t, 0, res.TotalSessions)
}

func TestPageFlows_BuildsGraph(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "Flows", "flows-graph")

	base := time.Now().UTC().Truncate(time.Second).Add(-30 * time.Minute)
	const home = "https://example.com/home"
	const pricing = "https://example.com/pricing"
	const checkout = "https://example.com/checkout"

	// Session 1: home -> pricing -> checkout (entered from google).
	insertPageView(t, s, p.ID, "s1", home, "google.com", base)
	insertPageView(t, s, p.ID, "s1", pricing, "google.com", base.Add(1*time.Minute))
	insertPageView(t, s, p.ID, "s1", checkout, "google.com", base.Add(2*time.Minute))

	// Session 2: home -> pricing then exits (direct entry).
	insertPageView(t, s, p.ID, "s2", home, "", base.Add(3*time.Minute))
	insertPageView(t, s, p.ID, "s2", pricing, "", base.Add(4*time.Minute))

	// Session 3: lands directly on pricing and exits immediately.
	insertPageView(t, s, p.ID, "s3", pricing, "bing.com", base.Add(5*time.Minute))

	from := base.Add(-time.Hour)
	to := base.Add(time.Hour)

	res, err := s.PageFlows(ctx, p.ID, pricing, 5, from, to, "")
	require.NoError(t, err)
	assert.Equal(t, pricing, res.FocusedPage)
	// 3 sessions visited pricing.
	assert.EqualValues(t, 3, res.TotalSessions)
	assert.NotEmpty(t, res.Nodes)
	assert.NotEmpty(t, res.Links)

	// The focused page node must exist at depth 0.
	var foundFocus bool
	for _, n := range res.Nodes {
		if n.NodeType == "page" && n.Depth == 0 && n.Label == pricing {
			foundFocus = true
		}
	}
	assert.True(t, foundFocus, "focused page node should be present")
}

func TestPageFlows_ResolvesTopPageWhenEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "FlowsTop", "flows-top")

	base := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Minute)
	const home = "https://example.com/home"

	insertPageView(t, s, p.ID, "s1", home, "", base)
	insertPageView(t, s, p.ID, "s2", home, "", base.Add(time.Minute))

	from := base.Add(-time.Hour)
	to := base.Add(time.Hour)

	res, err := s.PageFlows(ctx, p.ID, "", 5, from, to, "")
	require.NoError(t, err)
	assert.Equal(t, home, res.FocusedPage)
	assert.EqualValues(t, 2, res.TotalSessions)
}

func TestPageFlows_DepthClamped(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "FlowsDepth", "flows-depth")

	base := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Minute)
	const home = "https://example.com/home"
	insertPageView(t, s, p.ID, "s1", home, "", base)

	from := base.Add(-time.Hour)
	to := base.Add(time.Hour)

	// Out-of-range depths get clamped to the default; must not error.
	res, err := s.PageFlows(ctx, p.ID, home, 0, from, to, "")
	require.NoError(t, err)
	assert.Equal(t, home, res.FocusedPage)

	res, err = s.PageFlows(ctx, p.ID, home, 99, from, to, "")
	require.NoError(t, err)
	assert.Equal(t, home, res.FocusedPage)
}
