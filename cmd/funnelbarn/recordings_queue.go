package main

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/queue"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/storage"
)

// recordingsQueueName is the name of the Redis list that carries recording
// chunk metadata.
const recordingsQueueName = "recordings"

// chunkList is the part of queue.RedisList the recordings queue publishes to.
type chunkList interface {
	Publish(ctx context.Context, payload []byte) error
	QueuedLen(ctx context.Context) (int64, error)
}

// recordingsQueue takes chunk metadata off the request path: the handler
// uploads the chunk to R2, and a RedisBus consumer folds the metadata into
// SQLite. When the queue is full or Valkey cannot be reached, Enqueue says so
// and the handler applies the chunk itself, as it does with the queue off.
type recordingsQueue struct {
	list   chunkList
	maxLen int64
	bus    *command.RedisBus
	client *redis.Client
	// fallback is set in a reader, which cannot apply a chunk itself: a chunk
	// the queue does not take is buffered on disk and published later.
	fallback *fallbackForwarder

	// Requests enqueue concurrently, so the once-per-episode log flags are
	// atomic.
	failing atomic.Bool
	full    atomic.Bool
}

var _ service.ChunkQueue = (*recordingsQueue)(nil)

// newRecordings builds the recordings service when R2 is configured, and with
// FUNNELBARN_RECORDINGS_VIA_QUEUE on, the recordings queue it publishes to.
// Without R2 both are nil: recording is off. The queue's consumer is started.
func newRecordings(ctx context.Context, cfg config.Config, store *repository.Store, logger *slog.Logger) (service.Recordings, *recordingsQueue, error) {
	if cfg.R2Endpoint == "" || cfg.R2AccessKeyID == "" || cfg.R2SecretAccessKey == "" || cfg.R2Bucket == "" {
		return nil, nil, nil
	}
	r2, err := storage.NewR2(cfg.R2Endpoint, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2Bucket)
	if err != nil {
		logger.Warn("session recording disabled: failed to initialize R2 storage", "err", err)
		return nil, nil, nil
	}
	svc := service.NewRecordingService(store, store, store, r2)
	logger.Info("session recording enabled", "bucket", cfg.R2Bucket)

	q, err := newRecordingsQueue(ctx, cfg, svc, logger)
	if err != nil {
		return nil, nil, err
	}
	if q != nil {
		svc.SetChunkQueue(q)
	}
	return svc, q, nil
}

// recordingsFallbackName names a reader's on-disk buffer of chunk metadata in
// its spool directory.
const recordingsFallbackName = "recordings-fallback"

// newRecordingsQueue returns nil when FUNNELBARN_RECORDINGS_VIA_QUEUE is off
// in a standalone process, which keeps chunks on the request path and is the
// rollback path. A reader and a writer always use the queue; a reader only
// publishes to it.
func newRecordingsQueue(ctx context.Context, cfg config.Config, svc *service.RecordingService, logger *slog.Logger) (*recordingsQueue, error) {
	if !cfg.RecordingsViaQueue && !isSplit(cfg) {
		return nil, nil
	}
	if cfg.RedisQueueURL == "" {
		return nil, errors.New("FUNNELBARN_RECORDINGS_VIA_QUEUE needs FUNNELBARN_REDIS_QUEUE_URL")
	}
	client, err := queue.NewClient(cfg.RedisQueueURL)
	if err != nil {
		return nil, err
	}
	list := queue.NewRedisList(client, recordingsQueueName)
	// The URL may carry a password, so only the queue name is logged.
	logger.Info("recordings: redis queue", "queue", recordingsQueueName, "max_len", cfg.RecordingsQueueMaxLen, "consume", !isReader(cfg))
	if isReader(cfg) {
		fallback, err := command.NewSpoolFallback(cfg.SpoolDir, recordingsFallbackName, 0, logger)
		if err != nil {
			_ = client.Close()
			return nil, err
		}
		return &recordingsQueue{
			list:     list,
			maxLen:   cfg.RecordingsQueueMaxLen,
			client:   client,
			fallback: &fallbackForwarder{name: "recordings", fallback: fallback, list: list},
		}, nil
	}

	bus := command.NewRedisBus(command.RedisOptions{
		Queue:      list,
		Deps:       command.Deps{ApplyChunk: applyQueuedChunk(svc)},
		Consume:    true,
		Logger:     logger,
		DeadLetter: deadLetterChunk(logger),
	})
	bus.Start(ctx)
	return &recordingsQueue{list: list, maxLen: cfg.RecordingsQueueMaxLen, bus: bus, client: client}, nil
}

// applyQueuedChunk applies chunk metadata taken from the recordings queue.
func applyQueuedChunk(svc *service.RecordingService) func(context.Context, command.ApplyRecordingChunk) error {
	return func(ctx context.Context, c command.ApplyRecordingChunk) error {
		return svc.ApplyChunkMeta(ctx, service.ChunkMeta{
			Recording:  c.Recording,
			ChunkIndex: c.ChunkIndex,
			Traces:     c.Traces,
		})
	}
}

// deadLetterChunk reports a chunk whose metadata failed every retry. Its
// events stay in R2, but the recording row does not count it, so this is an
// unhandled error that reaches BugBarn as an issue.
func deadLetterChunk(logger *slog.Logger) func(context.Context, command.Command, error) {
	return func(ctx context.Context, c command.Command, err error) {
		chunk := c.(command.ApplyRecordingChunk)
		logger.ErrorContext(ctx, "recordings queue dropping chunk after failed retries",
			"err", err, "handled", false,
			"project_id", chunk.Recording.ProjectID,
			"recording_id", chunk.Recording.ID,
			"chunk_index", chunk.ChunkIndex)
	}
}

// Enqueue publishes meta and reports whether it is queued. It returns false
// while the queue holds maxLen chunks or Valkey cannot be reached.
func (q *recordingsQueue) Enqueue(ctx context.Context, meta service.ChunkMeta) bool {
	chunk := command.ApplyRecordingChunk{
		Recording:  meta.Recording,
		ChunkIndex: meta.ChunkIndex,
		Traces:     meta.Traces,
	}
	queued, err := q.list.QueuedLen(ctx)
	if err != nil {
		q.publishFailed(ctx, err)
		return q.toFallback(ctx, chunk)
	}
	if queued >= q.maxLen {
		if !q.full.Swap(true) {
			slog.WarnContext(ctx, "recordings queue full, chunks bypass it (applied on the request, or buffered on disk in a reader)",
				"handled", true, "queued", queued, "max_len", q.maxLen)
		}
		return q.toFallback(ctx, chunk)
	}
	if q.full.Swap(false) {
		slog.InfoContext(ctx, "recordings queue has room again", "queued", queued)
	}

	payload, err := command.Encode(chunk, time.Now())
	if err != nil {
		slog.ErrorContext(ctx, "recordings queue encode chunk", "err", err,
			"recording_id", meta.Recording.ID, "chunk_index", meta.ChunkIndex)
		return false
	}
	if err := q.list.Publish(ctx, payload); err != nil {
		q.publishFailed(ctx, err)
		return q.toFallback(ctx, chunk)
	}
	if q.failing.Swap(false) {
		slog.InfoContext(ctx, "recordings queue publish recovered")
	}
	return true
}

func (q *recordingsQueue) publishFailed(ctx context.Context, err error) {
	if !q.failing.Swap(true) {
		slog.ErrorContext(ctx, "recordings queue publish failed, chunks bypass it (applied on the request, or buffered on disk in a reader)",
			"err", err, "handled", true)
		return
	}
	slog.DebugContext(ctx, "recordings queue publish failed, chunks bypass it (applied on the request, or buffered on disk in a reader)",
		"err", err, "handled", true)
}

// toFallback buffers chunk in a reader and reports it queued. Without a
// fallback it reports false, and the caller applies the chunk itself.
func (q *recordingsQueue) toFallback(ctx context.Context, chunk command.ApplyRecordingChunk) bool {
	if q.fallback == nil {
		return false
	}
	q.fallback.fallback.Submit(ctx, chunk)
	return true
}

// Close drains the consumer, if any, closes a reader's fallback file, then
// the client. Safe on a nil queue.
func (q *recordingsQueue) Close(ctx context.Context) error {
	if q == nil {
		return nil
	}
	var errs []error
	if q.bus != nil {
		errs = append(errs, q.bus.Close(ctx))
	}
	if q.fallback != nil {
		errs = append(errs, q.fallback.fallback.Close(ctx))
	}
	return errors.Join(append(errs, q.client.Close())...)
}
