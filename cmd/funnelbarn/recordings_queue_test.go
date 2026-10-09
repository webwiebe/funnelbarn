package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/queue"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// memChunkStorage keeps uploaded chunks in memory in place of R2.
type memChunkStorage struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (m *memChunkStorage) Put(_ context.Context, key string, b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = b
	return nil
}

func (m *memChunkStorage) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[key], nil
}

func (m *memChunkStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func newTestRecordings(t *testing.T) (*service.RecordingService, *repository.Store, string) {
	t.Helper()
	_, store := newTestApplier(t)
	p, err := service.NewProjectService(store).CreateProject(context.Background(), "Rec", "rec-site")
	require.NoError(t, err)
	svc := service.NewRecordingService(store, store, store, &memChunkStorage{data: map[string][]byte{}})
	return svc, store, p.ID
}

func testChunk(projectID string) service.RecordingChunk {
	start := time.Now().UTC().Truncate(time.Second)
	return service.RecordingChunk{
		RecordingID: "rec-q",
		SessionID:   "sess-q",
		ChunkIndex:  0,
		Events:      json.RawMessage(`[{"type":2,"data":{},"timestamp":1000}]`),
		StartedAt:   start,
		DurationMs:  3000,
		ProjectID:   projectID,
		UserAgent:   "Mozilla/5.0",
	}
}

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

func TestNewRecordingsQueueOffIsNil(t *testing.T) {
	svc, _, _ := newTestRecordings(t)
	q, err := newRecordingsQueue(context.Background(), config.Config{RedisQueueURL: "redis://unused"}, svc, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	require.Nil(t, q)
	require.NoError(t, q.Close(context.Background()))
}

func TestNewRecordingsQueueNeedsRedisURL(t *testing.T) {
	svc, _, _ := newTestRecordings(t)
	_, err := newRecordingsQueue(context.Background(), config.Config{RecordingsViaQueue: true}, svc, slog.New(slog.DiscardHandler))
	require.ErrorContains(t, err, "FUNNELBARN_REDIS_QUEUE_URL")
}

// A chunk goes handler -> R2 + Valkey -> consumer -> SQLite. An SDK retry of
// the same chunk travels the queue twice and is counted once.
func TestRecordingsQueueAppliesChunks(t *testing.T) {
	svc, store, projectID := newTestRecordings(t)
	mr := miniredis.RunT(t)
	cfg := config.Config{
		RedisQueueURL:         "redis://" + mr.Addr(),
		RecordingsViaQueue:    true,
		RecordingsQueueMaxLen: 100,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q, err := newRecordingsQueue(ctx, cfg, svc, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	defer func() { require.NoError(t, q.Close(ctx)) }()
	svc.SetChunkQueue(q)

	chunk := testChunk(projectID)
	require.NoError(t, svc.IngestChunk(ctx, chunk))
	require.NoError(t, svc.IngestChunk(ctx, chunk))
	require.NoError(t, q.bus.Flush(ctx))

	rec, err := store.GetRecording(ctx, "rec-q")
	require.NoError(t, err)
	require.Equal(t, 1, rec.ChunkCount)
	require.Equal(t, projectID, rec.ProjectID)
}

// Enqueue refuses a chunk while the queue is at its cap or Valkey is down, so
// the handler applies it on the request.
func TestRecordingsQueueRefusesWhenFullOrDown(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client, err := queue.NewClient("redis://" + mr.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	q := &recordingsQueue{list: queue.NewRedisList(client, recordingsQueueName), maxLen: 1}
	meta := service.ChunkMeta{Recording: repository.Recording{ID: "rec-1", ProjectID: "p"}, ChunkIndex: 0}

	require.True(t, q.Enqueue(ctx, meta))
	require.False(t, q.Enqueue(ctx, meta), "the queue is at its cap")
	require.False(t, q.Enqueue(ctx, meta), "still full; logged once")
	mr.Del("funnelbarn:queue:" + recordingsQueueName)
	require.True(t, q.Enqueue(ctx, meta), "the consumer made room")

	mr.Close()
	require.False(t, q.Enqueue(ctx, meta), "Valkey is down")
	require.False(t, q.Enqueue(ctx, meta), "still down; logged once")
}

// flakyChunkList reports room but fails publishes while failPublish is set.
type flakyChunkList struct {
	failPublish bool
	published   int
}

func (l *flakyChunkList) Publish(context.Context, []byte) error {
	if l.failPublish {
		return errors.New("connection reset")
	}
	l.published++
	return nil
}

func (l *flakyChunkList) QueuedLen(context.Context) (int64, error) { return 0, nil }

func TestRecordingsQueueRefusesWhenPublishFails(t *testing.T) {
	ctx := context.Background()
	list := &flakyChunkList{failPublish: true}
	q := &recordingsQueue{list: list, maxLen: 10}
	meta := service.ChunkMeta{Recording: repository.Recording{ID: "rec-1", ProjectID: "p"}}

	require.False(t, q.Enqueue(ctx, meta))
	list.failPublish = false
	require.True(t, q.Enqueue(ctx, meta), "the publish recovered")
	require.Equal(t, 1, list.published)
}

// A chunk whose metadata cannot be encoded is refused, so the handler applies
// it on the request.
func TestRecordingsQueueRefusesUnencodableChunk(t *testing.T) {
	list := &flakyChunkList{}
	q := &recordingsQueue{list: list, maxLen: 10}
	bad := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) // outside RFC 3339
	meta := service.ChunkMeta{Recording: repository.Recording{ID: "rec-1", StartedAt: bad}}
	require.False(t, q.Enqueue(context.Background(), meta))
	require.Zero(t, list.published)
}

func TestDeadLetterChunkLogsAnUnhandledError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	c := command.ApplyRecordingChunk{Recording: repository.Recording{ID: "rec-9", ProjectID: "p-9"}, ChunkIndex: 4}
	deadLetterChunk(logger)(context.Background(), c, errors.New("database is locked"))

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	require.Equal(t, "ERROR", line["level"])
	require.Equal(t, false, line["handled"])
	require.Equal(t, "rec-9", line["recording_id"])
	require.EqualValues(t, 4, line["chunk_index"])
}
