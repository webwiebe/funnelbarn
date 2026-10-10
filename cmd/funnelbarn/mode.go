package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
	"github.com/wiebe-xyz/funnelbarn/internal/workerhealth"
)

// FUNNELBARN_MODE splits one process into readers and a writer (spec 012).
// A standalone process and a writer open the database read-write and consume
// every queue they use; a reader opens it read-only and publishes every write
// to Valkey, buffering on disk while Valkey is down.

// isReader reports whether this process runs as a reader.
func isReader(cfg config.Config) bool { return cfg.Mode == config.ModeReader }

// isSplit reports whether this process is one half of a reader/writer pair.
// Both halves move ingest and recordings through Valkey, whatever the
// per-queue flags say.
func isSplit(cfg config.Config) bool {
	return cfg.Mode == config.ModeReader || cfg.Mode == config.ModeWriter
}

// openStore opens the database for this process's role.
func openStore(cfg config.Config) (*repository.Store, error) {
	if isReader(cfg) {
		return repository.OpenReadOnly(cfg.DBPath)
	}
	return repository.Open(cfg.DBPath)
}

// markProjectHealth applies a MarkProjectHealth command through the health
// service, which skips the write once a field is known to be set.
func markProjectHealth(h service.ProjectHealth) func(ctx context.Context, projectID, field string) error {
	return func(ctx context.Context, projectID, field string) error {
		switch field {
		case command.HealthSetupCalled:
			return h.MarkSetupCalled(ctx, projectID)
		case command.HealthEventsReceived:
			_, err := h.MarkEventsReceived(ctx, projectID)
			return err
		case command.HealthRecordingsReceived:
			return h.MarkRecordingsReceived(ctx, projectID)
		default:
			return fmt.Errorf("%w: unknown project health field %q", command.ErrPermanent, field)
		}
	}
}

// healthViaCommands is a reader's project health port. Reads go to the
// read-only database; each mark becomes a command for the writer.
type healthViaCommands struct {
	ports.ProjectHealthRepo
	bus command.Bus
}

func (h healthViaCommands) MarkProjectHealthSetupCalled(ctx context.Context, projectID string) error {
	h.bus.Submit(ctx, command.MarkProjectHealth{ProjectID: projectID, Field: command.HealthSetupCalled})
	return nil
}

func (h healthViaCommands) MarkProjectHealthEventsReceived(ctx context.Context, projectID string) error {
	h.bus.Submit(ctx, command.MarkProjectHealth{ProjectID: projectID, Field: command.HealthEventsReceived})
	return nil
}

func (h healthViaCommands) MarkProjectHealthRecordingsReceived(ctx context.Context, projectID string) error {
	h.bus.Submit(ctx, command.MarkProjectHealth{ProjectID: projectID, Field: command.HealthRecordingsReceived})
	return nil
}

func (h healthViaCommands) MarkProjectHealthFlagsEvaluated(ctx context.Context, projectID string) error {
	h.bus.Submit(ctx, command.MarkFlagsEvaluated{ProjectID: projectID})
	return nil
}

// schemaGate keeps a reader out of the Service until the writer has migrated
// the database to the version this binary was built for.
type schemaGate struct {
	store *repository.ReadStore
	want  int64
	ok    atomic.Bool
}

func newSchemaGate(store *repository.ReadStore) (*schemaGate, error) {
	want, err := repository.EmbeddedSchemaVersion()
	if err != nil {
		return nil, err
	}
	return &schemaGate{store: store, want: want}, nil
}

// Ready returns an error while the database schema is older than the
// binary's. Once it has passed it stays passed: a schema does not go back.
func (g *schemaGate) Ready(ctx context.Context) error {
	if g.ok.Load() {
		return nil
	}
	have, err := g.store.SchemaVersion(ctx)
	if err != nil {
		return err
	}
	if have < g.want {
		return fmt.Errorf("database schema version %d is older than this binary's %d; waiting for the writer to migrate", have, g.want)
	}
	g.ok.Store(true)
	return nil
}

// batchPublisher is the part of queue.RedisList a fallback forwarder uses.
type batchPublisher interface {
	PublishBatch(ctx context.Context, payloads [][]byte) error
}

// fallbackForwarder publishes what a reader buffered in a SpoolFallback while
// Valkey could not be reached.
type fallbackForwarder struct {
	name     string
	fallback *command.SpoolFallback
	list     batchPublisher
	failing  bool // the last forward failed; logged once per outage
}

// forward runs on the reader's worker tick.
func (f *fallbackForwarder) forward(ctx context.Context) {
	pubCtx, cancel := context.WithTimeout(ctx, command.DefaultPublishTimeout)
	defer cancel()
	n, err := f.fallback.Forward(pubCtx, f.list.PublishBatch)
	if err != nil {
		if !f.failing {
			f.failing = true
			slog.WarnContext(ctx, f.name+": forwarding buffered commands failed, keeping them on disk",
				"err", err, "handled", true)
		}
		return
	}
	if f.failing {
		f.failing = false
		slog.InfoContext(ctx, f.name+": forwarding buffered commands recovered")
	}
	if n > 0 {
		slog.InfoContext(ctx, f.name+": forwarded buffered commands", "count", n)
	}
}

// runReaderWorker is a reader's background loop. It forwards spool records to
// the ingest queue and buffered commands to theirs. It runs no maintenance:
// that needs the write connection, which only the writer has.
func runReaderWorker(ctx context.Context, cfg config.Config, eventSpool *spool.Spool, ingestQ *ingestQueue, fallbacks []*fallbackForwarder) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	offset, err := spool.ReadCursor(cfg.SpoolDir)
	if err != nil {
		slog.Warn("reader: failed to read cursor, starting from 0", "err", err)
		offset = 0
	}
	health := workerhealth.New(workerhealth.Options{})

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
				offset = ingestQ.forwarder.forward(tickCtx, entries, offset)
			}
			checkSpoolProgress(health, cfg.SpoolDir, offset)
			for _, f := range fallbacks {
				f.forward(tickCtx)
			}
			rotateSpool(tickCtx, eventSpool, tickSpan)
			tickSpan.End()
		}
	}
}
