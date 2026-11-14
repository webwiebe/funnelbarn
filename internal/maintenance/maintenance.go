// Package maintenance holds the background jobs the writer runs beside the
// ingest worker: the daily retention and integrity pass, and the hourly check
// for a drop in production event volume.
package maintenance

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

// StartupDelay is how long after boot the first maintenance pass
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
const StartupDelay = 2 * time.Minute

// maintenanceRecordings is the narrow slice of service.Recordings the
// maintenance pass actually uses. Depending on the full fifteen-method
// interface would mean any test of this pass had to stub twelve unrelated
// methods, which is how a pass like this ends up untested.
type maintenanceRecordings interface {
	PurgeOldRecordings(ctx context.Context, retentionDays int) error
	PurgeBrokenRecordings(ctx context.Context) (int, error)
	PurgeBotRecordings(ctx context.Context) (int, error)
}

// Run executes one maintenance pass: retention purges, the recording
// sweeps, and the orphaned-row integrity check. Every step is independent and
// failure-tolerant — one failing purge logs and the rest still run — and every
// step is idempotent, so running the pass more often than daily is harmless.
func Run(ctx context.Context, cfg config.Config, store *repository.Store, recordings maintenanceRecordings) {
	purgeCtx, purgeSpan := tracing.StartSpan(ctx, "maintenance.purge")
	defer purgeSpan.End()
	p := &maintenancePass{ctx: purgeCtx, root: purgeSpan, store: store}

	p.purgeWebSessions()
	if cfg.EventRetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -cfg.EventRetentionDays)
		p.purgeEvents(cutoff)
		p.purgeEvaluations(cutoff)
	}
	if cfg.AutoRegisterTTLDays > 0 {
		p.purgeAutoFlags(time.Now().AddDate(0, 0, -cfg.AutoRegisterTTLDays))
	}
	p.checkOrphans()
	p.checkDuplicateIngestIDs()
	reportDeadLetter(cfg.SpoolDir)
	if recordings != nil {
		p.purgeRecordings(recordings)
	}
}

// maintenancePass is one run of the maintenance steps under a shared root span.
type maintenancePass struct {
	ctx   context.Context // carries the root span, so each step's span is its child
	root  trace.Span
	store *repository.Store
}

// step runs fn under its own child span so a slow pass shows which DELETE held
// the writer, with the row count it removed.
func (p *maintenancePass) step(name string, fn func(ctx context.Context, span trace.Span)) {
	stepCtx, span := tracing.StartSpan(p.ctx, "maintenance.purge."+name)
	defer span.End()
	fn(stepCtx, span)
}

// fail records err on the step's span and on the root span, and logs it.
func (p *maintenancePass) fail(span trace.Span, msg string, err error) {
	tracing.RecordError(span, err)
	tracing.RecordError(p.root, err)
	slog.Error(msg, "err", err)
}

// purgeBefore runs a purge that deletes rows older than cutoff. The step span
// carries the row count; a failure goes through fail and a non-zero count is
// logged with the cutoff date. after runs on a successful, non-empty purge.
func (p *maintenancePass) purgeBefore(name, what string, cutoff time.Time, purge func(ctx context.Context, cutoff time.Time) (int64, error), after func(n int64)) {
	p.step(name, func(c context.Context, span trace.Span) {
		n, err := purge(c, cutoff)
		span.SetAttributes(attribute.Int64("rows", n))
		if err != nil {
			p.fail(span, "purge "+what, err)
		} else if n > 0 {
			slog.Info("purged "+what, "count", n, "before", cutoff.Format(time.DateOnly))
			if after != nil {
				after(n)
			}
		}
	})
}

// Sessions past their absolute cap are already unusable (the
// middleware enforces absolute_expires_at); this keeps the table
// from accumulating.
func (p *maintenancePass) purgeWebSessions() {
	p.step("web_sessions", func(c context.Context, span trace.Span) {
		n, err := p.store.DeleteExpiredWebSessions(c, time.Now().UTC())
		span.SetAttributes(attribute.Int64("rows", n))
		if err != nil {
			p.fail(span, "purge expired web sessions", err)
		} else if n > 0 {
			slog.Info("purged expired web sessions", "count", n)
		}
	})
}

func (p *maintenancePass) purgeEvents(cutoff time.Time) {
	p.purgeBefore("events", "old events", cutoff, p.store.PurgeOldEvents,
		func(n int64) { metrics.EventsPurged.Add(float64(n)) })
}

func (p *maintenancePass) purgeEvaluations(cutoff time.Time) {
	p.purgeBefore("evaluations", "old evaluations", cutoff, p.store.PurgeOldEvaluations, nil)
}

func (p *maintenancePass) purgeAutoFlags(cutoff time.Time) {
	p.purgeBefore("auto_flags", "stale auto flags", cutoff, p.store.PurgeStaleAutoFlags, nil)
}

// Unreachable rows are invisible to the product by construction —
// every read is project-scoped — so nothing surfaces them except a
// count. The 344 events that prompted this check went unnoticed for
// two months. Error level so it reaches BugBarn as an issue: with
// foreign keys enforced and the project_id triggers in place, a
// non-zero count means a guard was bypassed.
func (p *maintenancePass) checkOrphans() {
	p.step("orphans", func(c context.Context, span trace.Span) {
		orphans, err := p.store.CountOrphanedRows(c)
		if err != nil {
			p.fail(span, "count orphaned rows", err)
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
}

// Ingest now skips an ingest_id that is already stored, so this count can
// only fall as old rows age out. Zero everywhere would allow a unique
// index on events.ingest_id.
func (p *maintenancePass) checkDuplicateIngestIDs() {
	p.step("duplicate_ingest_ids", func(c context.Context, span trace.Span) {
		n, err := p.store.CountDuplicateIngestIDs(c)
		if err != nil {
			p.fail(span, "count duplicate ingest ids", err)
			return
		}
		span.SetAttributes(attribute.Int64("rows", n))
		if n > 0 {
			slog.Info("events stored more than once before ingest deduplicated them", "ingest_ids", n)
		}
	})
}

// reportDeadLetter surfaces the dead-letter backlog. It is invisible otherwise — a file on a
// volume nobody looks at — and 108,343 records accumulated in it over two
// months before an audit found them.
func reportDeadLetter(spoolDir string) {
	size, err := spool.DeadLetterSize(spoolDir)
	if err != nil {
		slog.Warn("stat dead-letter file", "err", err, "handled", true)
		return
	}
	metrics.DeadLetterBytes.Set(float64(size))
	if size > 0 {
		slog.Info("dead-letter backlog", "bytes", size, "limit_bytes", spool.MaxDeadLetterBytes)
	}
}

// defaultRecordingRetentionDays applies when the instance setting is unset or
// not a positive integer.
const defaultRecordingRetentionDays = 90

// recordingRetentionDays reads the instance setting for recording retention.
func (p *maintenancePass) recordingRetentionDays(ctx context.Context) int {
	if v, ok, _ := p.store.GetInstanceSetting(ctx, "recording_retention_days"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultRecordingRetentionDays
}

func (p *maintenancePass) purgeRecordings(recordings maintenanceRecordings) {
	p.step("recordings", func(c context.Context, span trace.Span) {
		retentionDays := p.recordingRetentionDays(c)
		if err := recordings.PurgeOldRecordings(c, retentionDays); err != nil {
			p.fail(span, "purge old recordings", err)
		} else {
			slog.Debug("recording retention purge complete", "retention_days", retentionDays)
		}
		if n, err := recordings.PurgeBrokenRecordings(c); err != nil {
			p.fail(span, "purge broken recordings", err)
		} else if n > 0 {
			span.SetAttributes(attribute.Int("broken_rows", n))
			slog.Info("purged unplayable recordings", "count", n)
		}
		// Bot recordings written before ingest started refusing them.
		// The rows could be dropped by a migration, but their R2 chunk
		// objects could not — deriving those keys needs the row.
		if n, err := recordings.PurgeBotRecordings(c); err != nil {
			p.fail(span, "purge bot recordings", err)
		} else if n > 0 {
			span.SetAttributes(attribute.Int("bot_rows", n))
			slog.Info("purged bot recordings", "count", n)
		}
	})
}
