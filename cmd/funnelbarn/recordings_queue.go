package main

import (
	"context"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/blobstore"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest/pipeline"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// newRecordings builds the recordings service when R2 is configured, and with
// FUNNELBARN_RECORDINGS_VIA_QUEUE on, the recordings queue it publishes to.
// Without R2 both are nil: recording is off. The queue's consumer is started.
func newRecordings(ctx context.Context, cfg config.Config, store *repository.Store, logger *slog.Logger) (service.Recordings, *pipeline.RecordingQueue, error) {
	if cfg.R2Endpoint == "" || cfg.R2AccessKeyID == "" || cfg.R2SecretAccessKey == "" || cfg.R2Bucket == "" {
		return nil, nil, nil
	}
	r2, err := blobstore.NewR2(cfg.R2Endpoint, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2Bucket)
	if err != nil {
		logger.Warn("session recording disabled: failed to initialize R2 storage", "err", err)
		return nil, nil, nil
	}
	svc := service.NewRecordingService(store, store, store, r2)
	logger.Info("session recording enabled", "bucket", cfg.R2Bucket)

	q, err := pipeline.NewRecordingQueue(ctx, cfg, svc, logger)
	if err != nil {
		return nil, nil, err
	}
	if q != nil {
		svc.SetChunkQueue(q)
	}
	return svc, q, nil
}
