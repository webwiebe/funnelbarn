package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

func TestNewIngestQueueOffIsNil(t *testing.T) {
	a, _ := newTestApplier(t)
	q, err := newIngestQueue(context.Background(), config.Config{RedisQueueURL: "redis://unused"}, a, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	require.Nil(t, q)
	require.NoError(t, q.Close(context.Background()))
}

func TestNewIngestQueueNeedsRedisURL(t *testing.T) {
	a, _ := newTestApplier(t)
	_, err := newIngestQueue(context.Background(), config.Config{IngestViaQueue: true}, a, slog.New(slog.DiscardHandler))
	require.ErrorContains(t, err, "FUNNELBARN_REDIS_QUEUE_URL")
}

// Records go spool -> forwarder -> Valkey -> consumer -> SQLite. A record
// forwarded twice (a crash between the LPUSH and the cursor write) is stored
// once, and a record whose project can never resolve is dead-lettered without
// holding up the rest.
func TestIngestQueueStoresForwardedRecords(t *testing.T) {
	a, store := newTestApplier(t)
	mr := miniredis.RunT(t)
	dir := t.TempDir()
	cfg := config.Config{
		RedisQueueURL:     "redis://" + mr.Addr(),
		IngestViaQueue:    true,
		IngestQueueMaxLen: 100,
		SpoolDir:          dir,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q, err := newIngestQueue(ctx, cfg, a, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	defer func() { require.NoError(t, q.Close(ctx)) }()

	recs := []spool.RecordAtOffset{
		{Record: applyRecord("q-1", "queue-site", `{"name":"pageview","url":"https://example.com/","session_id":"s1"}`), EndOffset: 10},
		{Record: applyRecord("q-2", "", `{"name":"pageview","url":"https://example.com/"}`), EndOffset: 20},
		{Record: applyRecord("q-3", "queue-site", `{"name":"signup","url":"https://example.com/","session_id":"s1"}`), EndOffset: 30},
	}
	require.Equal(t, int64(30), q.forwarder.forward(ctx, recs, 0))
	// The cursor write was lost: the next tick sends the same records again.
	require.Equal(t, int64(30), q.forwarder.forward(ctx, recs, 0))
	require.NoError(t, q.bus.Flush(ctx))

	require.Equal(t, 1, countEvents(t, store, "q-1"))
	require.Equal(t, 0, countEvents(t, store, "q-2"))
	require.Equal(t, 1, countEvents(t, store, "q-3"))

	dead, err := spool.ReadRecords(spool.DeadLetterPath(dir))
	require.NoError(t, err)
	require.Len(t, dead, 2, "q-2 is dead-lettered on each delivery")
	require.Equal(t, "q-2", dead[0].IngestID)
}
