package main

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

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
	// Sessions past their absolute cap are already unusable (the
	// middleware enforces absolute_expires_at); this keeps the table
	// from accumulating.
	if n, err := store.DeleteExpiredWebSessions(purgeCtx, time.Now().UTC()); err != nil {
		tracing.RecordError(purgeSpan, err)
		slog.Error("purge expired web sessions", "err", err)
	} else if n > 0 {
		slog.Info("purged expired web sessions", "count", n)
	}
	if cfg.EventRetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -cfg.EventRetentionDays)
		n, err := store.PurgeOldEvents(purgeCtx, cutoff)
		if err != nil {
			tracing.RecordError(purgeSpan, err)
			slog.Error("purge old events", "err", err)
		} else if n > 0 {
			slog.Info("purged old events", "count", n, "before", cutoff.Format(time.DateOnly))
			metrics.EventsPurged.Add(float64(n))
		}
		ne, err := store.PurgeOldEvaluations(purgeCtx, cutoff)
		if err != nil {
			tracing.RecordError(purgeSpan, err)
			slog.Error("purge old evaluations", "err", err)
		} else if ne > 0 {
			slog.Info("purged old evaluations", "count", ne, "before", cutoff.Format(time.DateOnly))
		}
	}
	if cfg.AutoRegisterTTLDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -cfg.AutoRegisterTTLDays)
		nf, err := store.PurgeStaleAutoFlags(purgeCtx, cutoff)
		if err != nil {
			tracing.RecordError(purgeSpan, err)
			slog.Error("purge stale auto flags", "err", err)
		} else if nf > 0 {
			slog.Info("purged stale auto flags", "count", nf, "before", cutoff.Format(time.DateOnly))
		}
	}
	// Unreachable rows are invisible to the product by construction —
	// every read is project-scoped — so nothing surfaces them except a
	// count. The 344 events that prompted this check went unnoticed for
	// two months. Error level so it reaches BugBarn as an issue: with
	// foreign keys enforced and the project_id triggers in place, a
	// non-zero count means a guard was bypassed.
	if orphans, err := store.CountOrphanedRows(purgeCtx); err != nil {
		tracing.RecordError(purgeSpan, err)
		slog.Error("count orphaned rows", "err", err)
	} else if orphans.Total() > 0 {
		slog.Error("rows exist under a project_id that matches no project; they are unreachable by every query",
			"err", errors.New("orphaned rows present"), "handled", false,
			"events", orphans.Events,
			"sessions", orphans.Sessions,
			"funnels", orphans.Funnels,
			"api_keys", orphans.APIKeys,
		)
	}

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
		retentionDays := 90 // default
		if v, ok, _ := store.GetInstanceSetting(purgeCtx, "recording_retention_days"); ok {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				retentionDays = n
			}
		}
		if err := recordings.PurgeOldRecordings(purgeCtx, retentionDays); err != nil {
			tracing.RecordError(purgeSpan, err)
			slog.Error("purge old recordings", "err", err)
		} else {
			slog.Debug("recording retention purge complete", "retention_days", retentionDays)
		}
		if n, err := recordings.PurgeBrokenRecordings(purgeCtx); err != nil {
			tracing.RecordError(purgeSpan, err)
			slog.Error("purge broken recordings", "err", err)
		} else if n > 0 {
			slog.Info("purged unplayable recordings", "count", n)
		}
		// Bot recordings written before ingest started refusing them.
		// The rows could be dropped by a migration, but their R2 chunk
		// objects could not — deriving those keys needs the row.
		if n, err := recordings.PurgeBotRecordings(purgeCtx); err != nil {
			tracing.RecordError(purgeSpan, err)
			slog.Error("purge bot recordings", "err", err)
		} else if n > 0 {
			slog.Info("purged bot recordings", "count", n)
		}
	}
	purgeSpan.End()
}
