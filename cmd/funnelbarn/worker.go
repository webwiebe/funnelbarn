package main

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// startWorker starts the background worker for the process mode and returns
// the readiness gate for GET /api/v1/ready. A reader gets the worker that
// forwards its spool and fallback files to Valkey, and a gate that holds until
// the writer has migrated the database. Any other mode gets the full worker
// and no gate.
func startWorker(ctx context.Context, cfg config.Config, store *repository.Store, eventSpool *spool.Spool, applier *ingestApplier, ingestQ *ingestQueue, recordingsQ *recordingsQueue, recordings service.Recordings, commands command.Bus) (func(context.Context) error, error) {
	if !isReader(cfg) {
		bblog.Go("background-worker", func() {
			runBackgroundWorker(ctx, cfg, store, eventSpool, applier, ingestQ, recordings)
		})
		return nil, nil
	}

	gate, err := newSchemaGate(store.Reader())
	if err != nil {
		return nil, err
	}
	var fallbacks []*fallbackForwarder
	if rb, ok := commands.(*readerCommandBus); ok {
		fallbacks = append(fallbacks, rb.fallback)
	}
	if recordingsQ != nil && recordingsQ.fallback != nil {
		fallbacks = append(fallbacks, recordingsQ.fallback)
	}
	bblog.Go("reader-worker", func() {
		runReaderWorker(ctx, cfg, eventSpool, ingestQ, fallbacks)
	})
	return gate.Ready, nil
}

func runBackgroundWorker(ctx context.Context, cfg config.Config, store *repository.Store, eventSpool *spool.Spool, applier *ingestApplier, ingestQ *ingestQueue, recordings service.Recordings) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	purgeTicker := time.NewTicker(24 * time.Hour)
	defer purgeTicker.Stop()

	// A restart must not postpone maintenance indefinitely; see
	// startupMaintenanceDelay.
	startupMaintenance := time.NewTimer(startupMaintenanceDelay)
	defer startupMaintenance.Stop()

	offset, err := spool.ReadCursor(cfg.SpoolDir)
	if err != nil {
		slog.Warn("worker: failed to read cursor, starting from 0", "err", err)
		offset = 0
	}

	// Surface silent failure modes (stalled consumer, geo resolving nothing) as
	// BugBarn issues via slog.Error.
	health := applier.health

	retryCounts := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return
		case <-startupMaintenance.C:
			runMaintenance(ctx, cfg, store, recordings)
		case <-purgeTicker.C:
			runMaintenance(ctx, cfg, store, recordings)
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
				offset = ingestQ.forwarder.forward(tickCtx, entries, offset)
			} else {
				offset = applier.applyEntries(tickCtx, cfg.SpoolDir, entries, offset, retryCounts)
			}

			rotateSpool(tickCtx, eventSpool, tickSpan)
			tickSpan.End()
		}
	}
}
