package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
)

func TestNewRecordingsWithoutR2IsOff(t *testing.T) {
	_, store := newTestApplier(t)
	svc, q, err := newRecordings(context.Background(), config.Config{RecordingsViaQueue: true}, store, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	require.Nil(t, svc)
	require.Nil(t, q)
}

// With R2 configured the service is built, and the queue only with its flag.
// NewR2 does not connect, so placeholder credentials do.
func TestNewRecordingsWithR2(t *testing.T) {
	_, store := newTestApplier(t)
	mr := miniredis.RunT(t)
	cfg := config.Config{
		R2Endpoint:        "http://127.0.0.1:1",
		R2AccessKeyID:     "id",
		R2SecretAccessKey: "secret",
		R2Bucket:          "bucket",
		RedisQueueURL:     "redis://" + mr.Addr(),
	}
	logger := slog.New(slog.DiscardHandler)

	svc, q, err := newRecordings(context.Background(), cfg, store, logger)
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.Nil(t, q)

	cfg.RecordingsViaQueue = true
	cfg.RecordingsQueueMaxLen = 10
	svc, q, err = newRecordings(context.Background(), cfg, store, logger)
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, q)
	require.NoError(t, q.Close(context.Background()))

	cfg.RedisQueueURL = ""
	_, _, err = newRecordings(context.Background(), cfg, store, logger)
	require.Error(t, err)
}
