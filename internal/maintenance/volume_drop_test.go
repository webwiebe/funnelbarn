package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
	"github.com/wiebe-xyz/funnelbarn/internal/volumedrop"
)

// captureDefaultLog routes the default logger to a buffer for one test.
func captureDefaultLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// seedHours writes perHour events of one environment in every hour from from up
// to but excluding to.
func seedHours(t *testing.T, store *repository.Store, projectID, env string, from, to time.Time, perHour int) {
	t.Helper()
	ctx := context.Background()
	for h := from; h.Before(to); h = h.Add(time.Hour) {
		for i := 0; i < perHour; i++ {
			id := fmt.Sprintf("%s-%s-%d-%d", projectID, env, h.Unix(), i)
			_, err := store.DB().ExecContext(ctx,
				`INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, created_at, environment) VALUES (?, ?, 's1', 'page_view', ?, ?, ?, ?)`,
				id, projectID, id, h, h.Format(time.DateTime), env)
			require.NoError(t, err)
		}
	}
}

// seedFallingProject gives a project 3 production events an hour for 9 days,
// ending 22 hours before start. Its baseline is 72, so it is low under 7.2, and
// the 6 events left in the window at start make that the first low hour.
func seedFallingProject(t *testing.T, store *repository.Store, start time.Time) repository.Project {
	t.Helper()
	p, err := store.CreateProject(context.Background(), "shop", "shop")
	require.NoError(t, err)
	seedHours(t, store, p.ID, environment.Production, start.Add(-10*24*time.Hour), start.Add(-22*time.Hour), 3)
	return p
}

func logsAtLevel(t *testing.T, logs *bytes.Buffer, level string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["level"] == level {
			out = append(out, rec)
		}
	}
	return out
}

func TestVolumeDropCheck_LogsErrorOncePerDrop(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	seedFallingProject(t, store, start)
	logs := captureDefaultLog(t)

	clock := start.Add(5 * time.Minute)
	check := &VolumeDropCheck{store: store, now: func() time.Time { return clock }}
	check.Run(context.Background())

	errs := logsAtLevel(t, logs, "ERROR")
	require.Len(t, errs, 1)
	require.Equal(t, "shop", errs[0]["project"])
	require.EqualValues(t, 72, errs[0]["baseline_24h"])
	require.EqualValues(t, 6, errs[0]["current_24h"])
	require.Contains(t, errs[0]["msg"], "shop", "the project must be in the message so each gets its own issue")

	// Same hour, same instance: no second evaluation.
	clock = start.Add(15 * time.Minute)
	check.Run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)

	// Next hour: the drop is running.
	clock = start.Add(time.Hour + 5*time.Minute)
	check.Run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)
}

// A restart in the hour the drop starts loses lastHour, and the setting still
// stops the repeat. So does a restart later in the drop.
func TestVolumeDropCheck_RestartDoesNotRepeat(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	seedFallingProject(t, store, start)
	logs := captureDefaultLog(t)

	for _, at := range []time.Duration{0, 10 * time.Minute, 3 * time.Hour} {
		clock := start.Add(at)
		check := &VolumeDropCheck{store: store, now: func() time.Time { return clock }}
		check.Run(context.Background())
	}

	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)
}

// A project that recovers closes its drop, so a later fall logs again.
func TestVolumeDropCheck_LogsAgainAfterRecovery(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	p := seedFallingProject(t, store, start)
	logs := captureDefaultLog(t)

	clock := start
	check := &VolumeDropCheck{store: store, now: func() time.Time { return clock }}
	check.Run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)

	// 30 events in the window against a baseline of 72: over a fifth, so it closes.
	seedHours(t, store, p.ID, environment.Production, start, start.Add(10*time.Hour), 3)
	clock = start.Add(10 * time.Hour)
	check.Run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)
	value, ok, err := store.GetInstanceSetting(context.Background(), volumeDropOpenKey)
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, `[]`, value)

	// 22 hours after the events stop again it has 6 left in the window.
	clock = start.Add(32 * time.Hour)
	check.Run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 2)
}

// An unreadable setting costs one repeated line and is replaced.
func TestVolumeDropCheck_UnreadableSettingCountsAsNoneOpen(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	seedFallingProject(t, store, start)
	require.NoError(t, store.SetInstanceSetting(context.Background(), volumeDropOpenKey, "not json"))
	logs := captureDefaultLog(t)

	check := &VolumeDropCheck{store: store, now: func() time.Time { return start }}
	check.Run(context.Background())

	require.Len(t, logsAtLevel(t, logs, "WARN"), 1)
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)
	value, _, err := store.GetInstanceSetting(context.Background(), volumeDropOpenKey)
	require.NoError(t, err)
	require.JSONEq(t, `["shop"]`, value)
}

// Test rows follow deploys and say nothing about production.
func TestVolumeDropCheck_TestRowsDoNotHideAProductionDrop(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	p := seedFallingProject(t, store, start)
	seedHours(t, store, p.ID, environment.Test, start.Add(-22*time.Hour), start, 10)
	logs := captureDefaultLog(t)

	check := &VolumeDropCheck{store: store, now: func() time.Time { return start }}
	check.Run(context.Background())

	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)
}

// A project falling only in test keeps its production baseline and raises nothing.
func TestVolumeDropCheck_QuietWhenOnlyTestRowsFall(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	p := seedFallingProject(t, store, start)
	seedHours(t, store, p.ID, environment.Production, start.Add(-22*time.Hour), start, 3)
	logs := captureDefaultLog(t)

	check := &VolumeDropCheck{store: store, now: func() time.Time { return start }}
	check.Run(context.Background())

	require.Empty(t, logsAtLevel(t, logs, "ERROR"))
}

// A failed read logs Warn and leaves the hour open for a retry; the Error waits
// for the stall alarm.
func TestVolumeDropCheck_RetriesAfterAFailedRead(t *testing.T) {
	store := repotest.Open(t)
	logs := captureDefaultLog(t)

	prev := volumeDropTimeout
	volumeDropTimeout = time.Nanosecond
	t.Cleanup(func() { volumeDropTimeout = prev })

	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	check := &VolumeDropCheck{store: store, now: func() time.Time { return start }}
	check.Run(context.Background())

	require.Len(t, logsAtLevel(t, logs, "WARN"), 1)
	require.Empty(t, logsAtLevel(t, logs, "ERROR"))
	require.True(t, check.lastHour.IsZero())

	volumeDropTimeout = prev
	check.Run(context.Background())
	require.Equal(t, start, check.lastHour)
}

// A cancelled process context is a shutdown. It records no failure, so a
// rollout leaves neither a warning nor an error span, and the hour stays open.
func TestVolumeDropCheck_ShutdownIsNotAFailure(t *testing.T) {
	store := repotest.Open(t)
	logs := captureDefaultLog(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	check := &VolumeDropCheck{store: store, now: func() time.Time { return start }}
	check.Run(cancelled)

	require.Empty(t, logsAtLevel(t, logs, "WARN"))
	require.Empty(t, logsAtLevel(t, logs, "ERROR"))
	require.True(t, check.lastHour.IsZero())
	require.True(t, check.failingSince.IsZero())

	check.Run(context.Background())
	require.Equal(t, start, check.lastHour)
}

// An expired deadline ends the evaluation, so the worker loop moves on, and the
// hour stays open for a retry.
func TestVolumeDropCheck_DeadlineEndsTheEvaluation(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	seedFallingProject(t, store, start)
	logs := captureDefaultLog(t)

	prev := volumeDropTimeout
	volumeDropTimeout = time.Nanosecond
	t.Cleanup(func() { volumeDropTimeout = prev })

	check := &VolumeDropCheck{store: store, now: func() time.Time { return start }}
	check.Run(context.Background())

	warns := logsAtLevel(t, logs, "WARN")
	require.Len(t, warns, 1)
	require.Contains(t, warns[0]["err"], "read open drops")
	require.Empty(t, logsAtLevel(t, logs, "ERROR"))
	require.True(t, check.lastHour.IsZero())

	volumeDropTimeout = prev
	check.Run(context.Background())
	require.Equal(t, start, check.lastHour)
}

func setLastHour(t *testing.T, store *repository.Store, h time.Time) {
	t.Helper()
	require.NoError(t, store.SetInstanceSetting(context.Background(), volumeDropLastKey, h.Format(volumedrop.HourFormat)))
}

func lastHourSetting(t *testing.T, store *repository.Store) string {
	t.Helper()
	value, ok, err := store.GetInstanceSetting(context.Background(), volumeDropLastKey)
	require.NoError(t, err)
	require.True(t, ok)
	return value
}

// A drop that starts in an hour the check missed is still reported: evaluated
// at start+3h alone, the 3 events an hour from start lift the window over a
// tenth of the baseline and the drop would go unseen.
func TestVolumeDropCheck_CatchUpReportsADropInAMissedHour(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	p := seedFallingProject(t, store, start)
	seedHours(t, store, p.ID, environment.Production, start, start.Add(3*time.Hour), 3)
	setLastHour(t, store, start.Add(-time.Hour))
	logs := captureDefaultLog(t)

	check := &VolumeDropCheck{store: store, now: func() time.Time { return start.Add(3*time.Hour + 5*time.Minute) }}
	check.Run(context.Background())

	errs := logsAtLevel(t, logs, "ERROR")
	require.Len(t, errs, 1)
	require.Equal(t, "shop", errs[0]["project"])
	require.Equal(t, start.Format(volumedrop.HourFormat), errs[0]["hour"])
	require.Equal(t, start.Add(3*time.Hour).Format(volumedrop.HourFormat), lastHourSetting(t, store))
}

// Hours older than the catch-up limit are reported once, and the rest evaluated.
func TestVolumeDropCheck_ReportsHoursPastTheLimit(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	setLastHour(t, store, start.Add(-30*time.Hour))
	logs := captureDefaultLog(t)

	check := &VolumeDropCheck{store: store, now: func() time.Time { return start }}
	check.Run(context.Background())

	errs := logsAtLevel(t, logs, "ERROR")
	require.Len(t, errs, 1)
	require.Contains(t, errs[0]["msg"], "hours skipped")
	require.EqualValues(t, 30-volumeDropCatchUp, errs[0]["hours"])
	require.Equal(t, start.Add(-(volumeDropCatchUp-1)*time.Hour).Format(volumedrop.HourFormat), errs[0]["first_evaluated"])
	require.Equal(t, start.Format(volumedrop.HourFormat), lastHourSetting(t, store))

	check.lastHour = time.Time{}
	check.Run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1, "the skip is reported once")
}

// When the hourly read keeps failing, the stored last hour dates the outage, so
// the alarm fires after 3 hours even in a process that just started. It fires
// once per outage and resets after a good read.
func TestVolumeDropCheck_StallAlarmFromTheStoredHour(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	setLastHour(t, store, start.Add(-4*time.Hour))
	_, err := store.DB().Exec(`ALTER TABLE events RENAME TO events_away`)
	require.NoError(t, err)
	logs := captureDefaultLog(t)

	clock := start.Add(-time.Minute)
	check := &VolumeDropCheck{store: store, now: func() time.Time { return clock }}
	check.Run(context.Background())
	require.Empty(t, logsAtLevel(t, logs, "ERROR"), "2h59m since the first missed hour")

	clock = start
	check.Run(context.Background())
	errs := logsAtLevel(t, logs, "ERROR")
	require.Len(t, errs, 1)
	require.Contains(t, errs[0]["msg"], "no hour for 3 hours")
	require.Equal(t, start.Add(-3*time.Hour).Format(time.RFC3339), errs[0]["since"])

	clock = start.Add(10 * time.Minute)
	check.Run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1, "once per outage")

	_, err = store.DB().Exec(`ALTER TABLE events_away RENAME TO events`)
	require.NoError(t, err)
	clock = start.Add(20 * time.Minute)
	check.Run(context.Background())
	require.False(t, check.stallReported)
	require.Equal(t, start.Format(volumedrop.HourFormat), lastHourSetting(t, store))
}

// With nothing stored, the process's first failure dates the outage.
func TestVolumeDropCheck_StallAlarmInMemory(t *testing.T) {
	store := repotest.Open(t)
	logs := captureDefaultLog(t)
	prev := volumeDropTimeout
	volumeDropTimeout = time.Nanosecond
	t.Cleanup(func() { volumeDropTimeout = prev })

	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	clock := start
	check := &VolumeDropCheck{store: store, now: func() time.Time { return clock }}
	for _, at := range []time.Duration{0, time.Hour, 2*time.Hour + 50*time.Minute} {
		clock = start.Add(at)
		check.Run(context.Background())
	}
	require.Empty(t, logsAtLevel(t, logs, "ERROR"))

	clock = start.Add(3 * time.Hour)
	check.Run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)
}

func TestPendingHours(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	hour := func(n int) time.Time { return now.Add(time.Duration(n) * time.Hour) }
	for _, tc := range []struct {
		name    string
		last    time.Time
		first   time.Time
		n       int
		skipped int
	}{
		{"nothing stored", time.Time{}, now, 1, 0},
		{"previous hour", hour(-1), now, 1, 0},
		{"three missed", hour(-4), hour(-3), 4, 0},
		{"at the limit", hour(-volumeDropCatchUp), hour(-volumeDropCatchUp + 1), volumeDropCatchUp, 0},
		{"past the limit", hour(-30), hour(-volumeDropCatchUp + 1), volumeDropCatchUp, 30 - volumeDropCatchUp},
		{"stored hour ahead", hour(2), now, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pending, skipped := pendingHours(tc.last, now)
			require.Len(t, pending, tc.n)
			require.Equal(t, tc.first, pending[0])
			require.Equal(t, now, pending[len(pending)-1])
			require.Equal(t, tc.skipped, skipped)
		})
	}
}

// The hourly read must split into the wait for a read connection, the rowid bound
// and the count, under the check's span, so a slow read shows where it spent its
// time.
func TestVolumeDropCheck_SpansSplitTheRead(t *testing.T) {
	exp := captureSpans(t)
	store, err := repository.Open(repotest.Path(t, "volume-drop-spans.db"))
	require.NoError(t, err)
	defer store.Close()
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	seedFallingProject(t, store, start)
	captureDefaultLog(t)

	check := &VolumeDropCheck{store: store, now: func() time.Time { return start }}
	check.Run(context.Background())

	byName := map[string]tracetest.SpanStub{}
	for _, s := range exp.GetSpans() {
		byName[s.Name] = s
	}
	root, ok := byName["worker.volume_drop"]
	require.True(t, ok)
	read, ok := byName["repository.EnvironmentEventsByHour"]
	require.True(t, ok)
	require.Equal(t, root.SpanContext.SpanID(), read.Parent.SpanID())
	for _, name := range []string{"repository.acquire_read_conn", "repository.events_rowid_bound", "repository.events_hourly_count"} {
		s, ok := byName[name]
		require.True(t, ok, name)
		require.Equal(t, read.SpanContext.SpanID(), s.Parent.SpanID(), name)
	}
	attrs := map[string]int64{}
	for _, a := range byName["repository.events_rowid_bound"].Attributes {
		attrs[string(a.Key)] = a.Value.AsInt64()
	}
	require.Positive(t, attrs["lookups"])
	require.Positive(t, attrs["rowid.from"])
}
