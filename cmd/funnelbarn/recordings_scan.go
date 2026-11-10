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
	"runtime/debug"
	"syscall"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/storage"
)

const recordingsUsage = "usage: funnelbarn recordings scan-url-params [--project <slug>] [--max-memory-mib <n>] [--by-url] [--max-duration <d>] [--fresh]"

// scanFlags are the options of recordings scan-url-params.
type scanFlags struct {
	project      string
	maxMemoryMiB uint64
	byURL        bool
	maxDuration  time.Duration
	fresh        bool
}

func parseScanFlags(args []string) (scanFlags, error) {
	fs := flag.NewFlagSet("recordings scan-url-params", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var f scanFlags
	fs.StringVar(&f.project, "project", "", "scan only the project with this slug")
	fs.Uint64Var(&f.maxMemoryMiB, "max-memory-mib", 64, "stop when the scan holds more than this many MiB")
	fs.BoolVar(&f.byURL, "by-url", false, "also count matching URLs by host, masked path and parameter name")
	fs.DurationVar(&f.maxDuration, "max-duration", 0, "stop after this long, keep the totals and exit with an error (0 means no limit)")
	fs.BoolVar(&f.fresh, "fresh", false, "ignore the saved totals of finished projects and scan every project again")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if flags.maxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, flags.maxDuration)
		defer cancel()
	}
	opts := scanOptions{project: flags.project, chunk: service.ChunkScanOpts{BatchSize: 25, MemoryCap: memCap, Shapes: flags.byURL}, started: time.Now()}
	if flags.project == "" {
		// Resuming only makes sense for the whole run: a single-project scan is
		// a spot check and neither reads nor writes the saved totals.
		opts.state = &scanState{}
		if !flags.fresh {
			if opts.state, err = loadScanState(ctx, r2); err != nil {
				return err
			}
		}
		opts.save = func(st *scanState) error { return saveScanState(context.WithoutCancel(ctx), r2, st) }
	}
	return scanRecordingURLParams(ctx, os.Stdout, store, service.NewRecordingService(store, store, store, r2), opts)
}

// scanOptions selects the projects to scan and bounds each scan.
type scanOptions struct {
	project string // a slug; empty scans every project
	chunk   service.ChunkScanOpts
	started time.Time // when the run began; zero leaves elapsed_s out of the lines
	// state holds the projects an earlier run finished; they are not scanned
	// again. save writes it after each project. Both are nil for a spot check.
	state *scanState
	save  func(*scanState) error
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
	// Progress marks a partial count printed while the project is still being
	// read. The project's final line follows, unless the run is cut off first.
	Progress bool   `json:"progress,omitempty"`
	Stopped  string `json:"stopped,omitempty"`
	// FromPreviousRun marks the final line of a project that an earlier run
	// finished; this run did not read it again.
	FromPreviousRun bool `json:"from_previous_run,omitempty"`
	// ElapsedSeconds is the time since the run began, to read a rate from.
	ElapsedSeconds int `json:"elapsed_s,omitempty"`
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
// While a project is read, progress lines carry the counts so far, so a run
// that is killed still leaves partial results.
func scanRecordingURLParams(ctx context.Context, w io.Writer, projects projectLister, scanner chunkScanner, opts scanOptions) error {
	list, err := selectProjects(ctx, projects, opts.project)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	elapsed := func() int {
		if opts.started.IsZero() {
			return 0
		}
		return int(time.Since(opts.started).Seconds())
	}
	var total service.ChunkURLScan
	for _, p := range list {
		if prev, ok := opts.state.done(p.Slug); ok {
			if prev.Recordings > 0 {
				prev.FromPreviousRun = true
				if err := enc.Encode(prev); err != nil {
					return err
				}
				addScan(&total, prev.ChunkURLScan)
			}
			continue
		}
		chunk := opts.chunk
		chunk.Progress = func(part service.ChunkURLScan) {
			_ = enc.Encode(scanLine{Project: p.Slug, Progress: true, ElapsedSeconds: elapsed(), ChunkURLScan: part})
		}
		res, err := scanner.ScanChunkURLs(ctx, p.ID, chunk)
		if why := stopReason(err); why != "" {
			addScan(&total, res)
			_ = enc.Encode(scanLine{Project: p.Slug, Stopped: why, ElapsedSeconds: elapsed(), ChunkURLScan: res})
			_ = enc.Encode(scanLine{Project: "(total)", Stopped: why, ElapsedSeconds: elapsed(), ChunkURLScan: total})
			return fmt.Errorf("recordings scan: project %s: %w", p.Slug, err)
		}
		if err != nil {
			return fmt.Errorf("recordings scan: project %s: %w", p.Slug, err)
		}
		final := scanLine{Project: p.Slug, ElapsedSeconds: elapsed(), ChunkURLScan: res}
		if res.Recordings > 0 {
			if err := enc.Encode(final); err != nil {
				return err
			}
			addScan(&total, res)
		}
		saveFinished(opts, enc, p.Slug, final)
	}
	return enc.Encode(scanLine{Project: "(total)", ElapsedSeconds: elapsed(), ChunkURLScan: total})
}

// saveFinished keeps a finished project's line in the state. A failed save does
// not stop the scan: the line has been printed, and the next run reads the
// project again.
func saveFinished(opts scanOptions, enc *json.Encoder, slug string, line scanLine) {
	if opts.state == nil || opts.save == nil {
		return
	}
	opts.state.record(slug, line)
	if err := opts.save(opts.state); err != nil {
		slog.Warn("recordings scan: could not save the finished project", "project", slug, "err", err)
		_ = enc.Encode(map[string]string{"project": slug, "warning": "state not saved"})
	}
}

// stopReason names why a scan ended early, or returns "" for a real failure
// or no error. The memory cap and a signal (SIGTERM at the Job's deadline)
// both leave the counts so far.
func stopReason(err error) string {
	switch {
	case errors.Is(err, service.ErrMemoryCap):
		return "memory cap"
	case errors.Is(err, context.Canceled):
		return "interrupted"
	case errors.Is(err, context.DeadlineExceeded):
		return "time budget"
	}
	return ""
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
