package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

const backfillEnvUsage = "usage: funnelbarn recordings backfill-environment --project <slug> [--apply] [--batch <n>]"

func parseBackfillEnvFlags(args []string) (stripFlags, error) {
	fs := flag.NewFlagSet("recordings backfill-environment", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var f stripFlags
	fs.StringVar(&f.project, "project", "", "project slug (required)")
	fs.BoolVar(&f.apply, "apply", false, "write the change; without it the run only counts")
	fs.IntVar(&f.batch, "batch", 200, fmt.Sprintf("rows per batch, at most %d", repository.MaxPageURLBatch))
	if err := fs.Parse(args); err != nil {
		return f, fmt.Errorf("%s: %w", backfillEnvUsage, err)
	}
	if f.project == "" || fs.NArg() > 0 {
		return f, errors.New(backfillEnvUsage)
	}
	return f, nil
}

// environmentBackfiller is the store surface the command needs.
type environmentBackfiller interface {
	projectLister
	BackfillRecordingEnvironments(ctx context.Context, projectID string, batch int, apply bool, onBatch func(repository.EnvironmentBackfill)) (repository.EnvironmentBackfill, error)
}

// backfillResult is the one JSON line the command prints: counts only.
type backfillResult struct {
	Project string `json:"project"`
	Applied bool   `json:"applied"`
	repository.EnvironmentBackfill
}

func backfillEnvironments(ctx context.Context, w, progress io.Writer, store environmentBackfiller, f stripFlags) error {
	projects, err := selectProjects(ctx, store, f.project)
	if err != nil {
		return err
	}
	onBatch := func(r repository.EnvironmentBackfill) {
		_, _ = fmt.Fprintf(progress, "batch %d: scanned %d filled %d mixed %d no_events %d\n", r.Batches, r.Scanned, r.Filled, r.Mixed, r.NoEvents)
	}
	res, err := store.BackfillRecordingEnvironments(ctx, projects[0].ID, f.batch, f.apply, onBatch)
	if err != nil {
		return fmt.Errorf("recordings backfill-environment: project %s: %w", f.project, err)
	}
	return json.NewEncoder(w).Encode(backfillResult{Project: f.project, Applied: f.apply, EnvironmentBackfill: res})
}

// runBackfillEnvironment handles: funnelbarn recordings backfill-environment.
// It gives the recordings of one project that were stored with an empty
// environment (#560) the environment of their session's events. A dry run
// unless --apply is given, at most repository.MaxPageURLBatch rows at a time,
// counts only in the output. Sessions whose events disagree stay empty and are
// counted.
func runBackfillEnvironment(cfg config.Config, args []string) error {
	f, err := parseBackfillEnvFlags(args)
	if err != nil {
		return err
	}
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer logShutdown(slog.LevelError, "store", store.Close)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return backfillEnvironments(ctx, os.Stdout, os.Stderr, store, f)
}
