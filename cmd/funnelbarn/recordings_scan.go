package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/storage"
)

const recordingsUsage = "usage: funnelbarn recordings scan-url-params [--project <slug>] [--max-memory-mib <n>] [--by-url]"

// scanFlags are the options of recordings scan-url-params.
type scanFlags struct {
	project      string
	maxMemoryMiB uint64
	byURL        bool
}

func parseScanFlags(args []string) (scanFlags, error) {
	fs := flag.NewFlagSet("recordings scan-url-params", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var f scanFlags
	fs.StringVar(&f.project, "project", "", "scan only the project with this slug")
	fs.Uint64Var(&f.maxMemoryMiB, "max-memory-mib", 64, "stop when the scan holds more than this many MiB")
	fs.BoolVar(&f.byURL, "by-url", false, "also count matching URLs by host, masked path and parameter name")
	if err := fs.Parse(args); err != nil {
		return f, fmt.Errorf("%s: %w", recordingsUsage, err)
	}
	if fs.NArg() > 0 {
		return f, fmt.Errorf("%s: unexpected argument %q", recordingsUsage, fs.Arg(0))
	}
	if f.maxMemoryMiB == 0 {
		return f, fmt.Errorf("%s: --max-memory-mib must be above 0", recordingsUsage)
	}
	return f, nil
}

// runRecordingsCmd handles: funnelbarn recordings scan-url-params
//
// scan-url-params reads stored recording chunks and prints, per project, how
// many chunks hold a URL with a sensitive parameter value (see
// enrich.HasSensitiveURLInJSON). It opens the database read-only and only
// reads object storage. The output is counts and times: no keys, no event
// content. With --by-url each project line also lists "shapes": the host,
// the path with identifier-like segments masked (enrich.MaskPath) and the
// parameter name of each matching URL, with chunk and recording counts. No
// parameter value is printed. The process runs under a Go memory limit of --max-memory-mib and
// stops, printing the counts so far, when it holds more than that.
func runRecordingsCmd(cfg config.Config, args []string) error {
	if len(args) == 0 || args[0] != "scan-url-params" {
		return errors.New(recordingsUsage)
	}
	flags, err := parseScanFlags(args[1:])
	if err != nil {
		return err
	}
	if cfg.R2Endpoint == "" || cfg.R2AccessKeyID == "" || cfg.R2SecretAccessKey == "" || cfg.R2Bucket == "" {
		return fmt.Errorf("recordings scan: object storage is not configured")
	}
	store, err := repository.OpenReadOnly(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	r2, err := storage.NewR2(cfg.R2Endpoint, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2Bucket)
	if err != nil {
		return fmt.Errorf("recordings scan: object storage: %w", err)
	}

	memCap := flags.maxMemoryMiB << 20
	debug.SetMemoryLimit(int64(memCap))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	opts := scanOptions{project: flags.project, chunk: service.ChunkScanOpts{BatchSize: 25, MemoryCap: memCap, Shapes: flags.byURL}}
	return scanRecordingURLParams(ctx, os.Stdout, store, service.NewRecordingService(store, store, store, r2), opts)
}

// scanOptions selects the projects to scan and bounds each scan.
type scanOptions struct {
	project string // a slug; empty scans every project
	chunk   service.ChunkScanOpts
}

// projectLister is the one store method the scan needs besides the service.
type projectLister interface {
	ListProjects(ctx context.Context) ([]repository.Project, error)
}

type chunkScanner interface {
	ScanChunkURLs(ctx context.Context, projectID string, opts service.ChunkScanOpts) (service.ChunkURLScan, error)
}

// scanLine is one line of output: a project's counts, or the total.
type scanLine struct {
	Project string `json:"project"`
	Stopped string `json:"stopped,omitempty"`
	service.ChunkURLScan
}

// selectProjects returns every project, or the one whose slug is wanted.
func selectProjects(ctx context.Context, projects projectLister, slug string) ([]repository.Project, error) {
	list, err := projects.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("recordings scan: list projects: %w", err)
	}
	if slug == "" {
		return list, nil
	}
	for _, p := range list {
		if p.Slug == slug {
			return []repository.Project{p}, nil
		}
	}
	return nil, fmt.Errorf("recordings scan: no project with slug %q", slug)
}

// scanRecordingURLParams writes one JSON line per project that has
// recordings, then a total line. A scan that stops at its memory cap still
// writes the counts it has, marked "stopped", and then returns the error.
func scanRecordingURLParams(ctx context.Context, w io.Writer, projects projectLister, scanner chunkScanner, opts scanOptions) error {
	list, err := selectProjects(ctx, projects, opts.project)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	var total service.ChunkURLScan
	for _, p := range list {
		res, err := scanner.ScanChunkURLs(ctx, p.ID, opts.chunk)
		if errors.Is(err, service.ErrMemoryCap) {
			addScan(&total, res)
			_ = enc.Encode(scanLine{Project: p.Slug, Stopped: "memory cap", ChunkURLScan: res})
			_ = enc.Encode(scanLine{Project: "(total)", Stopped: "memory cap", ChunkURLScan: total})
			return fmt.Errorf("recordings scan: project %s: %w", p.Slug, err)
		}
		if err != nil {
			return fmt.Errorf("recordings scan: project %s: %w", p.Slug, err)
		}
		if res.Recordings == 0 {
			continue
		}
		if err := enc.Encode(scanLine{Project: p.Slug, ChunkURLScan: res}); err != nil {
			return err
		}
		addScan(&total, res)
	}
	return enc.Encode(scanLine{Project: "(total)", ChunkURLScan: total})
}

// addScan adds one project's counts to the total.
func addScan(total *service.ChunkURLScan, res service.ChunkURLScan) {
	total.Recordings += res.Recordings
	total.Chunks += res.Chunks
	total.Unreadable += res.Unreadable
	total.MatchingChunks += res.MatchingChunks
	total.MatchingRecordings += res.MatchingRecordings
	if res.FirstMatch != nil && (total.FirstMatch == nil || res.FirstMatch.Before(*total.FirstMatch)) {
		total.FirstMatch = res.FirstMatch
	}
	if res.LastMatch != nil && (total.LastMatch == nil || res.LastMatch.After(*total.LastMatch)) {
		total.LastMatch = res.LastMatch
	}
	if res.PeakMemoryBytes > total.PeakMemoryBytes {
		total.PeakMemoryBytes = res.PeakMemoryBytes
	}
}
