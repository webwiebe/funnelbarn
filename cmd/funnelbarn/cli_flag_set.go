package main

import (
	"context"
	"encoding/json"
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
//	funnelbarn flag set --project=profotograaf --key=landing_work_first --split=true=50,false=50
//
// --split sets a percentage rollout over the flag's own variants and leaves
// the default and the rules alone, and makes the flag an active experiment. Otherwise it changes an existing boolean flag in one of two ways. --default=on serves
// "on" to everyone and keeps the flag's rules. --on-in=<env> serves "off" by
// default and "on" in that one environment only. Running it again with the same
// arguments changes nothing, so a merge script can repeat it.
func runFlagSet(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("flag set", flag.ContinueOnError)
	projectSlug := fs.String("project", "", "project slug")
	key := fs.String("key", "", "flag key")
	def := fs.String("default", "", "on or off: the variant served when no rule matches")
	onIn := fs.String("on-in", "", "environment in which the flag is on; off everywhere else")
	splitSpec := fs.String("split", "", "percentage rollout as variant=percent pairs, for example true=50,false=50")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *projectSlug == "" || *key == "" || countSet(*def, *onIn, *splitSpec) != 1 {
		return fmt.Errorf("%s", flagSetUsage)
	}
	req, err := flagSetRequest(*def, *onIn, *splitSpec)
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

func countSet(values ...string) int {
	n := 0
	for _, v := range values {
		if v != "" {
			n++
		}
	}
	return n
}

func flagSetRequest(def, onIn, splitSpec string) (service.FlagRequest, error) {
	str := func(s string) *string { return &s }
	if splitSpec != "" {
		split, err := service.ParseSplitSpec(splitSpec)
		if err != nil {
			return service.FlagRequest{}, err
		}
		// A config flag serves its default to everyone and an inactive flag serves
		// nothing, so a split makes the flag an active experiment.
		return service.FlagRequest{Split: str(split), Kind: str("experiment"), Status: str("active")}, nil
	}
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
	if req.Split != nil && req.DefaultVariant == nil {
		variants, err := withBooleanVariants(existing.Variants, *req.Split)
		if err != nil {
			return false, fmt.Errorf("flag %q: %w", key, err)
		}
		if err := service.ValidateSplit(variants, *req.Split); err != nil {
			return false, fmt.Errorf("flag %q: %w", key, err)
		}
		req.Variants = &variants
	}
	updated, err := flags.PatchFlag(ctx, existing, req)
	if err != nil {
		return false, fmt.Errorf("update flag %q: %w", key, err)
	}
	return updated.DefaultVariant != existing.DefaultVariant || updated.Split != existing.Split || updated.TargetingRules != existing.TargetingRules, nil
}

// withBooleanVariants adds the variants a split names when they are one of the
// boolean names on, true, off or false, so a flag the SDK registered with only a
// "default" variant can be split in one step. Any other missing name is left for
// ValidateSplit to reject.
func withBooleanVariants(variantsJSON, splitJSON string) (string, error) {
	variants := map[string]any{}
	if err := json.Unmarshal([]byte(variantsJSON), &variants); err != nil {
		return "", fmt.Errorf("variants: %w", err)
	}
	var split map[string]int
	if err := json.Unmarshal([]byte(splitJSON), &split); err != nil {
		return "", fmt.Errorf("split: %w", err)
	}
	for name := range split {
		if _, ok := variants[name]; ok {
			continue
		}
		switch name {
		case "on", "true":
			variants[name] = true
		case "off", "false":
			variants[name] = false
		}
	}
	b, err := json.Marshal(variants)
	return string(b), err
}
