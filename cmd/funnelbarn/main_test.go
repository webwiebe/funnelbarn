package main

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
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

// TestValidateFailClosed pins the production fail-closed startup contract:
// production must refuse to boot when the API-key validator would fail open
// or when no login mechanism exists; other tiers stay permissive for dev.
func TestValidateFailClosed(t *testing.T) {
	cases := []struct {
		name             string
		env              string
		apiKeyConfigured bool
		authConfigured   bool
		wantErr          string
	}{
		{"production fully configured", "production", true, true, ""},
		{"production without api key", "production", false, true, "API key"},
		{"production without auth", "production", true, false, "authentication mechanism"},
		{"development without anything", "development", false, false, ""},
		{"test without anything", "test", false, false, ""},
		{"staging without anything", "staging", false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFailClosed(tc.env, tc.apiKeyConfigured, tc.authConfigured)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
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

// Dead-lettered events are recoverable: replay re-attributes records that
// carry no project slug to --project, persists them, and clears the file only
// when every record landed.
func TestRunReplayDeadLetter(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DBPath: filepath.Join(dir, "test.db"), SpoolDir: dir}

	body := base64.StdEncoding.EncodeToString([]byte(`{"name":"pageview","url":"https://example.com/x"}`))
	// No projectSlug — exactly the shape that used to fail the project_id
	// foreign key and get dead-lettered.
	rec := spool.Record{IngestID: "dl-1", ReceivedAt: time.Now().UTC(), BodyBase64: body}
	if err := spool.AppendDeadLetter(dir, rec); err != nil {
		t.Fatalf("AppendDeadLetter: %v", err)
	}

	// Without --project there is nothing to attribute the record to, so it is
	// skipped and the file is left intact for a later, correct replay.
	if err := runReplayDeadLetter(cfg, []string{"--dry-run"}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	left, err := spool.ReadRecords(spool.DeadLetterPath(dir))
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(left) != 1 {
		t.Fatalf("expected the record to survive a skipped replay, got %d", len(left))
	}

	if err := runReplayDeadLetter(cfg, []string{"--project=recovered"}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	left, err = spool.ReadRecords(spool.DeadLetterPath(dir))
	if err != nil {
		t.Fatalf("ReadRecords after replay: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("expected the dead-letter file to be cleared after a clean replay, got %d records", len(left))
	}

	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	got, err := store.GetEventByIngestID(context.Background(), "dl-1")
	if err != nil || got == nil {
		t.Fatalf("expected the replayed event to be stored, got %v (err %v)", got, err)
	}
	if got.ProjectID == "" {
		t.Error("replayed event should be attributed to the --project project")
	}
}
