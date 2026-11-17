package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestRunProjectMerge(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "merge.db")}
	for _, slug := range []string{"old-site", "new-site"} {
		if err := runAPIKeyCmd(cfg, []string{"create", "--project=" + slug, "--name=app", "--scope=ingest"}); err != nil {
			t.Fatalf("apikey create %s: %v", slug, err)
		}
	}

	if err := runProjectCmd(cfg, []string{"merge", "--from=old-site", "--to=new-site", "--env=production", "--dry-run"}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if got := keysOf(t, cfg, "old-site"); got != 1 {
		t.Fatalf("dry run moved keys: old-site holds %d, want 1", got)
	}

	for _, args := range [][]string{
		{"merge", "--from=old-site"},
		{"merge", "--from=old-site", "--to=old-site"},
		{"merge", "--from=missing", "--to=new-site"},
		{"merge", "--from=old-site", "--to=new-site", "--env=nonsense"},
	} {
		if err := runProjectCmd(cfg, args); err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}

	if err := runProjectCmd(cfg, []string{"merge", "--from=old-site", "--to=new-site", "--env=production"}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := keysOf(t, cfg, "old-site"); got != 0 {
		t.Errorf("old-site still holds %d keys", got)
	}
	if got := keysOf(t, cfg, "new-site"); got != 2 {
		t.Errorf("new-site holds %d keys, want 2", got)
	}
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	old, err := store.ProjectBySlug(context.Background(), "old-site")
	if err != nil || old.Status != repository.ProjectArchivedStatus {
		t.Errorf("old-site status = %q (err %v), want archived", old.Status, err)
	}
	keys, _ := store.ListAllAPIKeys(context.Background())
	moved := 0
	for _, k := range keys {
		if k.Environment == "production" {
			moved++
		}
	}
	if moved != 1 {
		t.Errorf("%d keys carry environment production, want only the moved one", moved)
	}
}

func keysOf(t *testing.T, cfg config.Config, slug string) int {
	t.Helper()
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	p, err := store.ProjectBySlug(context.Background(), slug)
	if err != nil {
		t.Fatalf("project %s: %v", slug, err)
	}
	keys, err := store.ListAPIKeys(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("keys %s: %v", slug, err)
	}
	return len(keys)
}
