package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"

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
)

// volumeDropTimeout bounds one evaluation. The check shares the worker loop with
// the spool drain and its stall alarm, so a read waiting on a busy pool must not
// hold them. It is a variable so a test can expire it.
var volumeDropTimeout = 30 * time.Second

// volumeDropCheck logs one Error per project at the start of a volume drop, which
// opens its BugBarn issue. lastHour only stops repeat evaluations within an hour;
// the open drops in the instance setting stop repeat lines, across restarts too.
type volumeDropCheck struct {
	store    *repository.Store
	now      func() time.Time
	lastHour time.Time
}

func newVolumeDropCheck(store *repository.Store) *volumeDropCheck {
	return &volumeDropCheck{store: store, now: time.Now}
}

// run evaluates the current clock hour unless it already did. Test and
// development rows follow deploys and agent work, so only production rows count.
// A failed read leaves the hour open for the next interval. The line goes out
// before the setting is saved, so a failed save repeats a line and loses none.
func (c *volumeDropCheck) run(ctx context.Context) {
	now := c.now().UTC().Truncate(time.Hour)
	if now.Equal(c.lastHour) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, volumeDropTimeout)
	defer cancel()
	ctx, span := tracing.StartSpan(ctx, "worker.volume_drop")
	defer span.End()

	open, err := c.readOpen(ctx)
	if err != nil {
		tracing.RecordError(span, err)
		slog.Error("volume drop: read open drops", "err", err)
		return
	}
	rows, err := c.store.EnvironmentEventsByHour(ctx, environment.Production, volumedrop.Since(now))
	if err != nil {
		tracing.RecordError(span, err)
		slog.Error("volume drop: read hourly event counts", "err", err)
		return
	}
	c.lastHour = now

	hours := make(map[string]map[string]int64)
	for _, r := range rows {
		if hours[r.Project] == nil {
			hours[r.Project] = make(map[string]int64)
		}
		hours[r.Project][r.Hour] = r.Count
	}
	started, stillOpen := volumedrop.Evaluate(hours, open, now)
	span.SetAttributes(attribute.Int("projects", len(hours)), attribute.Int("drops", len(started)), attribute.Int("open", len(stillOpen)))
	for _, d := range started {
		// The project is in the message so each gets its own BugBarn issue.
		slog.Error(fmt.Sprintf("volume drop: %s production events under a tenth of the 7-day baseline", d.Project),
			"project", d.Project, "baseline_24h", d.Baseline, "current_24h", d.Current)
	}
	if slices.Equal(open, stillOpen) {
		return
	}
	if err := c.writeOpen(ctx, stillOpen); err != nil {
		tracing.RecordError(span, err)
		slog.Error("volume drop: save open drops", "err", err)
	}
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
