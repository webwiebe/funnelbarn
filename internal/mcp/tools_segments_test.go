package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestListSegmentsTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{
		ProjectID: a.ID, Name: "Dutch",
		Rules: []repository.SegmentRule{{Field: "country_code", Operator: "eq", Value: "NL"}},
	})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}

	cs := env.connect(t, tokenRead, a.Slug)

	var out listSegmentsOut
	callOK(t, cs, "list_segments", nil, &out)

	if out.Project != a.Slug {
		t.Errorf("project = %q, want %q", out.Project, a.Slug)
	}
	if len(out.Segments) != 1 || out.Segments[0].ID != seg.ID {
		t.Fatalf("segments = %+v, want one with id %s", out.Segments, seg.ID)
	}
	if len(out.Segments[0].Rules) != 1 {
		t.Errorf("rules = %+v, want 1", out.Segments[0].Rules)
	}
}

func TestListSegmentsTool_EmptyForOtherProject(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	ctx := context.Background()
	if _, err := env.Store.CreateSegment(ctx, repository.Segment{ProjectID: a.ID, Name: "A Segment"}); err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}

	cs := env.connect(t, tokenRead, b.Slug)

	var out listSegmentsOut
	callOK(t, cs, "list_segments", nil, &out)
	if len(out.Segments) != 0 {
		t.Errorf("got %d segments for project b, want 0: %+v", len(out.Segments), out.Segments)
	}
}

func TestCreateSegmentTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, a.Slug)

	var out createSegmentOut
	callOK(t, cs, "create_segment", map[string]any{
		"name": "Mobile Dutch",
		"rules": []map[string]string{
			{"field": "device_type", "operator": "eq", "value": "mobile"},
			{"field": "country_code", "operator": "eq", "value": "NL"},
		},
	}, &out)

	if out.Segment.ID == "" {
		t.Fatal("want a non-empty segment id")
	}
	if out.Segment.ProjectID != a.ID {
		t.Errorf("project_id = %q, want %q", out.Segment.ProjectID, a.ID)
	}
	if len(out.Segment.Rules) != 2 {
		t.Fatalf("got %d rules, want 2: %+v", len(out.Segment.Rules), out.Segment.Rules)
	}

	stored, err := env.Store.SegmentByID(context.Background(), out.Segment.ID)
	if err != nil {
		t.Fatalf("SegmentByID: %v", err)
	}
	if stored.Name != "Mobile Dutch" {
		t.Errorf("stored name = %q, want Mobile Dutch", stored.Name)
	}
}

func TestCreateSegmentTool_MissingName(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "create_segment", map[string]any{})
	if !strings.Contains(msg, "name") {
		t.Errorf("error = %q, want it to mention name", msg)
	}
}

func TestCreateSegmentTool_InvalidField(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "create_segment", map[string]any{
		"name":  "Bad",
		"rules": []map[string]string{{"field": "not_a_real_field", "operator": "eq", "value": "x"}},
	})
	if !strings.Contains(msg, "unsupported segment field") {
		t.Errorf("error = %q, want it to mention the unsupported field", msg)
	}
}

func TestCreateSegmentTool_InvalidOperator(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "create_segment", map[string]any{
		"name":  "Bad",
		"rules": []map[string]string{{"field": "country_code", "operator": "like", "value": "NL"}},
	})
	if !strings.Contains(msg, "unsupported operator") {
		t.Errorf("error = %q, want it to mention the unsupported operator", msg)
	}
}

func TestCreateSegmentTool_ReadTokenScopeError(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "create_segment", map[string]any{"name": "X"})
	if !strings.Contains(msg, "scope") {
		t.Errorf("error = %q, want a scope error", msg)
	}
}

func TestUpdateSegmentTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{ProjectID: a.ID, Name: "Old"})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	cs := env.connect(t, tokenReadWrite, a.Slug)

	var out updateSegmentOut
	callOK(t, cs, "update_segment", map[string]any{
		"segment_id": seg.ID,
		"name":       "New",
		"rules":      []map[string]string{{"field": "browser", "operator": "eq", "value": "Chrome"}},
	}, &out)

	if out.Segment.Name != "New" {
		t.Errorf("name = %q, want New", out.Segment.Name)
	}
	if len(out.Segment.Rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(out.Segment.Rules))
	}
}

func TestUpdateSegmentTool_ValidationError(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{ProjectID: a.ID, Name: "Old"})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "update_segment", map[string]any{"segment_id": seg.ID})
	if !strings.Contains(msg, "name") {
		t.Errorf("error = %q, want it to mention name", msg)
	}
}

func TestUpdateSegmentTool_CrossProjectNotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	ctx := context.Background()
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{ProjectID: a.ID, Name: "Old"})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	cs := env.connect(t, tokenReadWrite, b.Slug)

	msg := callErr(t, cs, "update_segment", map[string]any{"segment_id": seg.ID, "name": "Hijacked"})
	if !strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("error = %q, want a not-found error", msg)
	}

	stillOld, err := env.Store.SegmentByID(ctx, seg.ID)
	if err != nil {
		t.Fatalf("SegmentByID: %v", err)
	}
	if stillOld.Name != "Old" {
		t.Errorf("segment was modified via the wrong project: name = %q", stillOld.Name)
	}
}

func TestUpdateSegmentTool_ReadTokenScopeError(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{ProjectID: a.ID, Name: "Old"})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "update_segment", map[string]any{"segment_id": seg.ID, "name": "New"})
	if !strings.Contains(msg, "scope") {
		t.Errorf("error = %q, want a scope error", msg)
	}
}

func TestDeleteSegmentTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{ProjectID: a.ID, Name: "Old"})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	cs := env.connect(t, tokenReadWrite, a.Slug)

	var out deleteSegmentOut
	callOK(t, cs, "delete_segment", map[string]any{"segment_id": seg.ID}, &out)
	if !out.Deleted {
		t.Error("deleted = false, want true")
	}

	if _, err := env.Store.SegmentByID(ctx, seg.ID); err == nil {
		t.Error("segment still exists after delete")
	}
}

func TestDeleteSegmentTool_CrossProjectNotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	ctx := context.Background()
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{ProjectID: a.ID, Name: "Old"})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	cs := env.connect(t, tokenReadWrite, b.Slug)

	msg := callErr(t, cs, "delete_segment", map[string]any{"segment_id": seg.ID})
	if !strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("error = %q, want a not-found error", msg)
	}

	if _, err := env.Store.SegmentByID(ctx, seg.ID); err != nil {
		t.Errorf("segment deleted via the wrong project: %v", err)
	}
}

func TestDeleteSegmentTool_ReadTokenScopeError(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	ctx := context.Background()
	seg, err := env.Store.CreateSegment(ctx, repository.Segment{ProjectID: a.ID, Name: "Old"})
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "delete_segment", map[string]any{"segment_id": seg.ID})
	if !strings.Contains(msg, "scope") {
		t.Errorf("error = %q, want a scope error", msg)
	}
}

func TestSegmentTools_ListedWithHints(t *testing.T) {
	env := newTestEnv(t, nil)
	cs := env.connect(t, tokenRead, "")

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	seen := map[string]bool{
		"list_segments": false, "create_segment": false, "update_segment": false, "delete_segment": false,
	}
	for _, tool := range res.Tools {
		if _, ok := seen[tool.Name]; !ok {
			continue
		}
		seen[tool.Name] = true
		switch tool.Name {
		case "list_segments":
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Errorf("%s: ReadOnlyHint not set", tool.Name)
			}
		case "delete_segment":
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
