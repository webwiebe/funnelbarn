package mcp

import (
	"context"
	"testing"
)

func TestListProjectsTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")

	cs := env.connect(t, tokenRead, a.Slug)

	var out listProjectsOut
	callOK(t, cs, "list_projects", nil, &out)

	if len(out.Projects) != 2 {
		t.Fatalf("got %d projects, want 2: %+v", len(out.Projects), out.Projects)
	}
	bySlug := map[string]projectSummary{}
	for _, p := range out.Projects {
		bySlug[p.Slug] = p
	}
	if p, ok := bySlug[a.Slug]; !ok || p.ID != a.ID || p.Name != a.Name || p.Status != a.Status {
		t.Errorf("project %s missing or wrong: %+v", a.Slug, p)
	}
	if _, ok := bySlug[b.Slug]; !ok {
		t.Errorf("project %s missing from list", b.Slug)
	}
	if out.Default != a.Slug {
		t.Errorf("default = %q, want %q", out.Default, a.Slug)
	}
}

func TestListProjectsTool_NoHeaderNoDefault(t *testing.T) {
	env := newTestEnv(t, nil)
	env.createProject(t, "Alpha", "alpha")

	cs := env.connect(t, tokenRead, "")

	var out listProjectsOut
	callOK(t, cs, "list_projects", nil, &out)

	if out.Default != "" {
		t.Errorf("default = %q, want empty", out.Default)
	}
}

func TestGetProjectTool_DefaultsToHeader(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	env.createProject(t, "Beta", "beta")
	ctx := context.Background()
	if err := env.Deps.ProjectHealth.MarkSetupCalled(ctx, a.ID); err != nil {
		t.Fatalf("MarkSetupCalled: %v", err)
	}
	if _, err := env.Deps.ProjectHealth.MarkEventsReceived(ctx, a.ID); err != nil {
		t.Fatalf("MarkEventsReceived: %v", err)
	}

	cs := env.connect(t, tokenRead, a.Slug)

	var out getProjectOut
	callOK(t, cs, "get_project", nil, &out)

	if out.ID != a.ID || out.Slug != a.Slug {
		t.Fatalf("got project %+v, want %s", out, a.Slug)
	}
	if !out.Health.SetupCalled || !out.Health.EventsReceived {
		t.Errorf("health = %+v, want setup_called and events_received true", out.Health)
	}
	if out.Health.FlagsEvaluated || out.Health.RecordingsReceived {
		t.Errorf("health = %+v, want flags_evaluated and recordings_received false", out.Health)
	}
}

func TestGetProjectTool_ArgOverridesHeader(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")

	cs := env.connect(t, tokenRead, a.Slug)

	var out getProjectOut
	callOK(t, cs, "get_project", map[string]any{"project": b.Slug}, &out)

	if out.ID != b.ID || out.Slug != b.Slug {
		t.Fatalf("got project %+v, want %s", out, b.Slug)
	}
	if out.Health.SetupCalled || out.Health.EventsReceived || out.Health.FlagsEvaluated || out.Health.RecordingsReceived {
		t.Errorf("health = %+v, want all false for a never-touched project", out.Health)
	}
}

func TestGetProjectTool_UnknownProject(t *testing.T) {
	env := newTestEnv(t, nil)
	env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, "alpha")

	msg := callErr(t, cs, "get_project", map[string]any{"project": "nope"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestGetProjectTool_NoProjectSelected(t *testing.T) {
	env := newTestEnv(t, nil)
	env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, "")

	msg := callErr(t, cs, "get_project", nil)
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestProjectTools_ListedWithReadOnlyHint(t *testing.T) {
	env := newTestEnv(t, nil)
	cs := env.connect(t, tokenRead, "")

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	want := map[string]bool{"list_projects": false, "get_project": false}
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
