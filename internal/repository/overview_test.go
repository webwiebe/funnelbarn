package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// wideRange returns a from/to that comfortably brackets "now".
func wideRange() (time.Time, time.Time) {
	now := time.Now().UTC()
	return now.AddDate(0, 0, -1), now.AddDate(0, 0, 1)
}

func TestOverview_ProjectRollupsAndTotals(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	from, to := wideRange()

	a, _ := s.CreateProject(ctx, "A", "proj-a")
	b, _ := s.CreateProject(ctx, "B", "proj-b")

	insertEvent(t, s, a.ID, "a-s1", "pageview", "https://a/1")
	insertEvent(t, s, a.ID, "a-s1", "pageview", "https://a/2")
	insertEvent(t, s, a.ID, "a-s2", "pageview", "https://a/1")
	insertEvent(t, s, b.ID, "b-s1", "pageview", "https://b/1")

	rollups, err := s.ProjectRollups(ctx, from, to, "")
	require.NoError(t, err)
	byID := map[string]repository.ProjectRollup{}
	for _, r := range rollups {
		byID[r.ProjectID] = r
	}
	require.EqualValues(t, 3, byID[a.ID].Events)
	require.EqualValues(t, 2, byID[a.ID].UniqueSessions)
	require.EqualValues(t, 1, byID[b.ID].Events)
	require.EqualValues(t, 1, byID[b.ID].UniqueSessions)

	events, sessions, err := s.OverviewTotals(ctx, from, to, "")
	require.NoError(t, err)
	require.EqualValues(t, 4, events)
	require.EqualValues(t, 3, sessions)
}

func TestOverview_MappingsAndSuggestions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p, _ := s.CreateProject(ctx, "P", "proj-p")
	insertEvent(t, s, p.ID, "s1", "registration", "https://p/signup")
	insertEvent(t, s, p.ID, "s1", "some_custom_event", "https://p/x")

	// Suggestions: "registration" should map to the seeded sign_up canonical;
	// the custom event has no confident guess.
	sugs, err := s.MappingSuggestions(ctx, p.ID)
	require.NoError(t, err)
	got := map[string]string{}
	for _, sg := range sugs {
		got[sg.RawName] = sg.SuggestedKey
	}
	require.Equal(t, "sign_up", got["registration"])
	require.Equal(t, "", got["some_custom_event"])

	// Confirm the mapping, then it should drop out of suggestions.
	require.NoError(t, s.UpsertMapping(ctx, p.ID, "registration", "sign_up"))
	sugs2, err := s.MappingSuggestions(ctx, p.ID)
	require.NoError(t, err)
	for _, sg := range sugs2 {
		require.NotEqual(t, "registration", sg.RawName, "confirmed mapping should not be suggested again")
	}

	grouped, err := s.MappingsByProject(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"registration"}, grouped[p.ID]["sign_up"])
}

func TestOverview_DeleteCanonicalEventBlockedByFunnel(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// A saved funnel references sign_up; deleting that canonical must conflict.
	_, err := s.CreateCanonicalFunnel(ctx, repository.CanonicalFunnel{
		Name:  "f",
		Scope: "session",
		Steps: []repository.CanonicalFunnelStep{{CanonicalKey: "sign_up"}},
	})
	require.NoError(t, err)

	err = s.DeleteCanonicalEvent(ctx, "sign_up")
	require.Error(t, err)
}

// TestAnalyzeCanonicalFunnel_Aggregates is the core correctness test: the
// aggregate step counts equal the sum of the per-project counts, a project
// missing a step's mapping is excluded, and a canonical key mapped to several
// raw names matches all of them.
func TestAnalyzeCanonicalFunnel_Aggregates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	from, to := wideRange()

	a, _ := s.CreateProject(ctx, "A", "proj-a")
	b, _ := s.CreateProject(ctx, "B", "proj-b")
	c, _ := s.CreateProject(ctx, "C", "proj-c")

	// Project A: page_view <- pageview ; sign_up <- {signup, createaccount}
	require.NoError(t, s.UpsertMapping(ctx, a.ID, "pageview", "page_view"))
	require.NoError(t, s.UpsertMapping(ctx, a.ID, "signup", "sign_up"))
	require.NoError(t, s.UpsertMapping(ctx, a.ID, "createaccount", "sign_up"))
	insertEvent(t, s, a.ID, "a1", "pageview", "https://a/1")
	insertEvent(t, s, a.ID, "a1", "signup", "https://a/signup")
	insertEvent(t, s, a.ID, "a2", "pageview", "https://a/1")
	insertEvent(t, s, a.ID, "a3", "pageview", "https://a/1")
	insertEvent(t, s, a.ID, "a3", "createaccount", "https://a/signup") // IN-clause hit
	// A: page_view sessions {a1,a2,a3}=3 ; sign_up {a1,a3}=2

	// Project B: page_view <- view ; sign_up <- register
	require.NoError(t, s.UpsertMapping(ctx, b.ID, "view", "page_view"))
	require.NoError(t, s.UpsertMapping(ctx, b.ID, "register", "sign_up"))
	insertEvent(t, s, b.ID, "b1", "view", "https://b/1")
	insertEvent(t, s, b.ID, "b1", "register", "https://b/signup")
	insertEvent(t, s, b.ID, "b2", "view", "https://b/1")
	// B: page_view {b1,b2}=2 ; sign_up {b1}=1

	// Project C: only page_view mapped -> excluded from a page_view->sign_up funnel.
	require.NoError(t, s.UpsertMapping(ctx, c.ID, "pageview", "page_view"))
	insertEvent(t, s, c.ID, "c1", "pageview", "https://c/1")

	funnel := repository.CanonicalFunnel{
		Name:  "signup flow",
		Scope: "session",
		Steps: []repository.CanonicalFunnelStep{
			{CanonicalKey: "page_view"},
			{CanonicalKey: "sign_up"},
		},
	}

	res, err := s.AnalyzeCanonicalFunnel(ctx, funnel, nil, from, to, nil)
	require.NoError(t, err)

	// Aggregate: page_view = 3+2 = 5, sign_up = 2+1 = 3.
	require.Len(t, res.Steps, 2)
	require.EqualValues(t, 5, res.Steps[0].Count)
	require.EqualValues(t, 3, res.Steps[1].Count)
	require.InDelta(t, 3.0/5.0, res.Steps[1].Conversion, 1e-9)

	// Aggregate equals the sum of per-project breakdowns.
	var sumStep0, sumStep1 int64
	for _, bp := range res.ByProject {
		sumStep0 += bp.Steps[0].Count
		sumStep1 += bp.Steps[1].Count
	}
	require.EqualValues(t, res.Steps[0].Count, sumStep0)
	require.EqualValues(t, res.Steps[1].Count, sumStep1)

	// Project C excluded and reported; not present in the breakdown.
	require.Len(t, res.ExcludedProjects, 1)
	require.Equal(t, c.ID, res.ExcludedProjects[0].ProjectID)
	for _, bp := range res.ByProject {
		require.NotEqual(t, c.ID, bp.ProjectID)
	}
}

func TestAnalyzeCanonicalFunnel_SegmentThreadsThrough(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	from, to := wideRange()

	a, _ := s.CreateProject(ctx, "A", "proj-a")
	require.NoError(t, s.UpsertMapping(ctx, a.ID, "pageview", "page_view"))
	insertEvent(t, s, a.ID, "a1", "pageview", "https://a/1") // device_type=desktop

	funnel := repository.CanonicalFunnel{
		Name:  "pv",
		Scope: "session",
		Steps: []repository.CanonicalFunnelStep{{CanonicalKey: "page_view"}},
	}

	// No segment: one session.
	res, err := s.AnalyzeCanonicalFunnel(ctx, funnel, nil, from, to, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Steps[0].Count)

	// device_type=mobile: the seeded event is desktop, so zero.
	seg := &repository.SegmentFilter{Field: "device_type", Op: "eq", Value: "mobile"}
	res2, err := s.AnalyzeCanonicalFunnel(ctx, funnel, nil, from, to, seg)
	require.NoError(t, err)
	require.EqualValues(t, 0, res2.Steps[0].Count)
}

func TestOverview_ListAllEventsCrossProject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a, _ := s.CreateProject(ctx, "A", "proj-a")
	b, _ := s.CreateProject(ctx, "B", "proj-b")
	insertEvent(t, s, a.ID, "a1", "pageview", "https://a/1")
	insertEvent(t, s, b.ID, "b1", "pageview", "https://b/1")

	all, err := s.ListAllEvents(ctx, repository.EventFilter{}, 50)
	require.NoError(t, err)
	require.Len(t, all, 2)

	// Filter by project.
	onlyA, err := s.ListAllEvents(ctx, repository.EventFilter{ProjectID: a.ID}, 50)
	require.NoError(t, err)
	require.Len(t, onlyA, 1)
	require.Equal(t, a.ID, onlyA[0].ProjectID)
}

func TestOverview_DimensionRollups(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	from, to := wideRange()

	a, _ := s.CreateProject(ctx, "A", "dim-a")
	b, _ := s.CreateProject(ctx, "B", "dim-b")
	insertEvent(t, s, a.ID, "a1", "pageview", "https://a/home")
	insertEvent(t, s, a.ID, "a2", "pageview", "https://a/home")
	insertEvent(t, s, b.ID, "b1", "pageview", "https://b/pricing")

	pages, err := s.OverviewTopPages(ctx, from, to, 10, "")
	require.NoError(t, err)
	require.NotEmpty(t, pages)
	require.Equal(t, "https://a/home", pages[0].URL)
	require.EqualValues(t, 2, pages[0].Views)

	refs, err := s.OverviewTopReferrers(ctx, from, to, 10, "")
	require.NoError(t, err)
	require.NotEmpty(t, refs) // insertEvent seeds referrer_domain=google.com

	_, err = s.OverviewTopCountries(ctx, from, to, 10, "")
	require.NoError(t, err) // no country on seeded events -> empty, but must not error

	visitors, err := s.OverviewVisitorsByProjectDaily(ctx, from, to, "")
	require.NoError(t, err)
	require.NotEmpty(t, visitors)

	dims, err := s.OverviewDimensionBreakdown(ctx, "device_type", from, to, 10, "")
	require.NoError(t, err)
	require.NotEmpty(t, dims) // insertEvent seeds device_type=desktop
	require.Equal(t, "desktop", dims[0].Value)

	_, err = s.OverviewDimensionBreakdown(ctx, "not_a_column", from, to, 10, "")
	require.Error(t, err)
}

func TestOverview_CanonicalCatalogCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.CreateCanonicalEvent(ctx, repository.CanonicalEvent{Key: "contact_form", Label: "Contact Form", SortOrder: 99})
	require.NoError(t, err)
	require.Equal(t, "contact_form", created.Key)

	// Duplicate key conflicts.
	_, err = s.CreateCanonicalEvent(ctx, repository.CanonicalEvent{Key: "contact_form", Label: "dup"})
	require.Error(t, err)

	updated, err := s.UpdateCanonicalEvent(ctx, repository.CanonicalEvent{Key: "contact_form", Label: "Contact", SortOrder: 5})
	require.NoError(t, err)
	require.Equal(t, "Contact", updated.Label)

	// Update of unknown key is a not-found.
	_, err = s.UpdateCanonicalEvent(ctx, repository.CanonicalEvent{Key: "nope", Label: "x"})
	require.Error(t, err)

	keys, err := s.CanonicalKeySet(ctx)
	require.NoError(t, err)
	require.True(t, keys["contact_form"])
	require.True(t, keys["page_view"]) // seeded

	// Unreferenced canonical can be deleted.
	require.NoError(t, s.DeleteCanonicalEvent(ctx, "contact_form"))
	keys2, _ := s.CanonicalKeySet(ctx)
	require.False(t, keys2["contact_form"])
}

func TestOverview_MappingDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p, _ := s.CreateProject(ctx, "P", "mapdel-p")

	require.NoError(t, s.UpsertMapping(ctx, p.ID, "pageview", "page_view"))
	m, err := s.ListMappings(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, m, 1)

	require.NoError(t, s.DeleteMapping(ctx, p.ID, "pageview"))
	m2, err := s.ListMappings(ctx, p.ID)
	require.NoError(t, err)
	require.Empty(t, m2)
}

func TestOverview_CanonicalFunnelCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	f, err := s.CreateCanonicalFunnel(ctx, repository.CanonicalFunnel{
		Name:       "Flow",
		Scope:      "session",
		ProjectIDs: []string{"p1", "p2"},
		Segment:    "mobile",
		Steps: []repository.CanonicalFunnelStep{
			{CanonicalKey: "page_view"},
			{CanonicalKey: "sign_up"},
		},
	})
	require.NoError(t, err)
	require.Len(t, f.Steps, 2)
	require.Equal(t, []string{"p1", "p2"}, f.ProjectIDs)
	require.Equal(t, "mobile", f.Segment)

	got, err := s.CanonicalFunnelByID(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, "Flow", got.Name)
	require.Equal(t, "Page View", got.Steps[0].Label) // resolved from catalog

	list, err := s.ListCanonicalFunnels(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	f.Name = "Renamed"
	f.Steps = []repository.CanonicalFunnelStep{{CanonicalKey: "login"}}
	updated, err := s.UpdateCanonicalFunnel(ctx, f)
	require.NoError(t, err)
	require.Equal(t, "Renamed", updated.Name)
	require.Len(t, updated.Steps, 1)
	require.Equal(t, "login", updated.Steps[0].CanonicalKey)

	require.NoError(t, s.DeleteCanonicalFunnel(ctx, f.ID))
	list2, err := s.ListCanonicalFunnels(ctx)
	require.NoError(t, err)
	require.Empty(t, list2)
}
