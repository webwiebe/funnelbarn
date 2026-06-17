package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestRecordingTraces_InsertAndLookup(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Traces", "traces")
	require.NoError(t, err)

	start := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p.ID, "sess-tr", start)
	rec.ID = "rec-tr"
	require.NoError(t, s.UpsertRecording(ctx, rec))

	// Trace fired 3.5s into the recording.
	traceTime := start.Add(3500 * time.Millisecond)
	links := []repository.TraceLink{
		{TraceID: "abc123", SpanID: "span1", URL: "https://example.com/checkout", OccurredAt: traceTime},
	}
	require.NoError(t, s.InsertTraceLinks(ctx, p.ID, "sess-tr", "rec-tr", links))

	got, found, err := s.LookupTrace(ctx, p.ID, "abc123")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "rec-tr", got.RecordingID)
	assert.Equal(t, "sess-tr", got.SessionID)
	assert.Equal(t, p.ID, got.ProjectID)
	assert.Equal(t, "https://example.com/checkout", got.URL)
	assert.Equal(t, int64(3500), got.OffsetMs, "offset = occurred_at - recording.started_at")
}

func TestRecordingTraces_LookupUnknownTrace(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Traces NF", "traces-nf")
	require.NoError(t, err)

	_, found, err := s.LookupTrace(ctx, p.ID, "does-not-exist")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestRecordingTraces_LookupScopedByProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p1, err := s.CreateProject(ctx, "Proj One", "proj-one")
	require.NoError(t, err)
	p2, err := s.CreateProject(ctx, "Proj Two", "proj-two")
	require.NoError(t, err)

	start := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p1.ID, "sess-sc", start)
	rec.ID = "rec-sc"
	require.NoError(t, s.UpsertRecording(ctx, rec))
	require.NoError(t, s.InsertTraceLinks(ctx, p1.ID, "sess-sc", "rec-sc",
		[]repository.TraceLink{{TraceID: "shared-trace", OccurredAt: start}}))

	// A different project must not resolve another project's trace.
	_, found, err := s.LookupTrace(ctx, p2.ID, "shared-trace")
	require.NoError(t, err)
	assert.False(t, found, "trace lookup must be scoped to the owning project")
}

func TestRecordingTraces_LookupReturnsEarliest(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Earliest", "earliest")
	require.NoError(t, err)

	start := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p.ID, "sess-e", start)
	rec.ID = "rec-e"
	require.NoError(t, s.UpsertRecording(ctx, rec))

	// Same trace_id observed twice (e.g. across chunks); earliest wins as seek target.
	require.NoError(t, s.InsertTraceLinks(ctx, p.ID, "sess-e", "rec-e", []repository.TraceLink{
		{TraceID: "dup", OccurredAt: start.Add(8 * time.Second)},
		{TraceID: "dup", OccurredAt: start.Add(2 * time.Second)},
	}))

	got, found, err := s.LookupTrace(ctx, p.ID, "dup")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, int64(2000), got.OffsetMs)
}

func TestRecordingTraces_InsertIdempotent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Idem", "idem")
	require.NoError(t, err)

	start := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p.ID, "sess-i", start)
	rec.ID = "rec-i"
	require.NoError(t, s.UpsertRecording(ctx, rec))

	link := repository.TraceLink{TraceID: "t1", SpanID: "s1", OccurredAt: start.Add(time.Second)}
	// A retried chunk re-sends the same link; the PK conflict must be a no-op.
	require.NoError(t, s.InsertTraceLinks(ctx, p.ID, "sess-i", "rec-i", []repository.TraceLink{link}))
	require.NoError(t, s.InsertTraceLinks(ctx, p.ID, "sess-i", "rec-i", []repository.TraceLink{link}))

	traces, err := s.TracesForRecording(ctx, "rec-i")
	require.NoError(t, err)
	assert.Len(t, traces, 1, "duplicate (recording_id, trace_id, occurred_at) must not double-insert")
}

func TestRecordingTraces_ForRecordingOrdered(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Timeline", "timeline")
	require.NoError(t, err)

	start := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p.ID, "sess-tl", start)
	rec.ID = "rec-tl"
	require.NoError(t, s.UpsertRecording(ctx, rec))

	require.NoError(t, s.InsertTraceLinks(ctx, p.ID, "sess-tl", "rec-tl", []repository.TraceLink{
		{TraceID: "c", OccurredAt: start.Add(3 * time.Second)},
		{TraceID: "a", OccurredAt: start.Add(1 * time.Second)},
		{TraceID: "b", OccurredAt: start.Add(2 * time.Second)},
	}))

	traces, err := s.TracesForRecording(ctx, "rec-tl")
	require.NoError(t, err)
	require.Len(t, traces, 3)
	assert.Equal(t, "a", traces[0].TraceID)
	assert.Equal(t, "b", traces[1].TraceID)
	assert.Equal(t, "c", traces[2].TraceID)
}

func TestRecordingTraces_InsertEmptyNoop(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Empty", "empty")
	require.NoError(t, err)

	require.NoError(t, s.InsertTraceLinks(ctx, p.ID, "sess-x", "rec-x", nil))
}
