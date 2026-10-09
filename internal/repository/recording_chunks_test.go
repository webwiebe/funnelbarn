package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func countTraceLinks(t *testing.T, s *repository.Store, recordingID string) int {
	t.Helper()
	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM recording_traces WHERE recording_id = ?`, recordingID).Scan(&n))
	return n
}

// A chunk delivered twice (an SDK retry, or a queue redelivery) used to bump
// chunk_count twice.
func TestApplyChunk_ReplayIsANoOp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Chunks", "chunks")
	require.NoError(t, err)

	start := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p.ID, "sess-ch", start)
	rec.ID = "rec-ch"
	links := []repository.TraceLink{{TraceID: "t1", OccurredAt: start.Add(time.Second)}}

	applied, err := s.ApplyChunk(ctx, rec, 0, links)
	require.NoError(t, err)
	assert.True(t, applied)

	applied, err = s.ApplyChunk(ctx, rec, 0, links)
	require.NoError(t, err)
	assert.False(t, applied, "the second delivery of chunk 0 must be skipped")

	got, err := s.GetRecording(ctx, "rec-ch")
	require.NoError(t, err)
	assert.Equal(t, 1, got.ChunkCount)
	assert.Equal(t, 1, countTraceLinks(t, s, "rec-ch"))
}

func TestApplyChunk_DistinctChunksCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Chunks", "chunks")
	require.NoError(t, err)

	start := time.Now().UTC().Truncate(time.Second)
	for _, idx := range []int{1, 0, 2, 1} {
		rec := makeRecording(p.ID, "sess-ch", start)
		rec.ID = "rec-ch"
		rec.FirstChunkIndex, rec.LastChunkIndex = idx, idx
		_, err := s.ApplyChunk(ctx, rec, idx, nil)
		require.NoError(t, err)
	}

	got, err := s.GetRecording(ctx, "rec-ch")
	require.NoError(t, err)
	assert.Equal(t, 3, got.ChunkCount)
	assert.Equal(t, 0, got.FirstChunkIndex)
	assert.Equal(t, 2, got.LastChunkIndex)
}

// A failure after the recording upsert must leave nothing behind, so the retry
// applies the chunk in full and counts it once.
func TestApplyChunk_FailureStoresNothing(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Chunks", "chunks")
	require.NoError(t, err)

	_, err = s.DB().Exec(`CREATE TRIGGER fail_trace_links BEFORE INSERT ON recording_traces
		BEGIN SELECT RAISE(ABORT, 'trace links unavailable'); END`)
	require.NoError(t, err)

	start := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p.ID, "sess-ch", start)
	rec.ID = "rec-ch"
	links := []repository.TraceLink{{TraceID: "t1", OccurredAt: start}}

	_, err = s.ApplyChunk(ctx, rec, 0, links)
	require.Error(t, err)
	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM recordings WHERE id = 'rec-ch'`).Scan(&n))
	assert.Equal(t, 0, n, "the recording upsert must roll back with the failed trace links")

	_, err = s.DB().Exec(`DROP TRIGGER fail_trace_links`)
	require.NoError(t, err)

	applied, err := s.ApplyChunk(ctx, rec, 0, links)
	require.NoError(t, err)
	assert.True(t, applied)
	got, err := s.GetRecording(ctx, "rec-ch")
	require.NoError(t, err)
	assert.Equal(t, 1, got.ChunkCount)
	assert.Equal(t, 1, countTraceLinks(t, s, "rec-ch"))
}

// Deleting a recording drops its chunk markers with it, so the id can be
// recorded again from scratch.
func TestApplyChunk_DeleteRecordingClearsChunks(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Chunks", "chunks")
	require.NoError(t, err)

	rec := makeRecording(p.ID, "sess-ch", time.Now().UTC().Truncate(time.Second))
	rec.ID = "rec-ch"
	_, err = s.ApplyChunk(ctx, rec, 0, nil)
	require.NoError(t, err)
	require.NoError(t, s.DeleteRecording(ctx, "rec-ch"))

	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM recording_chunks WHERE recording_id = 'rec-ch'`).Scan(&n))
	assert.Equal(t, 0, n)

	applied, err := s.ApplyChunk(ctx, rec, 0, nil)
	require.NoError(t, err)
	assert.True(t, applied)
}
