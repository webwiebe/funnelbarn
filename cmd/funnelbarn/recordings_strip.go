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

const stripPageURLsUsage = "usage: funnelbarn recordings strip-page-urls --project <slug> [--apply] [--batch <n>]"

// stripFlags are the options of recordings strip-page-urls.
type stripFlags struct {
	project string
	apply   bool
	batch   int
}

func parseStripFlags(args []string) (stripFlags, error) {
	fs := flag.NewFlagSet("recordings strip-page-urls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var f stripFlags
	fs.StringVar(&f.project, "project", "", "project slug (required)")
	fs.BoolVar(&f.apply, "apply", false, "write the change; without it the run only counts")
	fs.IntVar(&f.batch, "batch", 200, fmt.Sprintf("rows per batch, at most %d", repository.MaxPageURLBatch))
	if err := fs.Parse(args); err != nil {
		return f, fmt.Errorf("%s: %w", stripPageURLsUsage, err)
	}
	if f.project == "" || fs.NArg() > 0 {
		return f, errors.New(stripPageURLsUsage)
	}
	return f, nil
}

// pageURLStripper is the store surface the command needs.
type pageURLStripper interface {
	projectLister
	StripRecordingPageURLs(ctx context.Context, projectID string, batch int, apply bool) (repository.PageURLStrip, error)
}

// stripResult is the one JSON line the command prints: counts only.
type stripResult struct {
	Project string `json:"project"`
	Applied bool   `json:"applied"`
	repository.PageURLStrip
}

// stripPageURLs runs one project's strip and writes the counts.
func stripPageURLs(ctx context.Context, w io.Writer, store pageURLStripper, f stripFlags) error {
	projects, err := selectProjects(ctx, store, f.project)
	if err != nil {
		return err
	}
	res, err := store.StripRecordingPageURLs(ctx, projects[0].ID, f.batch, f.apply)
	if err != nil {
		return fmt.Errorf("recordings strip-page-urls: project %s: %w", f.project, err)
	}
	return json.NewEncoder(w).Encode(stripResult{Project: f.project, Applied: f.apply, PageURLStrip: res})
}

// runStripPageURLs handles: funnelbarn recordings strip-page-urls. It reduces
// recordings.page_url of one project to origin, path and utm_* parameters
// (enrich.RecordingPageURL), the form ingest stores since v0.6.226. It is a
// dry run unless --apply is given, reads at most repository.MaxPageURLBatch
// rows at a time and prints counts only (iambarn#489).
func runStripPageURLs(cfg config.Config, args []string) error {
	f, err := parseStripFlags(args)
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
	return stripPageURLs(ctx, os.Stdout, store, f)
}
