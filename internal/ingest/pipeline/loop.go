package pipeline

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

const (
	// maxRetries is how many times a record may fail before it is dead-lettered.
	maxRetries = 3
	// rotateThreshold is the spool file size at which the worker rotates it.
	rotateThreshold = 64 << 20 // 64 MiB
)

// IsReader reports whether this process runs as a reader.
func IsReader(cfg config.Config) bool { return cfg.Mode == config.ModeReader }

// IsSplit reports whether this process is one half of a reader/writer pair.
// Both halves move ingest and recordings through Valkey, whatever the
// per-queue flags say.
func IsSplit(cfg config.Config) bool {
	return cfg.Mode == config.ModeReader || cfg.Mode == config.ModeWriter
}

// Jobs are the periodic jobs the full worker runs on its own loop, next to the
// spool drain. The caller supplies them so the loop stays free of their
// dependencies.
type Jobs struct {
	// Maintenance runs once StartupDelay after boot and then every 24 hours.
	Maintenance  func(ctx context.Context)
	StartupDelay time.Duration

	// VolumeDrop runs every VolumeDropEvery.
	VolumeDrop      func(ctx context.Context)
	VolumeDropEvery time.Duration
}

// RunWorker is the full worker's loop: it drains the spool into the database,
// or into the ingest queue when ingestQ is set, and runs jobs. It returns when
// ctx ends.
func RunWorker(ctx context.Context, cfg config.Config, eventSpool *spool.Spool, applier *Applier, ingestQ *IngestQueue, jobs Jobs) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	purgeTicker := time.NewTicker(24 * time.Hour)
	defer purgeTicker.Stop()

	// A restart must not postpone maintenance indefinitely; see
	// maintenance.StartupDelay.
	startupMaintenance := time.NewTimer(jobs.StartupDelay)
	defer startupMaintenance.Stop()

	volumeDropTicker := time.NewTicker(jobs.VolumeDropEvery)
	defer volumeDropTicker.Stop()

	offset, err := spool.ReadCursor(cfg.SpoolDir)
	if err != nil {
		slog.Warn("worker: failed to read cursor, starting from 0", "err", err)
		offset = 0
	}

	// Surface silent failure modes (stalled consumer, geo resolving nothing) as
	// BugBarn issues via slog.Error.
	health := applier.Health

	retryCounts := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return
		case <-startupMaintenance.C:
			jobs.Maintenance(ctx)
		case <-purgeTicker.C:
			jobs.Maintenance(ctx)
		case <-volumeDropTicker.C:
			jobs.VolumeDrop(ctx)
		case <-ticker.C:
			tickCtx, tickSpan := tracing.StartSpan(ctx, "worker.tick",
				attribute.Int64("spool.offset", offset),
			)

			entries, err := spool.ReadRecordsFrom(spool.Path(cfg.SpoolDir), offset)
			if err != nil {
				tracing.RecordError(tickSpan, err)
				tickSpan.End()
				slog.Error("worker read spool", "err", err)
				checkSpoolProgress(health, cfg.SpoolDir, offset)
				continue
			}
			tickSpan.SetAttributes(attribute.Int("spool.entries", len(entries)))
			metrics.SpoolQueueDepth.Set(float64(len(entries)))
			checkSpoolProgress(health, cfg.SpoolDir, offset)

			if ingestQ != nil {
				offset = ingestQ.Forwarder.Forward(tickCtx, entries, offset)
			} else {
				offset = applier.applyEntries(tickCtx, cfg.SpoolDir, entries, offset, retryCounts)
			}

			rotateSpool(tickCtx, eventSpool, tickSpan)
			tickSpan.End()
		}
	}
}

// RunReaderWorker is a reader's background loop. It forwards spool records to
// the ingest queue and buffered commands to theirs. It runs no maintenance:
// that needs the write connection, which only the writer has.
func RunReaderWorker(ctx context.Context, cfg config.Config, eventSpool *spool.Spool, ingestQ *IngestQueue, fallbacks []*FallbackForwarder) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	offset, err := spool.ReadCursor(cfg.SpoolDir)
	if err != nil {
		slog.Warn("reader: failed to read cursor, starting from 0", "err", err)
		offset = 0
	}
	health := NewMonitor(MonitorOptions{})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickCtx, tickSpan := tracing.StartSpan(ctx, "reader.tick",
				attribute.Int64("spool.offset", offset),
			)
			entries, err := spool.ReadRecordsFrom(spool.Path(cfg.SpoolDir), offset)
			if err != nil {
				tracing.RecordError(tickSpan, err)
				slog.ErrorContext(tickCtx, "reader read spool", "err", err)
			} else {
				tickSpan.SetAttributes(attribute.Int("spool.entries", len(entries)))
				metrics.SpoolQueueDepth.Set(float64(len(entries)))
				offset = ingestQ.Forwarder.Forward(tickCtx, entries, offset)
			}
			checkSpoolProgress(health, cfg.SpoolDir, offset)
			for _, f := range fallbacks {
				f.Forward(tickCtx)
			}
			rotateSpool(tickCtx, eventSpool, tickSpan)
			tickSpan.End()
		}
	}
}
