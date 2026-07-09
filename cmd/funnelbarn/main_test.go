package main

import (
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
)

func TestBuildOIDCClient_NilWhenUnconfigured(t *testing.T) {
	// An empty config has none of the four required OIDC fields.
	if c := buildOIDCClient(config.Config{}); c != nil {
		t.Errorf("expected nil OIDC client when unconfigured, got %v", c)
	}
	// Partial config (missing secret + redirect) is still not enabled.
	partial := config.Config{OIDCIssuer: "https://iam.example", OIDCClientID: "id"}
	if c := buildOIDCClient(partial); c != nil {
		t.Errorf("expected nil OIDC client for partial config, got %v", c)
	}
}

func TestSlugPattern(t *testing.T) {
	valid := []string{"default", "my-project", "a1", "team-42-alpha"}
	for _, s := range valid {
		if !slugPattern.MatchString(s) {
			t.Errorf("slugPattern should match %q", s)
		}
	}
	invalid := []string{"", "-lead", "trail-", "Upper", "has space", "double--dash", "under_score"}
	for _, s := range invalid {
		if slugPattern.MatchString(s) {
			t.Errorf("slugPattern should reject %q", s)
		}
	}
}

func TestToSlugLocal(t *testing.T) {
	cases := map[string]string{
		"My Project":     "my-project",
		"  Trim Me  ":    "trim-me",
		"Foo/Bar & Baz":  "foo-bar-baz",
		"already-a-slug": "already-a-slug",
	}
	for in, want := range cases {
		if got := toSlugLocal(in); got != want {
			t.Errorf("toSlugLocal(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRunUserCmd_MissingSubcommand(t *testing.T) {
	if err := runUserCmd(config.Config{}, nil); err == nil {
		t.Error("expected error when no subcommand given")
	}
	if err := runUserCmd(config.Config{}, []string{"bogus"}); err == nil {
		t.Error("expected error for unknown user subcommand")
	}
}

func TestRunUserCmd_CreateRequiresUsernameAndPassword(t *testing.T) {
	t.Setenv("FUNNELBARN_ADMIN_USERNAME", "")
	t.Setenv("FUNNELBARN_ADMIN_PASSWORD", "")

	err := runUserCmd(config.Config{}, []string{"create", "--password=secret"})
	if err == nil || !strings.Contains(err.Error(), "username") {
		t.Errorf("expected --username required error, got %v", err)
	}
	err = runUserCmd(config.Config{}, []string{"create", "--username=bob"})
	if err == nil || !strings.Contains(err.Error(), "password") {
		t.Errorf("expected --password required error, got %v", err)
	}
}

func TestRunProjectCmd_Validation(t *testing.T) {
	if err := runProjectCmd(config.Config{}, nil); err == nil {
		t.Error("expected error when no subcommand given")
	}
	if err := runProjectCmd(config.Config{}, []string{"bogus"}); err == nil {
		t.Error("expected error for unknown project subcommand")
	}
	// Missing --name.
	if err := runProjectCmd(config.Config{}, []string{"create"}); err == nil ||
		!strings.Contains(err.Error(), "name") {
		t.Errorf("expected --name required error, got %v", err)
	}
	// An explicit slug that violates the pattern is rejected before any DB open.
	err := runProjectCmd(config.Config{}, []string{"create", "--name=Valid", "--slug=Bad Slug"})
	if err == nil || !strings.Contains(err.Error(), "invalid slug") {
		t.Errorf("expected invalid slug error, got %v", err)
	}
}

func TestRunAPIKeyCmd_Validation(t *testing.T) {
	if err := runAPIKeyCmd(config.Config{}, nil); err == nil {
		t.Error("expected error when no subcommand given")
	}
	if err := runAPIKeyCmd(config.Config{}, []string{"bogus"}); err == nil {
		t.Error("expected error for unknown apikey subcommand")
	}
	// Missing --name.
	if err := runAPIKeyCmd(config.Config{}, []string{"create"}); err == nil ||
		!strings.Contains(err.Error(), "name") {
		t.Errorf("expected --name required error, got %v", err)
	}
	// Invalid scope is rejected before any DB open.
	err := runAPIKeyCmd(config.Config{}, []string{"create", "--name=app", "--scope=wat"})
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Errorf("expected scope validation error, got %v", err)
	}
}
