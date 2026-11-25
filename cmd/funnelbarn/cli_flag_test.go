package main

import (
	"context"
	"encoding/json"
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

func TestRunFlagSet_DefaultOnAndOnInProductionAreRepeatable(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "flagset.db")}
	if err := runProjectCmd(cfg, []string{"create", "--name=Brand Trace"}); err != nil {
		t.Fatalf("project create: %v", err)
	}
	for _, k := range []string{"versus_enabled", "coverage_enabled"} {
		if err := runFlagCmd(cfg, []string{"create", "--project=brand-trace", "--key=" + k}); err != nil {
			t.Fatalf("create %s: %v", k, err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := runFlagCmd(cfg, []string{"set", "--project=brand-trace", "--key=versus_enabled", "--default=on"}); err != nil {
			t.Fatalf("set default run %d: %v", i, err)
		}
		if err := runFlagCmd(cfg, []string{"set", "--project=brand-trace", "--key=coverage_enabled", "--on-in=production"}); err != nil {
			t.Fatalf("set on-in run %d: %v", i, err)
		}
	}
	if err := runFlagCmd(cfg, []string{"set", "--project=brand-trace", "--key=nope", "--default=on"}); err == nil {
		t.Error("expected an error for an unknown flag")
	}
	if err := runFlagCmd(cfg, []string{"set", "--project=brand-trace", "--key=versus_enabled"}); err == nil {
		t.Error("expected usage error with neither --default nor --on-in")
	}

	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	p, _ := store.ProjectBySlug(context.Background(), "brand-trace")
	flags, err := store.ListFlags(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, f := range flags {
		switch f.FlagKey {
		case "versus_enabled":
			if f.DefaultVariant != "on" || f.TargetingRules != "[]" {
				t.Errorf("versus_enabled: default %q rules %s", f.DefaultVariant, f.TargetingRules)
			}
		case "coverage_enabled":
			if f.DefaultVariant != "off" || f.TargetingRules == "[]" {
				t.Errorf("coverage_enabled: default %q rules %s", f.DefaultVariant, f.TargetingRules)
			}
		}
	}
}

func TestRunFlagSet_SplitAddsBooleanVariantsAndRejectsBadSplits(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "split.db")}
	if err := runProjectCmd(cfg, []string{"create", "--name=Photo"}); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := runFlagCmd(cfg, []string{"create", "--project=photo", "--key=hero"}); err != nil {
		t.Fatalf("flag create: %v", err)
	}
	for _, bad := range []string{"true=60,false=60", "true=50", "maybe=100", "true"} {
		if err := runFlagCmd(cfg, []string{"set", "--project=photo", "--key=hero", "--split=" + bad}); err == nil {
			t.Errorf("--split=%s: want an error", bad)
		}
	}
	if err := runFlagCmd(cfg, []string{"set", "--project=photo", "--key=hero", "--split=true=50,false=50", "--default=on"}); err == nil {
		t.Error("--split with --default: want a usage error")
	}
	for i := 0; i < 2; i++ {
		if err := runFlagCmd(cfg, []string{"set", "--project=photo", "--key=hero", "--split=true=50,false=50"}); err != nil {
			t.Fatalf("split run %d: %v", i, err)
		}
	}
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	p, _ := store.ProjectBySlug(context.Background(), "photo")
	flags, _ := store.ListFlags(context.Background(), p.ID)
	if len(flags) != 1 || flags[0].Split != `{"false":50,"true":50}` {
		t.Fatalf("flags = %+v", flags)
	}
	if flags[0].Status != "active" || flags[0].Kind != "experiment" {
		t.Errorf("status %q kind %q, want active experiment", flags[0].Status, flags[0].Kind)
	}
	var variants map[string]any
	if err := json.Unmarshal([]byte(flags[0].Variants), &variants); err != nil || variants["true"] != true || variants["false"] != false {
		t.Errorf("variants = %s (%v)", flags[0].Variants, err)
	}
}
