package command_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

func TestOutageLevels(t *testing.T) {
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	o := &command.Outage{After: 30 * time.Second, Now: func() time.Time { return now }}

	_, ok := o.Recover()
	require.False(t, ok, "nothing failed yet")

	require.Equal(t, slog.LevelWarn, o.Fail(), "first failure")
	now = now.Add(29 * time.Second)
	require.Equal(t, slog.LevelDebug, o.Fail(), "under After")
	now = now.Add(time.Second)
	require.Equal(t, slog.LevelError, o.Fail(), "lasted After")
	now = now.Add(time.Hour)
	require.Equal(t, slog.LevelDebug, o.Fail(), "Error is logged once per run")

	lasted, ok := o.Recover()
	require.True(t, ok)
	require.Equal(t, time.Hour+30*time.Second, lasted)
	_, ok = o.Recover()
	require.False(t, ok, "the run already ended")

	require.Equal(t, slog.LevelWarn, o.Fail(), "a new run starts at Warn")
	now = now.Add(30 * time.Second)
	require.Equal(t, slog.LevelError, o.Fail(), "and can reach Error again")
}

func TestOutageZeroValueUsesDefault(t *testing.T) {
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	o := &command.Outage{Now: func() time.Time { return now }}
	require.Equal(t, slog.LevelWarn, o.Fail())
	now = now.Add(command.DefaultOutageAfter - time.Nanosecond)
	require.Equal(t, slog.LevelDebug, o.Fail())
	now = now.Add(time.Nanosecond)
	require.Equal(t, slog.LevelError, o.Fail())
}
