package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
	"github.com/wiebe-xyz/funnelbarn/internal/volumedrop"
)

const (
	// volumeDropInterval is how often the worker looks for a new clock hour. Each
	// hour is evaluated once.
	volumeDropInterval = 10 * time.Minute

	// volumeDropOpenKey is the instance setting that lists the projects whose drop
	// is open, as JSON. It is a setting because a release with a migration gets no
	// automatic rollback.
	volumeDropOpenKey = "volume_drop_open"

	// volumeDropLastKey is the instance setting holding the last clock hour
	// evaluated, in volumedrop.HourFormat, so the hours a failed read or a restart
	// missed are evaluated on the next good read.
	volumeDropLastKey = "volume_drop_last_hour"

	// volumeDropCatchUp is how many hours one good read evaluates. Older missed
	// hours are reported as skipped.
	volumeDropCatchUp = 24

	// volumeDropStallAfter is how long the check may go without evaluating an hour
	// before it logs an Error.
	volumeDropStallAfter = 3 * time.Hour
)

// volumeDropTimeout bounds one evaluation. The check shares the worker loop with
// the spool drain and its stall alarm, so a read waiting on a busy pool must not
// hold them. It is a variable so a test can expire it.
var volumeDropTimeout = 30 * time.Second

// volumeDropCheck logs one Error per project at the start of a volume drop, which
// opens its BugBarn issue. lastHour only stops repeat evaluations within an hour;
// the open drops in the instance setting stop repeat lines, across restarts too.
// failingSince and stallReported belong to the stall alarm.
type volumeDropCheck struct {
	store         *repository.Store
	now           func() time.Time
	lastHour      time.Time
	failingSince  time.Time
	stallReported bool
}

func newVolumeDropCheck(store *repository.Store) *volumeDropCheck {
	return &volumeDropCheck{store: store, now: time.Now}
}

// run evaluates every clock hour since the last one evaluated, up to the current
// hour, unless it already did this hour. Test and development rows follow deploys
// and agent work, so only production rows count. A failed read logs Warn and
// leaves the hours for the next interval; the Error comes from the stall alarm.
// The lines go out before the settings are saved, so a failed save repeats a line
// and loses none.
func (c *volumeDropCheck) run(ctx context.Context) {
	now := c.now().UTC().Truncate(time.Hour)
	if now.Equal(c.lastHour) {
		return
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, volumeDropTimeout)
	defer cancel()
	ctx, span := tracing.StartSpan(ctx, "worker.volume_drop")
	defer span.End()

	last, err := c.evaluate(ctx, span, now)
	if err != nil {
		if parent.Err() != nil {
			// The process is shutting down. The hour stays open and the next
			// process evaluates it; this is no failed evaluation.
			return
		}
		tracing.RecordError(span, err)
		c.failed(last, err)
		return
	}
	c.lastHour = now
	c.failingSince = time.Time{}
	c.stallReported = false
}

// evaluate reads the state and the hourly counts and evaluates the pending
// hours. On a failed read it returns the last hour evaluated, zero if unknown.
func (c *volumeDropCheck) evaluate(ctx context.Context, span trace.Span, now time.Time) (time.Time, error) {
	open, err := c.readOpen(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("read open drops: %w", err)
	}
	last, err := c.readLast(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("read last hour: %w", err)
	}
	pending, skipped := pendingHours(last, now)
	rows, err := c.store.EnvironmentEventsByHour(ctx, environment.Production, volumedrop.Since(pending[0]))
	if err != nil {
		return last, fmt.Errorf("read hourly event counts: %w", err)
	}
	if skipped > 0 {
		slog.Error("volume drop: hours skipped past the catch-up limit", "hours", skipped,
			"first_evaluated", pending[0].Format(volumedrop.HourFormat))
	}

	hours := make(map[string]map[string]int64)
	for _, r := range rows {
		if hours[r.Project] == nil {
			hours[r.Project] = make(map[string]int64)
		}
		hours[r.Project][r.Hour] = r.Count
	}
	stillOpen := open
	drops := 0
	for _, h := range pending {
		var started []volumedrop.Drop
		started, stillOpen = volumedrop.Evaluate(hours, stillOpen, h)
		drops += len(started)
		for _, d := range started {
			// The project is in the message so each gets its own BugBarn issue.
			slog.Error(fmt.Sprintf("volume drop: %s production events under a tenth of the 7-day baseline", d.Project),
				"project", d.Project, "baseline_24h", d.Baseline, "current_24h", d.Current,
				"hour", h.Format(volumedrop.HourFormat))
		}
	}
	span.SetAttributes(attribute.Int("projects", len(hours)), attribute.Int("hours", len(pending)),
		attribute.Int("skipped", skipped), attribute.Int("drops", drops), attribute.Int("open", len(stillOpen)))
	c.save(ctx, open, stillOpen, now)
	return now, nil
}

// pendingHours returns the hours after last up to now, oldest first, and how many
// older ones fall past volumeDropCatchUp. With no last hour only now is pending.
func pendingHours(last, now time.Time) (pending []time.Time, skipped int) {
	first := now
	if !last.IsZero() && last.Before(now) {
		first = last.Add(time.Hour)
	}
	if n := int(now.Sub(first)/time.Hour) + 1; n > volumeDropCatchUp {
		skipped = n - volumeDropCatchUp
		first = now.Add(-(volumeDropCatchUp - 1) * time.Hour)
	}
	for h := first; !h.After(now); h = h.Add(time.Hour) {
		pending = append(pending, h)
	}
	return pending, skipped
}

// failed logs a failed evaluation at Warn, and an Error once when no hour has
// been evaluated for volumeDropStallAfter. The outage starts at the hour after
// last, or at this process's first failure when last is unknown or later.
func (c *volumeDropCheck) failed(last time.Time, err error) {
	slog.Warn("volume drop: evaluation failed, retrying next interval", "err", err)
	now := c.now().UTC()
	if c.failingSince.IsZero() {
		c.failingSince = now
	}
	since := c.failingSince
	if !last.IsZero() && last.Add(time.Hour).Before(since) {
		since = last.Add(time.Hour)
	}
	if c.stallReported || now.Sub(since) < volumeDropStallAfter {
		return
	}
	c.stallReported = true
	slog.Error("volume drop: check has evaluated no hour for 3 hours",
		"since", since.Format(time.RFC3339), "err", err)
}

// save stores the open drops when they changed and the hour just evaluated.
func (c *volumeDropCheck) save(ctx context.Context, open, stillOpen []string, now time.Time) {
	if !slices.Equal(open, stillOpen) {
		if err := c.writeOpen(ctx, stillOpen); err != nil {
			slog.Warn("volume drop: save open drops", "err", err)
		}
	}
	if err := c.store.SetInstanceSetting(ctx, volumeDropLastKey, now.Format(volumedrop.HourFormat)); err != nil {
		slog.Warn("volume drop: save last hour", "err", err)
	}
}

// readLast returns the last hour evaluated, zero when none is stored. A value it
// cannot parse counts as none, which evaluates only the current hour.
func (c *volumeDropCheck) readLast(ctx context.Context) (time.Time, error) {
	value, ok, err := c.store.GetInstanceSetting(ctx, volumeDropLastKey)
	if err != nil || !ok {
		return time.Time{}, err
	}
	last, err := time.ParseInLocation(volumedrop.HourFormat, value, time.UTC)
	if err != nil {
		slog.Warn("volume drop: ignoring unreadable last hour", "err", err)
		return time.Time{}, nil
	}
	return last, nil
}

// readOpen returns the projects whose drop is open. A value it cannot parse counts
// as none open, which repeats a line for each drop that is still open.
func (c *volumeDropCheck) readOpen(ctx context.Context) ([]string, error) {
	value, ok, err := c.store.GetInstanceSetting(ctx, volumeDropOpenKey)
	if err != nil || !ok {
		return nil, err
	}
	var open []string
	if err := json.Unmarshal([]byte(value), &open); err != nil {
		slog.Warn("volume drop: ignoring unreadable open drops", "err", err)
		return nil, nil
	}
	return open, nil
}

func (c *volumeDropCheck) writeOpen(ctx context.Context, open []string) error {
	if open == nil {
		open = []string{}
	}
	value, err := json.Marshal(open)
	if err != nil {
		return err
	}
	return c.store.SetInstanceSetting(ctx, volumeDropOpenKey, string(value))
}
