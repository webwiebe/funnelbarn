package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

const projectMergeUsage = "usage: funnelbarn project merge --from <slug> --to <slug> [--env <environment>] [--dry-run]"

// runProjectMerge handles: funnelbarn project merge --from=<slug> --to=<slug> [--env=production] [--dry-run]
//
// It moves the source project's data into the target in one transaction
// (#545). The source's API keys move too, so old ingest keys keep working and
// file their events under the target, in --env when given. Nothing is deleted:
// rows the target already holds a key for stay with the source, which is
// archived. --dry-run prints the exact counts and changes nothing.
func runProjectMerge(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("project merge", flag.ContinueOnError)
	from := fs.String("from", "", "slug of the project to merge away")
	to := fs.String("to", "", "slug of the project that receives the data")
	env := fs.String("env", "", "environment the moved API keys file their events under")
	dryRun := fs.Bool("dry-run", false, "count what would move and change nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == "" || *to == "" {
		return fmt.Errorf("%s", projectMergeUsage)
	}
	if !environment.IsKnown(*env) {
		return fmt.Errorf("--env %q is not a known environment", *env)
	}
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer logShutdown(slog.LevelError, "store", store.Close)
	ctx := context.Background()
	source, err := store.ProjectBySlug(ctx, *from)
	if err != nil {
		return fmt.Errorf("source project %q: %w", *from, err)
	}
	target, err := store.ProjectBySlug(ctx, *to)
	if err != nil {
		return fmt.Errorf("target project %q: %w", *to, err)
	}
	report, err := store.MergeProjects(ctx, source, target, environment.Normalize(*env), *dryRun)
	if err != nil {
		return err
	}
	fmt.Print(repository.FormatMergeReport(report))
	return nil
}
