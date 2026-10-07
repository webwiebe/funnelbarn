package main

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func newCommandTestStore(t *testing.T) (*repository.Store, repository.Project, repository.FeatureFlag) {
	t.Helper()
	ctx := context.Background()
	store, err := repository.Open(filepath.Join(t.TempDir(), "cmd.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	p, err := store.CreateProject(ctx, "Cmd", "cmd")
	require.NoError(t, err)
	f, err := store.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID: p.ID, FlagKey: "cmd_flag", Name: "Cmd", FlagType: "boolean",
		Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: `{"on":50,"off":50}`, Status: "active",
	})
	require.NoError(t, err)
	return store, p, f
}

func evaluationRows(t *testing.T, store *repository.Store) int {
	t.Helper()
	var n int
	require.NoError(t, store.DB().QueryRow(`SELECT COUNT(*) FROM flag_evaluations`).Scan(&n))
	return n
}

func TestNewCommandBusWithoutRedisIsTheDispatcher(t *testing.T) {
	store, p, f := newCommandTestStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bus, err := newCommandBus(ctx, config.Config{}, command.Deps{Store: store}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	require.IsType(t, &command.Dispatcher{}, bus)

	bus.Submit(ctx, command.RecordEvaluation{Eval: repository.FlagEvaluation{ID: "e1", FlagID: f.ID, ProjectID: p.ID, Variant: "on"}})
	require.NoError(t, bus.Flush(ctx))
	require.Equal(t, 1, evaluationRows(t, store))
	require.NoError(t, bus.Close(ctx))
}

func TestNewCommandBusWithRedisConsumesTheQueue(t *testing.T) {
	store, p, f := newCommandTestStore(t)
	mr := miniredis.RunT(t)

	var logs bytes.Buffer
	cfg := config.Config{RedisQueueURL: "redis://:s3cret@" + mr.Addr() + "/0"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bus, err := newCommandBus(ctx, cfg, command.Deps{Store: store}, slog.New(slog.NewTextHandler(&logs, nil)))
	require.NoError(t, err)
	require.IsType(t, &redisClosingBus{}, bus)

	bus.Submit(ctx, command.RecordEvaluation{Eval: repository.FlagEvaluation{ID: "e1", FlagID: f.ID, ProjectID: p.ID, Variant: "on"}})
	require.NoError(t, bus.Flush(ctx))
	require.Equal(t, 1, evaluationRows(t, store))
	require.NoError(t, bus.Close(ctx))

	// The client is closed after the bus: a second Close of the client errors.
	require.Error(t, bus.(*redisClosingBus).client.Close())

	// Read after Close: nothing logs concurrently any more.
	require.Contains(t, logs.String(), "command bus: redis")
	require.Contains(t, logs.String(), "bookkeeping")
	require.NotContains(t, logs.String(), "s3cret")
}

func TestNewCommandBusRejectsBadRedisURL(t *testing.T) {
	_, err := newCommandBus(context.Background(), config.Config{RedisQueueURL: "http://not-redis"}, command.Deps{}, slog.New(slog.DiscardHandler))
	require.Error(t, err)
}
