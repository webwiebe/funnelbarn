package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/geoip"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
	"github.com/wiebe-xyz/funnelbarn/internal/worker"
	"github.com/wiebe-xyz/funnelbarn/internal/workerhealth"
)

// errProcessRecord marks a record whose payload could not be turned into an
// event. It is retried like a persist failure, under its own log message.
var errProcessRecord = errors.New("process record")

// ingestApplier turns one ingest record into stored rows. The spool worker
// calls it today; a queue consumer calls the same code, so both paths store a
// record the same way. Applying a record twice stores it once (PersistEvent
// skips a known ingest_id), so a redelivery is safe.
type ingestApplier struct {
	store  *repository.Store
	geo    *geoip.Lookup
	health *workerhealth.Monitor
}

// apply stores one record. geoEnabled is read once per batch by the caller.
//
// The error says what the caller should do with the record:
//   - nil: stored, or skipped as a duplicate.
//   - wraps repository.ErrProjectUnresolvable: it can never apply; dead-letter
//     it now instead of retrying.
//   - wraps errProcessRecord, or any other error: retry it.
func (a *ingestApplier) apply(ctx context.Context, record spool.Record, geoEnabled bool) error {
	event, err := worker.SafeProcess(record)
	if err != nil {
		return fmt.Errorf("%w: %w", errProcessRecord, err)
	}

	eventStart := time.Now()

	// Each DB operation gets a 30s timeout so a stuck write can't block the worker.
	opCtx, opCancel := context.WithTimeout(ctx, 30*time.Second)
	defer opCancel()
	opCtx, span := tracing.StartSpan(opCtx, "worker.persist_event",
		attribute.String("ingest.id", record.IngestID),
		attribute.String("project.slug", record.ProjectSlug),
	)
	defer span.End()

	// Resolve project from the slug stored in the spool record.
	projectID, projectErr := resolveEventProject(opCtx, a.store, record.ProjectSlug)
	switch {
	case projectErr == nil:
		event.ProjectID = projectID
	case errors.Is(projectErr, repository.ErrProjectUnresolvable):
		// Unrecoverable: retrying an insert that must fail the
		// project_id foreign key only burns attempts.
		tracing.RecordError(span, projectErr)
		slog.Error("worker dead-lettering record: project cannot be resolved",
			"err", projectErr, "handled", false,
			"ingest_id", record.IngestID,
			"project_slug", record.ProjectSlug,
			"event_name", event.Name,
		)
		return projectErr
	default:
		// A transient lookup failure (DB busy, timeout) — leave
		// ProjectID unset and let the persist path retry the record.
		slog.Warn("worker ensure project", "slug", record.ProjectSlug, "err", projectErr, "handled", true)
	}

	var geoResult *geoip.GeoResult
	if geoEnabled {
		geoResult = a.geo.Lookup(event.ClientIP)
		// Geo is on but resolving nothing usually means the real
		// client IP isn't reaching us (proxy/forwarding config).
		hit := geoResult != nil && geoResult.CountryCode != ""
		metrics.GeoLookups.Inc()
		if hit {
			metrics.GeoHits.Inc()
		}
		if alert, n := a.health.RecordGeo(hit); alert {
			slog.Error("geo enrichment resolved 0 countries over recent lookups; check FUNNELBARN_TRUSTED_PROXIES and client-IP forwarding",
				"handled", false, "lookups", n)
		}
	}

	err = worker.PersistEvent(opCtx, a.store, event, geoResult)
	metrics.EventProcessingDuration.Observe(time.Since(eventStart).Seconds())
	if err != nil {
		tracing.RecordError(span, err)
		return err
	}
	return nil
}
