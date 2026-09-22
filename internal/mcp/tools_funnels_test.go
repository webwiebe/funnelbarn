package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestListFunnelsTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID,
		Name:      "Checkout",
		Steps:     []repository.FunnelStep{{EventName: "cart_viewed"}, {EventName: "purchased"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}

	cs := env.connect(t, tokenRead, a.Slug)

	var out listFunnelsOut
	callOK(t, cs, "list_funnels", nil, &out)

	if out.Project != a.Slug {
		t.Errorf("project = %q, want %q", out.Project, a.Slug)
	}
	if len(out.Funnels) != 1 {
		t.Fatalf("got %d funnels, want 1: %+v", len(out.Funnels), out.Funnels)
	}
	if out.Funnels[0].ID != f.ID || len(out.Funnels[0].Steps) != 2 {
		t.Errorf("funnel = %+v, want id %s with 2 steps", out.Funnels[0], f.ID)
	}
}

func TestListFunnelsTool_EmptyForOtherProject(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	ctx := context.Background()
	if _, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "A Funnel", Steps: []repository.FunnelStep{{EventName: "ev"}},
	}); err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}

	cs := env.connect(t, tokenRead, b.Slug)

	var out listFunnelsOut
	callOK(t, cs, "list_funnels", nil, &out)
	if len(out.Funnels) != 0 {
		t.Errorf("got %d funnels for project b, want 0: %+v", len(out.Funnels), out.Funnels)
	}
}

func TestCreateFunnelTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, a.Slug)

	var out createFunnelOut
	callOK(t, cs, "create_funnel", map[string]any{
		"name":        "Onboarding",
		"description": "Tracks signup to activation",
		"steps": []map[string]any{
			{"event_name": "signup"},
			{"event_name": "activated", "filters": []map[string]string{{"property": "plan", "value": "pro"}}},
		},
	}, &out)

	if out.Funnel.ID == "" {
		t.Fatal("want a non-empty funnel id")
	}
	if out.Funnel.ProjectID != a.ID {
		t.Errorf("project_id = %q, want %q", out.Funnel.ProjectID, a.ID)
	}
	if out.Funnel.Name != "Onboarding" {
		t.Errorf("name = %q, want Onboarding", out.Funnel.Name)
	}
	if len(out.Funnel.Steps) != 2 {
		t.Fatalf("got %d steps, want 2: %+v", len(out.Funnel.Steps), out.Funnel.Steps)
	}
	if out.Funnel.Steps[1].EventName != "activated" || len(out.Funnel.Steps[1].Filters) != 1 {
		t.Errorf("step 2 = %+v, want event_name activated with 1 filter", out.Funnel.Steps[1])
	}

	// Persisted, visible via the store directly.
	stored, err := env.Store.FunnelByID(context.Background(), out.Funnel.ID)
	if err != nil {
		t.Fatalf("FunnelByID: %v", err)
	}
	if stored.Name != "Onboarding" {
		t.Errorf("stored name = %q, want Onboarding", stored.Name)
	}
}

func TestCreateFunnelTool_MissingName(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "create_funnel", map[string]any{
		"steps": []map[string]any{{"event_name": "ev"}},
	})
	if !strings.Contains(msg, "name") {
		t.Errorf("error = %q, want it to mention name", msg)
	}
}

func TestCreateFunnelTool_NoSteps(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "create_funnel", map[string]any{
		"name": "Empty", "steps": []map[string]any{},
	})
	if !strings.Contains(msg, "steps") {
		t.Errorf("error = %q, want it to mention steps", msg)
	}
}

func TestCreateFunnelTool_EmptyStepEventName(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "create_funnel", map[string]any{
		"name": "Bad Step", "steps": []map[string]any{{"event_name": ""}},
	})
	if !strings.Contains(msg, "event_name") {
		t.Errorf("error = %q, want it to mention event_name", msg)
	}
}

func TestCreateFunnelTool_ReadTokenScopeError(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "create_funnel", map[string]any{
		"name": "X", "steps": []map[string]any{{"event_name": "ev"}},
	})
	if !strings.Contains(msg, "scope") {
		t.Errorf("error = %q, want a scope error", msg)
	}
}

func TestUpdateFunnelTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID,
		Name:      "Old Name",
		Scope:     "page_view",
		Steps:     []repository.FunnelStep{{EventName: "step1"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}

	cs := env.connect(t, tokenReadWrite, a.Slug)

	var out updateFunnelOut
	callOK(t, cs, "update_funnel", map[string]any{
		"funnel_id": f.ID,
		"name":      "New Name",
		"steps": []map[string]any{
			{"event_name": "step_a"},
			{"event_name": "step_b"},
		},
	}, &out)

	if out.Funnel.Name != "New Name" {
		t.Errorf("name = %q, want New Name", out.Funnel.Name)
	}
	if len(out.Funnel.Steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(out.Funnel.Steps))
	}
	// Scope is not a tool argument and must survive the update untouched.
	if out.Funnel.Scope != "page_view" {
		t.Errorf("scope = %q, want page_view to be preserved", out.Funnel.Scope)
	}
}

func TestUpdateFunnelTool_ValidationError(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "update_funnel", map[string]any{
		"funnel_id": f.ID,
		"steps":     []map[string]any{{"event_name": "ev"}},
	})
	if !strings.Contains(msg, "name") {
		t.Errorf("error = %q, want it to mention name", msg)
	}
}

func TestUpdateFunnelTool_CrossProjectNotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}

	cs := env.connect(t, tokenReadWrite, b.Slug)
	msg := callErr(t, cs, "update_funnel", map[string]any{
		"funnel_id": f.ID,
		"name":      "Hijacked",
		"steps":     []map[string]any{{"event_name": "ev"}},
	})
	if !strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("error = %q, want a not-found error", msg)
	}

	// The funnel must survive untouched.
	stillA, err := env.Store.FunnelByID(ctx, f.ID)
	if err != nil {
		t.Fatalf("FunnelByID: %v", err)
	}
	if stillA.Name != "F" {
		t.Errorf("funnel was modified via the wrong project: name = %q", stillA.Name)
	}
}

func TestUpdateFunnelTool_ReadTokenScopeError(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "update_funnel", map[string]any{
		"funnel_id": f.ID, "name": "X", "steps": []map[string]any{{"event_name": "ev"}},
	})
	if !strings.Contains(msg, "scope") {
		t.Errorf("error = %q, want a scope error", msg)
	}
}

func TestDeleteFunnelTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	cs := env.connect(t, tokenReadWrite, a.Slug)

	var out deleteFunnelOut
	callOK(t, cs, "delete_funnel", map[string]any{"funnel_id": f.ID}, &out)
	if !out.Deleted {
		t.Error("deleted = false, want true")
	}

	if _, err := env.Store.FunnelByID(ctx, f.ID); err == nil {
		t.Error("funnel still exists after delete")
	}
}

func TestDeleteFunnelTool_CrossProjectNotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	cs := env.connect(t, tokenReadWrite, b.Slug)

	msg := callErr(t, cs, "delete_funnel", map[string]any{"funnel_id": f.ID})
	if !strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("error = %q, want a not-found error", msg)
	}

	if _, err := env.Store.FunnelByID(ctx, f.ID); err != nil {
		t.Errorf("funnel deleted via the wrong project: %v", err)
	}
}

func TestDeleteFunnelTool_ReadTokenScopeError(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "delete_funnel", map[string]any{"funnel_id": f.ID})
	if !strings.Contains(msg, "scope") {
		t.Errorf("error = %q, want a scope error", msg)
	}
}

func TestAnalyzeFunnelTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID,
		Name:      "Checkout",
		Steps:     []repository.FunnelStep{{EventName: "cart_viewed"}, {EventName: "purchased"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	cs := env.connect(t, tokenRead, a.Slug)

	var out analyzeFunnelOut
	callOK(t, cs, "analyze_funnel", map[string]any{"funnel_id": f.ID}, &out)

	if out.Project != a.Slug {
		t.Errorf("project = %q, want %q", out.Project, a.Slug)
	}
	if out.Funnel.ID != f.ID {
		t.Errorf("funnel id = %q, want %q", out.Funnel.ID, f.ID)
	}
	if len(out.Results) != 2 {
		t.Fatalf("got %d results, want 2 (one per step): %+v", len(out.Results), out.Results)
	}
}

func TestAnalyzeFunnelTool_WithSegment(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{
		ProjectID: a.ID, Name: "Dutch",
		Rules: []repository.SegmentRule{{Field: "country_code", Operator: "eq", Value: "NL"}},
	})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	cs := env.connect(t, tokenRead, a.Slug)

	var out analyzeFunnelOut
	callOK(t, cs, "analyze_funnel", map[string]any{"funnel_id": f.ID, "segment_id": seg.ID}, &out)
	if len(out.Results) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(out.Results), out.Results)
	}
}

func TestAnalyzeFunnelTool_CrossProjectSegmentNotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	// Segment belongs to project b, funnel and call are scoped to project a.
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{
		ProjectID: b.ID, Name: "Other Project Segment",
	})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "analyze_funnel", map[string]any{"funnel_id": f.ID, "segment_id": seg.ID})
	if !strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("error = %q, want a not-found error", msg)
	}
}

func TestAnalyzeFunnelTool_CrossProjectFunnelNotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	cs := env.connect(t, tokenRead, b.Slug)

	msg := callErr(t, cs, "analyze_funnel", map[string]any{"funnel_id": f.ID})
	if !strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("error = %q, want a not-found error", msg)
	}
}

func TestAnalyzeFunnelTool_InvalidRange(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	f, err := env.Store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: a.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "ev"}},
	})
	if err != nil {
		t.Fatalf("CreateFunnel: %v", err)
	}
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "analyze_funnel", map[string]any{
		"funnel_id": f.ID, "from": "2026-01-10", "to": "2026-01-01",
	})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestFunnelTools_ListedWithHints(t *testing.T) {
	env := newTestEnv(t, nil)
	cs := env.connect(t, tokenRead, "")

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	readOnly := map[string]bool{"list_funnels": false, "analyze_funnel": false}
	destructive := map[string]bool{"delete_funnel": false}
	seen := map[string]bool{
		"list_funnels": false, "analyze_funnel": false, "create_funnel": false,
		"update_funnel": false, "delete_funnel": false,
	}
	for _, tool := range res.Tools {
		if _, ok := seen[tool.Name]; !ok {
			continue
		}
		seen[tool.Name] = true
		if _, want := readOnly[tool.Name]; want {
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Errorf("%s: ReadOnlyHint not set", tool.Name)
			}
		}
		if _, want := destructive[tool.Name]; want {
			if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
				t.Errorf("%s: DestructiveHint not set to true", tool.Name)
			}
		}
	}
	for name, ok := range seen {
		if !ok {
			t.Errorf("%s not in tools/list", name)
		}
	}
}
