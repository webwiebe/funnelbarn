package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// The project, apikey and user subcommands create rows through the services
// on a real database.
func TestRunCLI_CreateProjectAPIKeyAndUser(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "cli.db")}

	if err := runProjectCmd(cfg, []string{"create", "--name=My Site"}); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := runProjectCmd(cfg, []string{"create", "--name=My Site"}); err == nil {
		t.Error("expected a conflict creating the same slug twice")
	}
	// An existing project, then one created automatically.
	if err := runAPIKeyCmd(cfg, []string{"create", "--project=my-site", "--name=app", "--scope=ingest"}); err != nil {
		t.Fatalf("apikey create: %v", err)
	}
	if err := runAPIKeyCmd(cfg, []string{"create", "--project=fresh", "--name=app"}); err != nil {
		t.Fatalf("apikey create with auto project: %v", err)
	}
	if err := runUserCmd(cfg, []string{"create", "--username=admin", "--password=secret"}); err != nil {
		t.Fatalf("user create: %v", err)
	}

	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	keys, err := store.ListAllAPIKeys(context.Background())
	if err != nil || len(keys) != 2 {
		t.Fatalf("want 2 keys, got %d (err %v)", len(keys), err)
	}
	if _, err := store.ProjectBySlug(context.Background(), "fresh"); err != nil {
		t.Errorf("auto-created project missing: %v", err)
	}
}
