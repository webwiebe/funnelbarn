package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResolveProject(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")

	t.Run("arg wins over header", func(t *testing.T) {
		p, err := ResolveProject(ctx, env.Deps.Projects, b.Slug, a.Slug)
		if err != nil {
			t.Fatalf("ResolveProject: %v", err)
		}
		if p.ID != b.ID {
			t.Errorf("got project %s, want %s", p.Slug, b.Slug)
		}
	})

	t.Run("header used when arg empty", func(t *testing.T) {
		p, err := ResolveProject(ctx, env.Deps.Projects, "", a.Slug)
		if err != nil {
			t.Fatalf("ResolveProject: %v", err)
		}
		if p.ID != a.ID {
			t.Errorf("got project %s, want %s", p.Slug, a.Slug)
		}
	})

	t.Run("matches by ID", func(t *testing.T) {
		p, err := ResolveProject(ctx, env.Deps.Projects, a.ID, "")
		if err != nil {
			t.Fatalf("ResolveProject: %v", err)
		}
		if p.ID != a.ID {
			t.Errorf("got project %s, want %s", p.Slug, a.Slug)
		}
	})

	t.Run("matches by slug", func(t *testing.T) {
		p, err := ResolveProject(ctx, env.Deps.Projects, "", b.Slug)
		if err != nil {
			t.Fatalf("ResolveProject: %v", err)
		}
		if p.ID != b.ID {
			t.Errorf("got project %s, want %s", p.Slug, b.Slug)
		}
	})

	t.Run("unknown value lists available projects", func(t *testing.T) {
		_, err := ResolveProject(ctx, env.Deps.Projects, "nope", "")
		if err == nil {
			t.Fatal("want error, got nil")
		}
		var in *inputError
		if !errors.As(err, &in) {
			t.Fatalf("want inputError, got %T: %v", err, err)
		}
		msg := err.Error()
		if !strings.Contains(msg, `"nope"`) || !strings.Contains(msg, "alpha") || !strings.Contains(msg, "beta") {
			t.Errorf("message = %q, want it to name the value and list alpha, beta", msg)
		}
	})

	t.Run("nothing set lists available projects", func(t *testing.T) {
		_, err := ResolveProject(ctx, env.Deps.Projects, "", "")
		if err == nil {
			t.Fatal("want error, got nil")
		}
		var in *inputError
		if !errors.As(err, &in) {
			t.Fatalf("want inputError, got %T: %v", err, err)
		}
		msg := err.Error()
		if !strings.Contains(msg, ProjectHeader) || !strings.Contains(msg, "alpha") || !strings.Contains(msg, "beta") {
			t.Errorf("message = %q, want it to name %s and list alpha, beta", msg, ProjectHeader)
		}
	})
}

func TestResolveProject_SingleProjectIsTheDefault(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()
	only := env.createProject(t, "Only", "only")

	p, err := ResolveProject(ctx, env.Deps.Projects, "", "")
	if err != nil {
		t.Fatalf("ResolveProject: %v", err)
	}
	if p.ID != only.ID {
		t.Errorf("got project %q, want the only project %q", p.Slug, only.Slug)
	}

	if _, err := ResolveProject(ctx, env.Deps.Projects, "nope", ""); err == nil {
		t.Error("an unknown project argument must still fail on a single-project instance")
	}
}

func TestResolveProject_NoProjectsAtAll(t *testing.T) {
	env := newTestEnv(t, nil)

	_, err := ResolveProject(context.Background(), env.Deps.Projects, "", "")
	var in *inputError
	if !errors.As(err, &in) {
		t.Fatalf("want inputError, got %T: %v", err, err)
	}
}
