package main

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

	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
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
	check := &volumeDropCheck{store: store, now: func() time.Time { return clock }}
	check.run(context.Background())

	errs := logsAtLevel(t, logs, "ERROR")
	require.Len(t, errs, 1)
	require.Equal(t, "shop", errs[0]["project"])
	require.EqualValues(t, 72, errs[0]["baseline_24h"])
	require.EqualValues(t, 6, errs[0]["current_24h"])
	require.Contains(t, errs[0]["msg"], "shop", "the project must be in the message so each gets its own issue")

	// Same hour, same instance: no second evaluation.
	clock = start.Add(15 * time.Minute)
	check.run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)

	// Next hour: the drop is running.
	clock = start.Add(time.Hour + 5*time.Minute)
	check.run(context.Background())
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
		check := &volumeDropCheck{store: store, now: func() time.Time { return clock }}
		check.run(context.Background())
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
	check := &volumeDropCheck{store: store, now: func() time.Time { return clock }}
	check.run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)

	// 30 events in the window against a baseline of 72: over a fifth, so it closes.
	seedHours(t, store, p.ID, environment.Production, start, start.Add(10*time.Hour), 3)
	clock = start.Add(10 * time.Hour)
	check.run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)
	value, ok, err := store.GetInstanceSetting(context.Background(), volumeDropOpenKey)
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, `[]`, value)

	// 22 hours after the events stop again it has 6 left in the window.
	clock = start.Add(32 * time.Hour)
	check.run(context.Background())
	require.Len(t, logsAtLevel(t, logs, "ERROR"), 2)
}

// An unreadable setting costs one repeated line and is replaced.
func TestVolumeDropCheck_UnreadableSettingCountsAsNoneOpen(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	seedFallingProject(t, store, start)
	require.NoError(t, store.SetInstanceSetting(context.Background(), volumeDropOpenKey, "not json"))
	logs := captureDefaultLog(t)

	check := &volumeDropCheck{store: store, now: func() time.Time { return start }}
	check.run(context.Background())

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

	check := &volumeDropCheck{store: store, now: func() time.Time { return start }}
	check.run(context.Background())

	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)
}

// A project falling only in test keeps its production baseline and raises nothing.
func TestVolumeDropCheck_QuietWhenOnlyTestRowsFall(t *testing.T) {
	store := repotest.Open(t)
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	p := seedFallingProject(t, store, start)
	seedHours(t, store, p.ID, environment.Production, start.Add(-22*time.Hour), start, 3)
	logs := captureDefaultLog(t)

	check := &volumeDropCheck{store: store, now: func() time.Time { return start }}
	check.run(context.Background())

	require.Empty(t, logsAtLevel(t, logs, "ERROR"))
}

// A failed read reports the error and leaves the hour open for a retry.
func TestVolumeDropCheck_RetriesAfterAFailedRead(t *testing.T) {
	store := repotest.Open(t)
	logs := captureDefaultLog(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	check := &volumeDropCheck{store: store, now: func() time.Time { return start }}
	check.run(cancelled)

	require.Len(t, logsAtLevel(t, logs, "ERROR"), 1)
	require.True(t, check.lastHour.IsZero())

	check.run(context.Background())
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

	check := &volumeDropCheck{store: store, now: func() time.Time { return start }}
	check.run(context.Background())

	errs := logsAtLevel(t, logs, "ERROR")
	require.Len(t, errs, 1)
	require.Contains(t, errs[0]["msg"], "read open drops")
	require.True(t, check.lastHour.IsZero())

	volumeDropTimeout = prev
	check.run(context.Background())
	require.Equal(t, start, check.lastHour)
}
