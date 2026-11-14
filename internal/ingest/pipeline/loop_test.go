package pipeline

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

func TestReaderWorkerForwardsTheSpool(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := config.Config{Mode: config.ModeReader, RedisQueueURL: "redis://" + mr.Addr() + "/0", SpoolDir: t.TempDir(), IngestQueueMaxLen: 10}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sp, err := spool.New(cfg.SpoolDir)
	require.NoError(t, err)
	defer sp.Close()
	require.NoError(t, sp.Append(spool.Record{IngestID: "i1", ReceivedAt: time.Now(), BodyBase64: "e30=", ProjectSlug: "s"}))

	iq, err := NewIngestQueue(ctx, cfg, nil, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	defer iq.Close(context.Background())

	done := make(chan struct{})
	go func() {
		RunReaderWorker(ctx, cfg, sp, iq, nil)
		close(done)
	}()
	require.Eventually(t, func() bool {
		n, err := iq.Forwarder.list.QueuedLen(context.Background())
		return err == nil && n == 1
	}, 5*time.Second, 20*time.Millisecond)
	cancel()
	<-done
}
