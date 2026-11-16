package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// runFlagCmd handles: funnelbarn flag create --project=brandtrace --key=docs_public [--on-in=production]
//
// It creates a boolean on/off flag whose default variant is off. With --on-in
// the flag gets one rule that serves "on" when the evaluation context's
// environment equals that value. An existing key is left untouched, so the
// command can be repeated. Flag creation otherwise needs a dashboard session or
// an IAMBarn user token, which a deploy has neither of.
func runFlagCmd(cfg config.Config, args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return fmt.Errorf("usage: funnelbarn flag create --project=<slug> --key=<flag_key> [--on-in=<environment>]")
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
	rules := "[]"
	if *onIn != "" {
		if !environment.IsKnown(*onIn) {
			return fmt.Errorf("--on-in %q is not a known environment", *onIn)
		}
		raw, err := json.Marshal([]service.TargetingRule{{
			Name:    "on in " + environment.Normalize(*onIn),
			Variant: "on",
			Match:   "all",
			Conditions: []service.TargetingCondition{{
				ContextKey: "environment", Operator: "eq", Value: environment.Normalize(*onIn),
			}},
		}})
		if err != nil {
			return err
		}
		rules = string(raw)
	}

	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer logShutdown(slog.LevelError, "store", store.Close)
	ctx := context.Background()

	project, err := service.NewProjectService(store).GetProjectBySlug(ctx, *projectSlug)
	if err != nil {
		return fmt.Errorf("project %q: %w", *projectSlug, err)
	}
	flags := service.NewFlagService(store)
	existing, err := flags.ListFlags(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("list flags: %w", err)
	}
	for _, f := range existing {
		if f.FlagKey == *key {
			fmt.Printf("{\"flag\":%q,\"project\":%q,\"created\":false}\n", *key, project.Slug)
			return nil
		}
	}

	str := func(s string) *string { return &s }
	if _, err := flags.CreateFromRequest(ctx, project.ID, service.FlagRequest{
		FlagKey:        key,
		Name:           key,
		FlagType:       str("boolean"),
		Variants:       str(`{"on":true,"off":false}`),
		DefaultVariant: str("off"),
		Split:          str(`{"on":0,"off":100}`),
		TargetingRules: str(rules),
		Status:         str("active"),
	}); err != nil {
		return fmt.Errorf("create flag %q: %w", *key, err)
	}
	fmt.Printf("{\"flag\":%q,\"project\":%q,\"created\":true}\n", *key, project.Slug)
	return nil
}
