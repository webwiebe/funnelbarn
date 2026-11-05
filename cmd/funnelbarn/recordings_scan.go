package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/storage"
)

// runRecordingsCmd handles: funnelbarn recordings scan-url-params
//
// scan-url-params reads every stored recording chunk and prints, per project,
// how many chunks hold a URL with a sensitive parameter value (see
// enrich.RedactURLsInJSON). It opens the database read-only and only reads
// object storage. The output is counts and times: no keys, no event content.
func runRecordingsCmd(cfg config.Config, args []string) error {
	if len(args) == 0 || args[0] != "scan-url-params" {
		return fmt.Errorf("usage: funnelbarn recordings scan-url-params")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return scanRecordingURLParams(ctx, os.Stdout, store, service.NewRecordingService(store, store, store, r2))
}

// projectLister is the one store method the scan needs besides the service.
type projectLister interface {
	ListProjects(ctx context.Context) ([]repository.Project, error)
}

type chunkScanner interface {
	ScanChunkURLs(ctx context.Context, projectID string) (service.ChunkURLScan, error)
}

// scanRecordingURLParams writes one JSON line per project that has
// recordings, then a total line.
func scanRecordingURLParams(ctx context.Context, w io.Writer, projects projectLister, scanner chunkScanner) error {
	list, err := projects.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("recordings scan: list projects: %w", err)
	}
	enc := json.NewEncoder(w)
	var total service.ChunkURLScan
	for _, p := range list {
		res, err := scanner.ScanChunkURLs(ctx, p.ID)
		if err != nil {
			return fmt.Errorf("recordings scan: project %s: %w", p.Slug, err)
		}
		if res.Recordings == 0 {
			continue
		}
		if err := enc.Encode(struct {
			Project string `json:"project"`
			service.ChunkURLScan
		}{p.Slug, res}); err != nil {
			return err
		}
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
	}
	return enc.Encode(struct {
		Project string `json:"project"`
		service.ChunkURLScan
	}{"(total)", total})
}
