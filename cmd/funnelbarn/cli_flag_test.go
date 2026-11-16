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
