package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func makeRecording(projectID, sessionID string, start time.Time) repository.Recording {
	return repository.Recording{
		ID:              "placeholder", // caller must override
		ProjectID:       projectID,
		SessionID:       sessionID,
		Environment:     "testing",
		FirstChunkIndex: 0,
		ChunkCount:      1,
		DurationMs:      5000,
		StartedAt:       start,
		DeviceType:      "desktop",
		UserAgent:       "Mozilla/5.0",
		IsBot:           false,
		PageURL:         "https://example.com/",
	}
}

func TestRecording_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Rec Project", "rec-project")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p.ID, "sess-abc", now)
	rec.ID = "rec-001"

	require.NoError(t, s.UpsertRecording(ctx, rec))

	got, err := s.GetRecording(ctx, "rec-001")
	require.NoError(t, err)
	assert.Equal(t, "rec-001", got.ID)
	assert.Equal(t, p.ID, got.ProjectID)
	assert.Equal(t, "sess-abc", got.SessionID)
	assert.Equal(t, "desktop", got.DeviceType)
	assert.False(t, got.IsBot)
	assert.Equal(t, "https://example.com/", got.PageURL)
}

func TestRecording_UpsertUpdatesProgress(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Rec Project 2", "rec-project-2")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p.ID, "sess-upd", now)
	rec.ID = "rec-upd"
	require.NoError(t, s.UpsertRecording(ctx, rec))

	// Each successful ingest increments chunk_count by 1; metadata stays the same.
	rec.DurationMs = 50000
	rec.DeviceType = "mobile" // should NOT be updated
	require.NoError(t, s.UpsertRecording(ctx, rec))
	require.NoError(t, s.UpsertRecording(ctx, rec))

	got, err := s.GetRecording(ctx, "rec-upd")
	require.NoError(t, err)
	assert.Equal(t, 3, got.ChunkCount) // 1 insert + 2 upserts
	assert.Equal(t, int64(50000), got.DurationMs)
	assert.Equal(t, "desktop", got.DeviceType) // unchanged from first insert
}

func TestRecording_FirstChunkIndexPreserved(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "First Chunk", "first-chunk")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	// Simulates chunk 0 being lost — first successful ingest is chunk index 1.
	rec := makeRecording(p.ID, "sess-fc", now)
	rec.ID = "rec-fc"
	rec.FirstChunkIndex = 1
	require.NoError(t, s.UpsertRecording(ctx, rec))

	// Second chunk (index 2) arrives — first_chunk_index must NOT change.
	rec.FirstChunkIndex = 2
	rec.DurationMs = 20000
	require.NoError(t, s.UpsertRecording(ctx, rec))

	got, err := s.GetRecording(ctx, "rec-fc")
	require.NoError(t, err)
	assert.Equal(t, 1, got.FirstChunkIndex) // preserved from first insert
	assert.Equal(t, 2, got.ChunkCount)
}

func TestRecording_OutOfOrderChunksKeepSnapshotInSpan(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "OOO Chunks", "ooo-chunks")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)

	// A small later chunk (index 1) wins the upload race and inserts the row.
	later := makeRecording(p.ID, "sess-ooo", now)
	later.ID = "rec-ooo"
	later.FirstChunkIndex = 1
	later.LastChunkIndex = 1
	later.HasSnapshot = false
	require.NoError(t, s.UpsertRecording(ctx, later))

	// The large snapshot chunk (index 0) lands afterwards.
	snapshot := makeRecording(p.ID, "sess-ooo", now)
	snapshot.ID = "rec-ooo"
	snapshot.FirstChunkIndex = 0
	snapshot.LastChunkIndex = 0
	snapshot.HasSnapshot = true
	require.NoError(t, s.UpsertRecording(ctx, snapshot))

	got, err := s.GetRecording(ctx, "rec-ooo")
	require.NoError(t, err)
	assert.Equal(t, 0, got.FirstChunkIndex, "first_chunk_index must track the lowest index, not arrival order")
	assert.Equal(t, 1, got.LastChunkIndex, "last_chunk_index must track the highest index")
	assert.True(t, got.HasSnapshot, "has_snapshot must stick once the snapshot chunk lands")
	assert.Equal(t, 2, got.ChunkCount)
}

func TestRecording_ListBrokenRecordings(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Broken", "broken")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)

	good := makeRecording(p.ID, "sess-good", now)
	good.ID = "rec-good"
	good.HasSnapshot = true
	require.NoError(t, s.UpsertRecording(ctx, good))

	broken := makeRecording(p.ID, "sess-broken", now)
	broken.ID = "rec-broken"
	broken.FirstChunkIndex = 1
	broken.LastChunkIndex = 1
	broken.HasSnapshot = false
	require.NoError(t, s.UpsertRecording(ctx, broken))

	recs, err := s.ListBrokenRecordings(ctx)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "rec-broken", recs[0].ID)
	assert.Equal(t, p.ID, recs[0].ProjectID)
}

func TestRecording_ListRecordings(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Rec List", "rec-list")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		rec := makeRecording(p.ID, "sess-lst-"+string(rune('a'+i)), now.Add(-time.Duration(i)*time.Minute))
		rec.ID = "rec-lst-" + string(rune('a'+i))
		require.NoError(t, s.UpsertRecording(ctx, rec))
	}

	recs, err := s.ListRecordings(ctx, p.ID, repository.RecordingListOpts{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, recs, 3)
}

func TestRecording_ListRecordings_BotFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Bot Filter", "bot-filter")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)

	human := makeRecording(p.ID, "sess-human", now)
	human.ID = "rec-human"
	human.IsBot = false
	require.NoError(t, s.UpsertRecording(ctx, human))

	bot := makeRecording(p.ID, "sess-bot", now.Add(-time.Second))
	bot.ID = "rec-bot"
	bot.IsBot = true
	require.NoError(t, s.UpsertRecording(ctx, bot))

	recs, err := s.ListRecordings(ctx, p.ID, repository.RecordingListOpts{HumanOnly: true, Limit: 10})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "rec-human", recs[0].ID)
}

func TestRecording_ListRecordings_DeviceFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Device Filter", "device-filter")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	for i, dev := range []string{"desktop", "mobile", "tablet"} {
		rec := makeRecording(p.ID, "sess-dev-"+dev, now.Add(-time.Duration(i)*time.Second))
		rec.ID = "rec-dev-" + dev
		rec.DeviceType = dev
		require.NoError(t, s.UpsertRecording(ctx, rec))
	}

	recs, err := s.ListRecordings(ctx, p.ID, repository.RecordingListOpts{DeviceType: "mobile", Limit: 10})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "rec-dev-mobile", recs[0].ID)
}

func TestRecording_ListRecordings_SessionIDFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "SessID Filter", "sessid-filter")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 4; i++ {
		rec := makeRecording(p.ID, "sess-si-"+string(rune('a'+i)), now.Add(-time.Duration(i)*time.Second))
		rec.ID = "rec-si-" + string(rune('a'+i))
		require.NoError(t, s.UpsertRecording(ctx, rec))
	}

	recs, err := s.ListRecordings(ctx, p.ID, repository.RecordingListOpts{
		SessionIDs: []string{"sess-si-a", "sess-si-c"},
		Limit:      10,
	})
	require.NoError(t, err)
	assert.Len(t, recs, 2)
}

func TestRecording_DeleteRecording(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Del Rec", "del-rec")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	rec := makeRecording(p.ID, "sess-del", now)
	rec.ID = "rec-del"
	require.NoError(t, s.UpsertRecording(ctx, rec))

	require.NoError(t, s.DeleteRecording(ctx, "rec-del"))

	_, err = s.GetRecording(ctx, "rec-del")
	assert.Error(t, err)
}

func TestRecording_ListOldRecordings(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Old Rec", "old-rec")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	old := makeRecording(p.ID, "sess-old", now.Add(-100*24*time.Hour))
	old.ID = "rec-old"
	require.NoError(t, s.UpsertRecording(ctx, old))

	fresh := makeRecording(p.ID, "sess-fresh", now)
	fresh.ID = "rec-fresh"
	require.NoError(t, s.UpsertRecording(ctx, fresh))

	threshold := now.Add(-30 * 24 * time.Hour)
	recs, err := s.ListOldRecordings(ctx, threshold)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "rec-old", recs[0].ID)
	assert.Equal(t, p.ID, recs[0].ProjectID)
}
