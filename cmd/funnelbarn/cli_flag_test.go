package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestRunFlagCmd_CreatesOffFlagWithEnvironmentRuleOnce(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "flag.db")}
	if err := runProjectCmd(cfg, []string{"create", "--name=Brand Trace"}); err != nil {
		t.Fatalf("project create: %v", err)
	}
	for i := 0; i < 2; i++ { // the second run leaves the flag alone
		if err := runFlagCmd(cfg, []string{"create", "--project=brand-trace", "--key=docs_public", "--on-in=production"}); err != nil {
			t.Fatalf("flag create run %d: %v", i, err)
		}
	}
	if err := runFlagCmd(cfg, []string{"create", "--project=brand-trace", "--key=versus_enabled"}); err != nil {
		t.Fatalf("flag create without rule: %v", err)
	}
	if err := runFlagCmd(cfg, []string{"create", "--project=missing", "--key=x"}); err == nil {
		t.Error("expected an error for an unknown project")
	}

	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	p, err := store.ProjectBySlug(context.Background(), "brand-trace")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	flags, err := store.ListFlags(context.Background(), p.ID)
	if err != nil || len(flags) != 2 {
		t.Fatalf("want 2 flags, got %d (err %v)", len(flags), err)
	}
	for _, f := range flags {
		if f.DefaultVariant != "off" || f.FlagType != "boolean" {
			t.Errorf("%s: default %q type %q", f.FlagKey, f.DefaultVariant, f.FlagType)
		}
		wantRule := f.FlagKey == "docs_public"
		if hasRule := f.TargetingRules != "[]"; hasRule != wantRule {
			t.Errorf("%s: rules %s", f.FlagKey, f.TargetingRules)
		}
	}
}

func TestRunAPIKeySetEnvironment(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "keyenv.db")}
	if err := runAPIKeyCmd(cfg, []string{"create", "--project=shop", "--name=app", "--scope=ingest"}); err != nil {
		t.Fatalf("apikey create: %v", err)
	}
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	keys, err := store.ListAllAPIKeys(context.Background())
	store.Close()
	if err != nil || len(keys) != 1 {
		t.Fatalf("want 1 key, got %d (err %v)", len(keys), err)
	}

	if err := runAPIKeyCmd(cfg, []string{"set-environment", "--id=" + keys[0].ID, "--environment=stg"}); err != nil {
		t.Fatalf("set-environment: %v", err)
	}
	for _, args := range [][]string{
		{"set-environment", "--id=" + keys[0].ID, "--environment=nonsense"},
		{"set-environment", "--id=missing", "--environment=staging"},
		{"set-environment", "--environment=staging"},
	} {
		if err := runAPIKeyCmd(cfg, args); err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}

	store, err = repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store.Close()
	keys, err = store.ListAllAPIKeys(context.Background())
	if err != nil || keys[0].Environment != "staging" {
		t.Errorf("want environment staging, got %+v (err %v)", keys, err)
	}
}
