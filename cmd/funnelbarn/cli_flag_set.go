package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// runFlagSet handles: funnelbarn flag set --project=brandtrace --key=versus_enabled --default=on
//
//	funnelbarn flag set --project=brandtrace --key=coverage_enabled --on-in=production
//
// It changes an existing boolean flag in one of two ways. --default=on serves
// "on" to everyone and keeps the flag's rules. --on-in=<env> serves "off" by
// default and "on" in that one environment only. Running it again with the same
// arguments changes nothing, so a merge script can repeat it.
func runFlagSet(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("flag set", flag.ContinueOnError)
	projectSlug := fs.String("project", "", "project slug")
	key := fs.String("key", "", "flag key")
	def := fs.String("default", "", "on or off: the variant served when no rule matches")
	onIn := fs.String("on-in", "", "environment in which the flag is on; off everywhere else")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *projectSlug == "" || *key == "" || (*def == "") == (*onIn == "") {
		return fmt.Errorf("%s", flagSetUsage)
	}
	req, err := flagSetRequest(*def, *onIn)
	if err != nil {
		return err
	}
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer logShutdown(slog.LevelError, "store", store.Close)
	changed, err := setFlag(context.Background(), store, *projectSlug, *key, req)
	if err != nil {
		return err
	}
	fmt.Printf("{\"flag\":%q,\"project\":%q,\"changed\":%t}\n", *key, *projectSlug, changed)
	return nil
}

func flagSetRequest(def, onIn string) (service.FlagRequest, error) {
	str := func(s string) *string { return &s }
	if onIn != "" {
		rules, err := onInRules(onIn)
		if err != nil {
			return service.FlagRequest{}, err
		}
		return service.FlagRequest{DefaultVariant: str("off"), Split: str(`{"on":0,"off":100}`), TargetingRules: str(rules)}, nil
	}
	switch def {
	case "on":
		return service.FlagRequest{DefaultVariant: str("on"), Split: str(`{"on":100,"off":0}`)}, nil
	case "off":
		return service.FlagRequest{DefaultVariant: str("off"), Split: str(`{"on":0,"off":100}`)}, nil
	}
	return service.FlagRequest{}, fmt.Errorf("--default must be on or off, got %q", def)
}

// setFlag lays req over the project's flag with that key and reports whether
// anything differed.
func setFlag(ctx context.Context, store *repository.Store, projectSlug, key string, req service.FlagRequest) (bool, error) {
	project, err := service.NewProjectService(store).GetProjectBySlug(ctx, projectSlug)
	if err != nil {
		return false, fmt.Errorf("project %q: %w", projectSlug, err)
	}
	flags := service.NewFlagService(store)
	existing, err := flags.GetFlagByKey(ctx, project.ID, key)
	if err != nil {
		return false, fmt.Errorf("flag %q in project %q: %w", key, projectSlug, err)
	}
	updated, err := flags.PatchFlag(ctx, existing, req)
	if err != nil {
		return false, fmt.Errorf("update flag %q: %w", key, err)
	}
	return updated.DefaultVariant != existing.DefaultVariant || updated.Split != existing.Split || updated.TargetingRules != existing.TargetingRules, nil
}
