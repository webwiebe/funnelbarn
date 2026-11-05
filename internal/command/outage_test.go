package command_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

// outageClock drives an Outage on a fake clock.
type outageClock struct {
	t   *testing.T
	now time.Time
	o   *command.Outage
}

func newOutageClock(t *testing.T) *outageClock {
	c := &outageClock{t: t, now: time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)}
	c.o = &command.Outage{Now: func() time.Time { return c.now }}
	return c
}

// fail waits d, records a failure and returns its level.
func (c *outageClock) fail(d time.Duration) slog.Level {
	c.now = c.now.Add(d)
	level, _ := c.o.Fail()
	return level
}

// failAttrs is fail for tests that read the total and the number of runs.
func (c *outageClock) failAttrs(d time.Duration) (slog.Level, string, int64) {
	c.now = c.now.Add(d)
	level, attr := c.o.Fail()
	require.Equal(c.t, "outage", attr.Key)
	var total string
	var runs int64
	for _, a := range attr.Value.Group() {
		switch a.Key {
		case "total":
			total = a.Value.String()
		case "runs":
			runs = a.Value.Int64()
		}
	}
	return level, total, runs
}

// recoverAfter waits d and ends the run.
func (c *outageClock) recoverAfter(d time.Duration) time.Duration {
	c.now = c.now.Add(d)
	lasted, ok := c.o.Recover()
	require.True(c.t, ok)
	return lasted
}

func TestOutageLevels(t *testing.T) {
	c := newOutageClock(t)

	_, ok := c.o.Recover()
	require.False(t, ok, "nothing failed yet")

	require.Equal(t, slog.LevelWarn, c.fail(0), "first failure")
	require.Equal(t, slog.LevelDebug, c.fail(29*time.Second), "under After")
	require.Equal(t, slog.LevelError, c.fail(time.Second), "lasted After")
	require.Equal(t, slog.LevelDebug, c.fail(time.Second), "Error is logged once per window")

	require.Equal(t, 31*time.Second, c.recoverAfter(0))
	_, ok = c.o.Recover()
	require.False(t, ok, "the run already ended")

	require.Equal(t, slog.LevelWarn, c.fail(time.Second), "a new run starts at Warn")
	require.Equal(t, slog.LevelDebug, c.fail(30*time.Second), "the window already had its Error")
}

func TestOutageZeroValueUsesDefault(t *testing.T) {
	c := newOutageClock(t)
	require.Equal(t, slog.LevelWarn, c.fail(0))
	require.Equal(t, slog.LevelDebug, c.fail(command.DefaultOutageAfter-time.Nanosecond))
	require.Equal(t, slog.LevelError, c.fail(time.Nanosecond))
}

// One success between stalls no longer resets the count: 29s runs reach Error
// at the point the total reaches 30s, and only once inside the window.
func TestOutageRepeatedRunsLogErrorOnceAtTheTotal(t *testing.T) {
	c := newOutageClock(t)

	require.Equal(t, slog.LevelWarn, c.fail(0))
	require.Equal(t, slog.LevelDebug, c.fail(29*time.Second))
	require.Equal(t, 29*time.Second, c.recoverAfter(time.Second))

	// This call counts for its timeout, which takes the total past 30s.
	level, total, runs := c.failAttrs(time.Second)
	require.Equal(t, slog.LevelError, level)
	require.Equal(t, "31s", total)
	require.EqualValues(t, 2, runs)

	require.Equal(t, slog.LevelDebug, c.fail(29*time.Second))
	c.recoverAfter(time.Second)

	for range 5 {
		require.Equal(t, slog.LevelWarn, c.fail(time.Second), "inside the window")
		require.Equal(t, slog.LevelDebug, c.fail(29*time.Second), "inside the window")
		c.recoverAfter(time.Second)
	}
}

// The pattern going on past the window logs Error again.
func TestOutageRepeatedRunsLogErrorOncePerWindow(t *testing.T) {
	c := newOutageClock(t)
	errors := 0
	for range 44 { // 44 runs of 30s is 22 minutes.
		for _, d := range []time.Duration{time.Second, 28 * time.Second} {
			if c.fail(d) == slog.LevelError {
				errors++
			}
		}
		c.recoverAfter(time.Second)
	}
	require.Equal(t, 3, errors)
}

func TestOutageIdleBeforeSuccessAddsNothing(t *testing.T) {
	c := newOutageClock(t)
	c.fail(0)
	require.Equal(t, command.DefaultPublishTimeout, c.recoverAfter(9*time.Minute),
		"a single failure counts for the call's timeout")

	level, total, runs := c.failAttrs(0)
	require.Equal(t, slog.LevelWarn, level)
	require.Equal(t, "4s", total, "two single failures, no idle time")
	require.EqualValues(t, 2, runs)
}

// A quiet service sees one failed call per stall and no success between them.
func TestOutageIdleBetweenFailuresAddsNothing(t *testing.T) {
	c := newOutageClock(t)
	c.fail(0)
	level, total, runs := c.failAttrs(5 * time.Minute)
	require.Equal(t, slog.LevelWarn, level, "a new run")
	require.Equal(t, "4s", total)
	require.EqualValues(t, 2, runs)
}

func TestOutageShortStallsNeverLogError(t *testing.T) {
	c := newOutageClock(t)
	for range 100 {
		require.Equal(t, slog.LevelWarn, c.fail(3*time.Hour))
		require.Equal(t, slog.LevelDebug, c.fail(4*time.Second))
		c.recoverAfter(time.Second)
	}
}

func TestOutageTimeOutsideTheWindowDropsOut(t *testing.T) {
	c := newOutageClock(t)
	c.fail(0)
	c.fail(20 * time.Second)
	c.recoverAfter(0)

	// Five seconds of the 20s run are left in the window.
	level, total, runs := c.failAttrs(command.OutageWindow - 5*time.Second)
	require.Equal(t, slog.LevelWarn, level)
	require.Equal(t, "7s", total)
	require.EqualValues(t, 2, runs)
	c.recoverAfter(0)

	level, total, runs = c.failAttrs(time.Minute)
	require.Equal(t, slog.LevelWarn, level)
	require.Equal(t, "4s", total, "the 20s run is gone, the 2s run is not")
	require.EqualValues(t, 2, runs)
}

// A stall that does not end reports once per window, and the total stays what
// the window holds.
func TestOutageLongRunRepeatsErrorPerWindow(t *testing.T) {
	c := newOutageClock(t)
	errors := 0
	var total string
	for range 300 { // 25 minutes: Error at 30s, 10m30s and 20m30s.
		var level slog.Level
		level, total, _ = c.failAttrs(5 * time.Second)
		if level == slog.LevelError {
			errors++
		}
	}
	require.Equal(t, 3, errors)
	require.Equal(t, command.OutageWindow.String(), total)
}

func TestOutageFlappingKeepsOneEntryPerSecond(t *testing.T) {
	c := newOutageClock(t)
	for i := range 5000 {
		c.fail(time.Millisecond)
		c.recoverAfter(time.Millisecond)
		if i%1000 == 999 {
			require.LessOrEqual(t, command.OutageClosedEntries(c.o), 11)
		}
	}
}

func TestOutageAfterOverridesTheDefault(t *testing.T) {
	c := newOutageClock(t)
	c.o.After = 10 * time.Second
	require.Equal(t, slog.LevelWarn, c.fail(0))
	require.Equal(t, slog.LevelError, c.fail(10*time.Second))
}
