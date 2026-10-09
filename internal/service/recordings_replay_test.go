package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func replayChunk(projectID string) service.RecordingChunk {
	start := time.Now().UTC().Truncate(time.Second)
	return service.RecordingChunk{
		RecordingID: "rec-replay",
		SessionID:   "sess-replay",
		ChunkIndex:  0,
		Events:      json.RawMessage(`[{"type":2,"data":{},"timestamp":1000}]`),
		StartedAt:   start,
		DurationMs:  4000,
		ProjectID:   projectID,
		UserAgent:   "Mozilla/5.0",
		Traces:      []repository.TraceLink{{TraceID: "t-replay", OccurredAt: start.Add(time.Second)}},
	}
}

// The SDK retries a chunk upload whose response it never saw. The retry must
// not count the chunk a second time.
func TestRecordingService_IngestChunk_RetryCountsOnce(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	p, err := service.NewProjectService(store).CreateProject(ctx, "Replay", "replay")
	require.NoError(t, err)
	svc := service.NewRecordingService(store, store, store, newMemStorage())

	chunk := replayChunk(p.ID)
	require.NoError(t, svc.IngestChunk(ctx, chunk))
	require.NoError(t, svc.IngestChunk(ctx, chunk))

	rec, err := store.GetRecording(ctx, "rec-replay")
	require.NoError(t, err)
	assert.Equal(t, 1, rec.ChunkCount)
	assert.True(t, rec.HasSnapshot)
	links, err := store.TracesForRecording(ctx, "rec-replay")
	require.NoError(t, err)
	assert.Len(t, links, 1)
}

// ChunkMeta is what a queue carries between the upload and the apply, so it
// must survive a JSON round trip and apply the same rows.
func TestRecordingService_ChunkMetaSurvivesJSON(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	p, err := service.NewProjectService(store).CreateProject(ctx, "Replay", "replay")
	require.NoError(t, err)
	storage := newMemStorage()
	svc := service.NewRecordingService(store, store, store, storage)

	meta, ok, err := svc.UploadChunk(ctx, replayChunk(p.ID))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Len(t, storage.keys(), 1, "the upload stores the chunk in object storage")

	raw, err := json.Marshal(meta)
	require.NoError(t, err)
	var decoded service.ChunkMeta
	require.NoError(t, json.Unmarshal(raw, &decoded))

	require.NoError(t, svc.ApplyChunkMeta(ctx, decoded))
	require.NoError(t, svc.ApplyChunkMeta(ctx, decoded))

	rec, err := store.GetRecording(ctx, "rec-replay")
	require.NoError(t, err)
	assert.Equal(t, p.ID, rec.ProjectID)
	assert.Equal(t, "sess-replay", rec.SessionID)
	assert.Equal(t, 1, rec.ChunkCount)
	assert.Equal(t, int64(4000), rec.DurationMs)
	assert.True(t, rec.HasSnapshot)
	require.NotNil(t, rec.EndedAt)
	assert.Equal(t, meta.Recording.EndedAt.UTC(), rec.EndedAt.UTC())
}

// A bot chunk is dropped before upload, so there is nothing to apply.
func TestRecordingService_UploadChunk_BotIsDropped(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	storage := newMemStorage()
	svc := service.NewRecordingService(store, store, store, storage)

	chunk := replayChunk("p")
	chunk.UserAgent = "Googlebot/2.1 (+http://www.google.com/bot.html)"
	_, ok, err := svc.UploadChunk(ctx, chunk)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, storage.keys())
}

// fakeChunkQueue records what it is handed and takes it when accept is set.
type fakeChunkQueue struct {
	accept bool
	got    []service.ChunkMeta
}

func (q *fakeChunkQueue) Enqueue(_ context.Context, meta service.ChunkMeta) bool {
	q.got = append(q.got, meta)
	return q.accept
}

// With a chunk queue set, a queued chunk is left to the queue's consumer, and
// one the queue refuses is applied on the request as before.
func TestRecordingService_IngestChunk_ChunkQueue(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	p, err := service.NewProjectService(store).CreateProject(ctx, "Replay", "replay")
	require.NoError(t, err)
	svc := service.NewRecordingService(store, store, store, newMemStorage())
	q := &fakeChunkQueue{accept: true}
	svc.SetChunkQueue(q)

	chunk := replayChunk(p.ID)
	require.NoError(t, svc.IngestChunk(ctx, chunk))
	require.Len(t, q.got, 1)
	assert.Equal(t, 0, q.got[0].ChunkIndex)
	_, err = store.GetRecording(ctx, "rec-replay")
	require.Error(t, err, "a queued chunk is not applied on the request")

	q.accept = false
	require.NoError(t, svc.IngestChunk(ctx, chunk))
	rec, err := store.GetRecording(ctx, "rec-replay")
	require.NoError(t, err)
	assert.Equal(t, 1, rec.ChunkCount)
}
