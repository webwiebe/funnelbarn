package main

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// startupMaintenanceDelay is how long after boot the first maintenance pass
// runs. The pass used to fire only on a 24h ticker with no initial run, so a
// redeploy reset the timer — and during active development the pod restarts
// more often than once a day, which meant the pass could go weeks without ever
// executing. Production accumulated 7,373 bot recordings that
// PurgeBotRecordings was never reached to sweep, and the orphan integrity
// check never ran at all.
//
// The delay keeps a potentially R2-heavy sweep off the boot path so the
// readiness probe settles first, and stops a crash-loop turning the pass into
// a hot loop of object-store deletes.
const startupMaintenanceDelay = 2 * time.Minute

// maintenanceRecordings is the narrow slice of service.Recordings the
// maintenance pass actually uses. Depending on the full fifteen-method
// interface would mean any test of this pass had to stub twelve unrelated
// methods, which is how a pass like this ends up untested.
type maintenanceRecordings interface {
	PurgeOldRecordings(ctx context.Context, retentionDays int) error
	PurgeBrokenRecordings(ctx context.Context) (int, error)
	PurgeBotRecordings(ctx context.Context) (int, error)
}

// runMaintenance executes one maintenance pass: retention purges, the recording
// sweeps, and the orphaned-row integrity check. Every step is independent and
// failure-tolerant — one failing purge logs and the rest still run — and every
// step is idempotent, so running the pass more often than daily is harmless.
func runMaintenance(ctx context.Context, cfg config.Config, store *repository.Store, recordings maintenanceRecordings) {
	purgeCtx, purgeSpan := tracing.StartSpan(ctx, "maintenance.purge")
	defer purgeSpan.End()

	// Each step runs under its own child span so a slow pass shows which
	// DELETE held the writer, with the row count it removed.
	step := func(name string, fn func(ctx context.Context, span trace.Span)) {
		stepCtx, span := tracing.StartSpan(purgeCtx, "maintenance.purge."+name)
		defer span.End()
		fn(stepCtx, span)
	}
	fail := func(span trace.Span, msg string, err error) {
		tracing.RecordError(span, err)
		tracing.RecordError(purgeSpan, err)
		slog.Error(msg, "err", err)
	}

	// Sessions past their absolute cap are already unusable (the
	// middleware enforces absolute_expires_at); this keeps the table
	// from accumulating.
	step("web_sessions", func(c context.Context, span trace.Span) {
		n, err := store.DeleteExpiredWebSessions(c, time.Now().UTC())
		span.SetAttributes(attribute.Int64("rows", n))
		if err != nil {
			fail(span, "purge expired web sessions", err)
		} else if n > 0 {
			slog.Info("purged expired web sessions", "count", n)
		}
	})
	if cfg.EventRetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -cfg.EventRetentionDays)
		step("events", func(c context.Context, span trace.Span) {
			n, err := store.PurgeOldEvents(c, cutoff)
			span.SetAttributes(attribute.Int64("rows", n))
			if err != nil {
				fail(span, "purge old events", err)
			} else if n > 0 {
				slog.Info("purged old events", "count", n, "before", cutoff.Format(time.DateOnly))
				metrics.EventsPurged.Add(float64(n))
			}
		})
		step("evaluations", func(c context.Context, span trace.Span) {
			n, err := store.PurgeOldEvaluations(c, cutoff)
			span.SetAttributes(attribute.Int64("rows", n))
			if err != nil {
				fail(span, "purge old evaluations", err)
			} else if n > 0 {
				slog.Info("purged old evaluations", "count", n, "before", cutoff.Format(time.DateOnly))
			}
		})
	}
	if cfg.AutoRegisterTTLDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -cfg.AutoRegisterTTLDays)
		step("auto_flags", func(c context.Context, span trace.Span) {
			n, err := store.PurgeStaleAutoFlags(c, cutoff)
			span.SetAttributes(attribute.Int64("rows", n))
			if err != nil {
				fail(span, "purge stale auto flags", err)
			} else if n > 0 {
				slog.Info("purged stale auto flags", "count", n, "before", cutoff.Format(time.DateOnly))
			}
		})
	}
	// Unreachable rows are invisible to the product by construction —
	// every read is project-scoped — so nothing surfaces them except a
	// count. The 344 events that prompted this check went unnoticed for
	// two months. Error level so it reaches BugBarn as an issue: with
	// foreign keys enforced and the project_id triggers in place, a
	// non-zero count means a guard was bypassed.
	step("orphans", func(c context.Context, span trace.Span) {
		orphans, err := store.CountOrphanedRows(c)
		if err != nil {
			fail(span, "count orphaned rows", err)
			return
		}
		span.SetAttributes(attribute.Int64("rows", int64(orphans.Total())))
		if orphans.Total() > 0 {
			slog.Error("rows exist under a project_id that matches no project; they are unreachable by every query",
				"err", errors.New("orphaned rows present"), "handled", false,
				"events", orphans.Events,
				"sessions", orphans.Sessions,
				"funnels", orphans.Funnels,
				"api_keys", orphans.APIKeys,
			)
		}
	})
	// Ingest now skips an ingest_id that is already stored, so this count can
	// only fall as old rows age out. Zero everywhere would allow a unique
	// index on events.ingest_id.
	step("duplicate_ingest_ids", func(c context.Context, span trace.Span) {
		n, err := store.CountDuplicateIngestIDs(c)
		if err != nil {
			fail(span, "count duplicate ingest ids", err)
			return
		}
		span.SetAttributes(attribute.Int64("rows", n))
		if n > 0 {
			slog.Info("events stored more than once before ingest deduplicated them", "ingest_ids", n)
		}
	})

	// Surface the dead-letter backlog. It is invisible otherwise — a file on a
	// volume nobody looks at — and 108,343 records accumulated in it over two
	// months before an audit found them.
	if size, err := spool.DeadLetterSize(cfg.SpoolDir); err != nil {
		slog.Warn("stat dead-letter file", "err", err, "handled", true)
	} else {
		metrics.DeadLetterBytes.Set(float64(size))
		if size > 0 {
			slog.Info("dead-letter backlog", "bytes", size, "limit_bytes", spool.MaxDeadLetterBytes)
		}
	}

	if recordings != nil {
		step("recordings", func(c context.Context, span trace.Span) {
			retentionDays := 90 // default
			if v, ok, _ := store.GetInstanceSetting(c, "recording_retention_days"); ok {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					retentionDays = n
				}
			}
			if err := recordings.PurgeOldRecordings(c, retentionDays); err != nil {
				fail(span, "purge old recordings", err)
			} else {
				slog.Debug("recording retention purge complete", "retention_days", retentionDays)
			}
			if n, err := recordings.PurgeBrokenRecordings(c); err != nil {
				fail(span, "purge broken recordings", err)
			} else if n > 0 {
				span.SetAttributes(attribute.Int("broken_rows", n))
				slog.Info("purged unplayable recordings", "count", n)
			}
			// Bot recordings written before ingest started refusing them.
			// The rows could be dropped by a migration, but their R2 chunk
			// objects could not — deriving those keys needs the row.
			if n, err := recordings.PurgeBotRecordings(c); err != nil {
				fail(span, "purge bot recordings", err)
			} else if n > 0 {
				span.SetAttributes(attribute.Int("bot_rows", n))
				slog.Info("purged bot recordings", "count", n)
			}
		})
	}
}
