package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

const flagCreateUsage = "usage: funnelbarn flag create --project=<slug> --key=<flag_key> [--on-in=<environment>]"
const flagSetUsage = "usage: funnelbarn flag set --project=<slug> --key=<flag_key> (--default=on|off | --on-in=<environment>)"

// runFlagCmd handles: funnelbarn flag create --project=brandtrace --key=docs_public [--on-in=production]
//
// It creates a boolean on/off flag whose default variant is off. With --on-in
// the flag gets one rule that serves "on" when the evaluation context's
// environment equals that value. An existing key is left untouched, so the
// command can be repeated. Flag creation otherwise needs a dashboard session or
// an IAMBarn user token, which a deploy has neither of.
func runFlagCmd(cfg config.Config, args []string) error {
	if len(args) > 0 && args[0] == "set" {
		return runFlagSet(cfg, args[1:])
	}
	if len(args) == 0 || args[0] != "create" {
		return errors.New(flagCreateUsage + "\n" + flagSetUsage)
	}
	fs := flag.NewFlagSet("flag create", flag.ContinueOnError)
	projectSlug := fs.String("project", "", "project slug")
	key := fs.String("key", "", "flag key")
	onIn := fs.String("on-in", "", "environment in which the flag is on for everyone; empty leaves no rule")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *projectSlug == "" || *key == "" {
		return fmt.Errorf("--project and --key are required")
	}
	rules, err := onInRules(*onIn)
	if err != nil {
		return err
	}

	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer logShutdown(slog.LevelError, "store", store.Close)
	created, slug, err := createOffFlag(context.Background(), store, *projectSlug, *key, rules)
	if err != nil {
		return err
	}
	fmt.Printf("{\"flag\":%q,\"project\":%q,\"created\":%t}\n", *key, slug, created)
	return nil
}

// onInRules returns the targeting rules JSON that turns a flag on for one
// environment, or "[]" when env is empty.
func onInRules(env string) (string, error) {
	if env == "" {
		return "[]", nil
	}
	if !environment.IsKnown(env) {
		return "", fmt.Errorf("--on-in %q is not a known environment", env)
	}
	canonical := environment.Normalize(env)
	raw, err := json.Marshal([]service.TargetingRule{{
		Name:       "on in " + canonical,
		Variant:    "on",
		Match:      "all",
		Conditions: []service.TargetingCondition{{ContextKey: "environment", Operator: "eq", Value: canonical}},
	}})
	return string(raw), err
}

// createOffFlag creates the flag unless the project already has that key. It
// returns whether it created one and the project's slug.
func createOffFlag(ctx context.Context, store *repository.Store, projectSlug, key, rules string) (bool, string, error) {
	project, err := service.NewProjectService(store).GetProjectBySlug(ctx, projectSlug)
	if err != nil {
		return false, "", fmt.Errorf("project %q: %w", projectSlug, err)
	}
	flags := service.NewFlagService(store)
	existing, err := flags.ListFlags(ctx, project.ID)
	if err != nil {
		return false, "", fmt.Errorf("list flags: %w", err)
	}
	for _, f := range existing {
		if f.FlagKey == key {
			return false, project.Slug, nil
		}
	}
	str := func(s string) *string { return &s }
	if _, err := flags.CreateFromRequest(ctx, project.ID, service.FlagRequest{
		FlagKey:        &key,
		Name:           &key,
		FlagType:       str("boolean"),
		Variants:       str(`{"on":true,"off":false}`),
		DefaultVariant: str("off"),
		Split:          str(`{"on":0,"off":100}`),
		TargetingRules: str(rules),
		Status:         str("active"),
	}); err != nil {
		return false, "", fmt.Errorf("create flag %q: %w", key, err)
	}
	return true, project.Slug, nil
}
