package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestGetSetupGuideTool_ReturnsDoc(t *testing.T) {
	var gotProject repository.Project
	env := newTestEnv(t, func(d *Deps) {
		d.SetupDoc = func(ctx context.Context, p repository.Project) (string, error) {
			gotProject = p
			return "# Setup: " + p.Slug, nil
		}
	})
	a := env.createProject(t, "Alpha", "alpha")

	cs := env.connect(t, tokenRead, a.Slug)

	var out getSetupGuideOut
	callOK(t, cs, "get_setup_guide", nil, &out)

	if out.Markdown != "# Setup: alpha" {
		t.Errorf("markdown = %q, want %q", out.Markdown, "# Setup: alpha")
	}
	if gotProject.ID != a.ID {
		t.Errorf("SetupDoc called with project %+v, want %s", gotProject, a.ID)
	}
}

func TestGetSetupGuideTool_ProjectArgOverridesHeader(t *testing.T) {
	env := newTestEnv(t, func(d *Deps) {
		d.SetupDoc = func(ctx context.Context, p repository.Project) (string, error) {
			return "# Setup: " + p.Slug, nil
		}
	})
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")

	cs := env.connect(t, tokenRead, a.Slug)

	var out getSetupGuideOut
	callOK(t, cs, "get_setup_guide", map[string]any{"project": b.Slug}, &out)

	if out.Markdown != "# Setup: beta" {
		t.Errorf("markdown = %q, want %q", out.Markdown, "# Setup: beta")
	}
}

func TestGetSetupGuideTool_NoProjectSelected(t *testing.T) {
	env := newTestEnv(t, func(d *Deps) {
		d.SetupDoc = func(ctx context.Context, p repository.Project) (string, error) {
			return "unreachable", nil
		}
	})
	env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, "")

	msg := callErr(t, cs, "get_setup_guide", nil)
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestGetSetupGuideTool_NilDeps(t *testing.T) {
	// SetupDoc left nil, as newTestEnv does by default.
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "get_setup_guide", nil)
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestGetSetupGuideTool_ServiceErrorPassesThrough(t *testing.T) {
	env := newTestEnv(t, func(d *Deps) {
		d.SetupDoc = func(ctx context.Context, p repository.Project) (string, error) {
			return "", errors.New("boom")
		}
	})
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "get_setup_guide", nil)
	if msg != "internal error" {
		t.Errorf("message = %q, want an opaque internal error", msg)
	}
}

func TestGetSetupGuideTool_WriteScopeNotRequired(t *testing.T) {
	env := newTestEnv(t, func(d *Deps) {
		d.SetupDoc = func(ctx context.Context, p repository.Project) (string, error) {
			return "ok", nil
		}
	})
	a := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, a.Slug)

	var out getSetupGuideOut
	callOK(t, cs, "get_setup_guide", nil, &out)
	if out.Markdown != "ok" {
		t.Errorf("markdown = %q, want %q", out.Markdown, "ok")
	}
}

func TestGetSetupGuideTool_ListedWithReadOnlyHint(t *testing.T) {
	env := newTestEnv(t, nil)
	cs := env.connect(t, tokenRead, "")

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range res.Tools {
		if tool.Name != "get_setup_guide" {
			continue
		}
		found = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("get_setup_guide: ReadOnlyHint not set")
		}
	}
	if !found {
		t.Fatal("get_setup_guide not in tools/list")
	}
}
