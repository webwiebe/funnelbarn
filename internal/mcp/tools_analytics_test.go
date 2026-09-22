package mcp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// seedAnalyticsEvent inserts e with id (and, unless e already sets one, a
// derived IngestID), for tests that need control over fields seedEvent
// (internal/api's helper, in a different package) doesn't expose:
// Environment, Properties, OccurredAt, Browser and friends.
func seedAnalyticsEvent(t *testing.T, store *repository.Store, id string, e repository.Event) {
	t.Helper()
	e.ID = id
	if e.IngestID == "" {
		e.IngestID = "ing-" + id
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	if err := store.InsertEvent(context.Background(), e); err != nil {
		t.Fatalf("InsertEvent(%s): %v", id, err)
	}
}

// seedEventsNamed inserts n events with the given name for a project, each
// in its own session, at occurredAt.
func seedEventsNamed(t *testing.T, store *repository.Store, projectID, name string, n int, occurredAt time.Time) {
	t.Helper()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%s-%d", projectID, name, i)
		seedAnalyticsEvent(t, store, id, repository.Event{
			ProjectID:  projectID,
			SessionID:  id,
			Name:       name,
			OccurredAt: occurredAt,
		})
	}
}

func TestAnalyticsTools_ListedWithReadOnlyHint(t *testing.T) {
	env := newTestEnv(t, nil)
	cs := env.connect(t, tokenRead, "")

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	want := map[string]bool{
		"get_stats": false, "list_event_names": false, "get_event_counts": false,
		"list_events": false, "list_event_properties": false,
		"list_event_property_values": false, "list_environments": false,
	}
	for _, tool := range res.Tools {
		if _, ok := want[tool.Name]; !ok {
			continue
		}
		want[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint not set", tool.Name)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("%s not in tools/list", name)
		}
	}
}

func TestGetStatsTool_ProjectIsolation(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	now := time.Now().UTC()
	seedEventsNamed(t, env.Store, a.ID, "pageview", 3, now)
	seedEventsNamed(t, env.Store, b.ID, "pageview", 1, now)

	cs := env.connect(t, tokenRead, a.Slug)

	var out getStatsOut
	callOK(t, cs, "get_stats", nil, &out)
	if out.Project != a.Slug {
		t.Errorf("project = %q, want %q", out.Project, a.Slug)
	}
	if out.TotalEvents != 3 {
		t.Errorf("total_events = %d, want 3 (project A only)", out.TotalEvents)
	}

	callOK(t, cs, "get_stats", map[string]any{"project": b.Slug}, &out)
	if out.Project != b.Slug {
		t.Errorf("project = %q, want %q", out.Project, b.Slug)
	}
	if out.TotalEvents != 1 {
		t.Errorf("total_events = %d, want 1 (project B only, arg overrides header)", out.TotalEvents)
	}
}

func TestGetStatsTool_InvalidRange(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "get_stats", map[string]any{"from": "2026-01-10", "to": "2026-01-01"})
	if msg == "" {
		t.Fatal("want a non-empty error message for from after to")
	}
}

func TestListEventNamesTool_ProjectIsolation(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	now := time.Now().UTC()
	seedEventsNamed(t, env.Store, a.ID, "pageview", 1, now)
	seedEventsNamed(t, env.Store, a.ID, "signup", 1, now)
	seedEventsNamed(t, env.Store, b.ID, "pageview", 1, now)

	cs := env.connect(t, tokenRead, a.Slug)

	var out listEventNamesOut
	callOK(t, cs, "list_event_names", nil, &out)
	if got := out.EventNames; len(got) != 2 || got[0] != "pageview" || got[1] != "signup" {
		t.Errorf("event_names = %v, want [pageview signup]", got)
	}

	callOK(t, cs, "list_event_names", map[string]any{"project": b.Slug}, &out)
	if got := out.EventNames; len(got) != 1 || got[0] != "pageview" {
		t.Errorf("event_names for B = %v, want [pageview] only (no signup leaked from A)", got)
	}
}

func TestGetEventCountsTool_ProjectIsolationAndNamesFilter(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	now := time.Now().UTC()
	seedEventsNamed(t, env.Store, a.ID, "pageview", 3, now)
	seedEventsNamed(t, env.Store, a.ID, "signup", 2, now)
	seedEventsNamed(t, env.Store, b.ID, "pageview", 5, now)

	cs := env.connect(t, tokenRead, a.Slug)

	var out getEventCountsOut
	callOK(t, cs, "get_event_counts", nil, &out)
	if out.TotalEvents != 5 {
		t.Errorf("total_events = %d, want 5 (project A only)", out.TotalEvents)
	}
	if len(out.Events) != 2 {
		t.Fatalf("events = %+v, want 2 distinct names", out.Events)
	}

	// project arg overrides the header and isolates B's events from A's.
	callOK(t, cs, "get_event_counts", map[string]any{"project": b.Slug}, &out)
	if out.TotalEvents != 5 {
		t.Errorf("total_events for B = %d, want 5", out.TotalEvents)
	}
	if len(out.Events) != 1 || out.Events[0].Name != "pageview" || out.Events[0].Count != 5 {
		t.Errorf("events for B = %+v, want [{pageview 5}]", out.Events)
	}

	// names filters to the requested subset.
	callOK(t, cs, "get_event_counts", map[string]any{"names": []string{"signup"}}, &out)
	if len(out.Events) != 1 || out.Events[0].Name != "signup" || out.Events[0].Count != 2 {
		t.Errorf("filtered events = %+v, want [{signup 2}]", out.Events)
	}
	if out.TotalEvents != 2 {
		t.Errorf("filtered total_events = %d, want 2", out.TotalEvents)
	}
}

func TestGetEventCountsTool_InvalidLimit(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "get_event_counts", map[string]any{"limit": 501})
	if msg == "" {
		t.Fatal("want a non-empty error message for an out-of-range limit")
	}
}

func TestGetEventCountsTool_InvalidRange(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "get_event_counts", map[string]any{"from": "not-a-date"})
	if msg == "" {
		t.Fatal("want a non-empty error message for an unparseable date")
	}
}

func TestListEventsTool_ProjectIsolationAndPaging(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		seedAnalyticsEvent(t, env.Store, fmt.Sprintf("a-evt-%d", i), repository.Event{
			ProjectID:  a.ID,
			SessionID:  fmt.Sprintf("a-s-%d", i),
			Name:       "pageview",
			OccurredAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	seedAnalyticsEvent(t, env.Store, "b-evt-0", repository.Event{
		ProjectID: b.ID, SessionID: "b-s-0", Name: "pageview", OccurredAt: base,
	})

	cs := env.connect(t, tokenRead, a.Slug)

	var out listEventsOut
	callOK(t, cs, "list_events", map[string]any{"limit": 2}, &out)
	if len(out.Events) != 2 || !out.HasMore {
		t.Fatalf("page 1 = %d events, has_more=%v; want 2 events, has_more=true", len(out.Events), out.HasMore)
	}
	for _, e := range out.Events {
		if e.ProjectID != a.ID {
			t.Errorf("event %s belongs to project %s, want %s (project isolation)", e.ID, e.ProjectID, a.ID)
		}
	}

	callOK(t, cs, "list_events", map[string]any{"limit": 2, "offset": 2}, &out)
	if len(out.Events) != 1 || out.HasMore {
		t.Fatalf("page 2 = %d events, has_more=%v; want 1 event, has_more=false", len(out.Events), out.HasMore)
	}

	callOK(t, cs, "list_events", map[string]any{"project": b.Slug}, &out)
	if len(out.Events) != 1 || out.Events[0].ProjectID != b.ID {
		t.Errorf("events for B = %+v, want project B's single event only", out.Events)
	}
}

func TestListEventPropertiesTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	seedAnalyticsEvent(t, env.Store, "a-signup", repository.Event{
		ProjectID: a.ID, SessionID: "a-s", Name: "signup",
		Properties: `{"plan":"pro"}`, Browser: "Chrome",
	})
	seedAnalyticsEvent(t, env.Store, "b-signup", repository.Event{
		ProjectID: b.ID, SessionID: "b-s", Name: "signup",
	})

	cs := env.connect(t, tokenRead, a.Slug)

	var out listEventPropertiesOut
	callOK(t, cs, "list_event_properties", map[string]any{"event_name": "signup"}, &out)
	has := map[string]bool{}
	for _, p := range out.Properties {
		has[p] = true
	}
	if !has["plan"] {
		t.Errorf("properties = %v, want to include the custom property %q", out.Properties, "plan")
	}
	if !has["browser"] {
		t.Errorf("properties = %v, want to include the populated metadata column %q", out.Properties, "browser")
	}

	// Project isolation: B's signup event has neither a plan property nor a
	// populated browser column.
	callOK(t, cs, "list_event_properties", map[string]any{"event_name": "signup", "project": b.Slug}, &out)
	for _, p := range out.Properties {
		if p == "plan" || p == "browser" {
			t.Errorf("properties for B = %v, leaked %q from project A", out.Properties, p)
		}
	}

	if msg := callErr(t, cs, "list_event_properties", map[string]any{"event_name": ""}); msg == "" {
		t.Error("want a non-empty error message for a missing event_name")
	}
}

func TestListEventPropertyValuesTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	seedAnalyticsEvent(t, env.Store, "a-signup-1", repository.Event{
		ProjectID: a.ID, SessionID: "a-s-1", Name: "signup", Properties: `{"plan":"pro"}`,
	})
	seedAnalyticsEvent(t, env.Store, "a-signup-2", repository.Event{
		ProjectID: a.ID, SessionID: "a-s-2", Name: "signup", Properties: `{"plan":"free"}`,
	})
	seedAnalyticsEvent(t, env.Store, "b-signup-1", repository.Event{
		ProjectID: b.ID, SessionID: "b-s-1", Name: "signup", Properties: `{"plan":"enterprise"}`,
	})

	cs := env.connect(t, tokenRead, a.Slug)

	var out listEventPropertyValuesOut
	callOK(t, cs, "list_event_property_values", map[string]any{"event_name": "signup", "property": "plan"}, &out)
	want := map[string]bool{"pro": true, "free": true}
	if len(out.Values) != 2 {
		t.Fatalf("values = %v, want 2 entries", out.Values)
	}
	for _, v := range out.Values {
		if !want[v] {
			t.Errorf("values = %v, unexpected value %q", out.Values, v)
		}
	}

	callOK(t, cs, "list_event_property_values", map[string]any{"event_name": "signup", "property": "plan", "project": b.Slug}, &out)
	if len(out.Values) != 1 || out.Values[0] != "enterprise" {
		t.Errorf("values for B = %v, want [enterprise] only (project isolation)", out.Values)
	}

	if msg := callErr(t, cs, "list_event_property_values", map[string]any{"property": "plan"}); msg == "" {
		t.Error("want a non-empty error message for a missing event_name")
	}
	if msg := callErr(t, cs, "list_event_property_values", map[string]any{"event_name": "signup"}); msg == "" {
		t.Error("want a non-empty error message for a missing property")
	}
}

func TestListEnvironmentsTool_ProjectIsolation(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	seedAnalyticsEvent(t, env.Store, "a-prod", repository.Event{ProjectID: a.ID, SessionID: "a-s1", Name: "pageview", Environment: "production"})
	seedAnalyticsEvent(t, env.Store, "a-staging", repository.Event{ProjectID: a.ID, SessionID: "a-s2", Name: "pageview", Environment: "staging"})
	seedAnalyticsEvent(t, env.Store, "b-dev", repository.Event{ProjectID: b.ID, SessionID: "b-s1", Name: "pageview", Environment: "development"})

	cs := env.connect(t, tokenRead, a.Slug)

	var out listEnvironmentsOut
	callOK(t, cs, "list_environments", nil, &out)
	if len(out.Environments) != 2 || out.Environments[0] != "production" || out.Environments[1] != "staging" {
		t.Errorf("environments = %v, want [production staging]", out.Environments)
	}

	callOK(t, cs, "list_environments", map[string]any{"project": b.Slug}, &out)
	if len(out.Environments) != 1 || out.Environments[0] != "development" {
		t.Errorf("environments for B = %v, want [development] only", out.Environments)
	}
}
