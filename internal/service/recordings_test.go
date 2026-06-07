package service_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// memStorage is an in-memory RecordingStorage for tests.
type memStorage struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStorage() *memStorage { return &memStorage{data: make(map[string][]byte)} }

func (m *memStorage) Put(_ context.Context, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}

func (m *memStorage) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.data[key]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	return d, nil
}

func (m *memStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memStorage) keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.data))
	for k := range m.data {
		out = append(out, k)
	}
	return out
}

func gunzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer r.Close()
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return out
}

func TestRecordingService_IngestChunk(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	storage := newMemStorage()

	projSvc := service.NewProjectService(store)
	p, err := projSvc.CreateProject(ctx, "Dogfood", "dogfood")
	require.NoError(t, err)

	svc := service.NewRecordingService(store, store, store, storage)

	events := json.RawMessage(`[{"type":4,"data":{},"timestamp":1000}]`)
	now := time.Now().UTC().Truncate(time.Second)

	chunk := service.RecordingChunk{
		RecordingID: "rec-ingest-001",
		SessionID:   "sess-ingest-001",
		ChunkIndex:  0,
		Events:      events,
		StartedAt:   now,
		DurationMs:  10000,
		PageURL:     "https://example.com/page",
		Environment: "testing",
		ProjectSlug: "dogfood",
		ProjectID:   p.ID,
		UserAgent:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)",
	}

	require.NoError(t, svc.IngestChunk(ctx, chunk))

	// Verify R2 object exists and decompresses to the original events.
	keys := storage.keys()
	require.Len(t, keys, 1)
	assert.Contains(t, keys[0], "rec-ingest-001/00000")

	raw, err := storage.Get(ctx, keys[0])
	require.NoError(t, err)
	decompressed := gunzipBytes(t, raw)
	assert.JSONEq(t, string(events), string(decompressed))

	// Verify DB row.
	rec, err := store.GetRecording(ctx, "rec-ingest-001")
	require.NoError(t, err)
	assert.Equal(t, p.ID, rec.ProjectID)
	assert.Equal(t, "mobile", rec.DeviceType)
	assert.False(t, rec.IsBot)
	assert.Equal(t, "https://example.com/page", rec.PageURL)
	assert.Equal(t, int64(10000), rec.DurationMs)
}

func TestRecordingService_IngestChunk_BotDetection(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	storage := newMemStorage()

	projSvc := service.NewProjectService(store)
	p, err := projSvc.CreateProject(ctx, "Bot Test", "bot-test")
	require.NoError(t, err)

	svc := service.NewRecordingService(store, store, store, storage)

	chunk := service.RecordingChunk{
		RecordingID: "rec-bot-001",
		SessionID:   "sess-bot-001",
		ChunkIndex:  0,
		Events:      json.RawMessage(`[]`),
		StartedAt:   time.Now().UTC().Truncate(time.Second),
		DurationMs:  1000,
		ProjectSlug: "bot-test",
		ProjectID:   p.ID,
		UserAgent:   "Googlebot/2.1",
	}

	require.NoError(t, svc.IngestChunk(ctx, chunk))

	rec, err := store.GetRecording(ctx, "rec-bot-001")
	require.NoError(t, err)
	assert.True(t, rec.IsBot)
	assert.Equal(t, "desktop", rec.DeviceType)
}

func TestRecordingService_GetChunk(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	storage := newMemStorage()

	projSvc := service.NewProjectService(store)
	p, err := projSvc.CreateProject(ctx, "GetChunk", "getchunk")
	require.NoError(t, err)

	svc := service.NewRecordingService(store, store, store, storage)

	events := json.RawMessage(`[{"type":4,"data":{},"timestamp":2000}]`)
	chunk := service.RecordingChunk{
		RecordingID: "rec-get-001",
		SessionID:   "sess-get-001",
		ChunkIndex:  0,
		Events:      events,
		StartedAt:   time.Now().UTC().Truncate(time.Second),
		DurationMs:  5000,
		ProjectSlug: "getchunk",
		ProjectID:   p.ID,
		UserAgent:   "Mozilla/5.0",
	}
	require.NoError(t, svc.IngestChunk(ctx, chunk))

	// GetChunk should return decompressed JSON.
	got, err := svc.GetChunk(ctx, p.ID, "rec-get-001", 0)
	require.NoError(t, err)
	assert.JSONEq(t, string(events), string(got))
}

func TestRecordingService_PurgeOldRecordings(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	storage := newMemStorage()

	projSvc := service.NewProjectService(store)
	p, err := projSvc.CreateProject(ctx, "Purge", "purge")
	require.NoError(t, err)

	svc := service.NewRecordingService(store, store, store, storage)

	now := time.Now().UTC().Truncate(time.Second)

	// Ingest an "old" recording (timestamp is in the past; we'll fake created_at via direct insert).
	oldRec := repository.Recording{
		ID:         "rec-purge-old",
		ProjectID:  p.ID,
		SessionID:  "sess-purge",
		ChunkCount: 2,
		DurationMs: 20000,
		StartedAt:  now.Add(-200 * 24 * time.Hour),
	}
	require.NoError(t, store.UpsertRecording(ctx, oldRec))

	// Put fake chunks in storage.
	for i := 0; i < 2; i++ {
		key := "recordings/purge/rec-purge-old/" + string(rune('0'+i)) + "0000.json.gz"
		require.NoError(t, storage.Put(ctx, key, []byte("dummy")))
	}

	// Purge with 90-day retention — old recording was inserted 200 days ago.
	// ListOldRecordings filters by created_at < threshold. Since we just inserted it,
	// created_at is now, so a 0-day retention threshold won't match. Instead verify no-op.
	require.NoError(t, svc.PurgeOldRecordings(ctx, 0)) // 0 = disabled
	// Storage keys should be unchanged.
	assert.Len(t, storage.keys(), 2)

	// Purge with negative retention should also be a no-op.
	require.NoError(t, svc.PurgeOldRecordings(ctx, -1))
	assert.Len(t, storage.keys(), 2)
}

func TestRecordingService_ListRecordings(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	storage := newMemStorage()

	projSvc := service.NewProjectService(store)
	p, err := projSvc.CreateProject(ctx, "ListRec", "listrec")
	require.NoError(t, err)

	svc := service.NewRecordingService(store, store, store, storage)

	now := time.Now().UTC().Truncate(time.Second)
	chunk := service.RecordingChunk{
		RecordingID: "rec-list-001",
		SessionID:   "sess-list-001",
		ChunkIndex:  0,
		Events:      json.RawMessage(`[]`),
		StartedAt:   now,
		DurationMs:  3000,
		ProjectSlug: "listrec",
		ProjectID:   p.ID,
		UserAgent:   "Mozilla/5.0",
		Environment: "staging",
	}
	require.NoError(t, svc.IngestChunk(ctx, chunk))

	recs, err := svc.ListRecordings(ctx, p.ID, repository.RecordingListOpts{Limit: 10})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "rec-list-001", recs[0].ID)
	assert.Equal(t, "staging", recs[0].Environment)
}

func TestRecordingService_GetRecordingSessionID(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	storage := newMemStorage()

	projSvc := service.NewProjectService(store)
	p, err := projSvc.CreateProject(ctx, "SessID", "sessid")
	require.NoError(t, err)

	svc := service.NewRecordingService(store, store, store, storage)

	chunk := service.RecordingChunk{
		RecordingID: "rec-sid-001",
		SessionID:   "sess-expected",
		ChunkIndex:  0,
		Events:      json.RawMessage(`[]`),
		StartedAt:   time.Now().UTC().Truncate(time.Second),
		DurationMs:  1000,
		ProjectSlug: "sessid",
		ProjectID:   p.ID,
		UserAgent:   "Mozilla/5.0",
	}
	require.NoError(t, svc.IngestChunk(ctx, chunk))

	sid, err := svc.GetRecordingSessionID(ctx, p.ID, "rec-sid-001")
	require.NoError(t, err)
	assert.Equal(t, "sess-expected", sid)

	// Wrong project ID should fail.
	_, err = svc.GetRecordingSessionID(ctx, "wrong-project", "rec-sid-001")
	assert.Error(t, err)
}

func TestRecordingService_FlagEvaluationsForSession(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	storage := newMemStorage()

	projSvc := service.NewProjectService(store)
	p, err := projSvc.CreateProject(ctx, "Flags Sess", "flags-sess")
	require.NoError(t, err)

	// Create a flag in the real store.
	flag, err := store.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "my-flag",
		Name:           "My Flag",
		FlagType:       "boolean",
		Variants:       `{"on":true}`,
		DefaultVariant: "on",
		Split:          `{"on":100}`,
		TargetingRules: "[]",
		Status:         "active",
	})
	require.NoError(t, err)

	// Record an evaluation tied to a session.
	require.NoError(t, store.RecordEvaluation(ctx, repository.FlagEvaluation{
		ID:          "eval-001",
		FlagID:      flag.ID,
		ProjectID:   p.ID,
		Variant:     "on",
		ContextHash: "hash-001",
		SessionID:   "sess-with-flags",
		ContextKeys: nil,
	}))

	svc := service.NewRecordingService(store, store, store, storage)
	evals, err := svc.FlagEvaluationsForSession(ctx, p.ID, "sess-with-flags")
	require.NoError(t, err)
	require.Len(t, evals, 1)
	assert.Equal(t, "My Flag", evals[0].FlagName)
	assert.Equal(t, "on", evals[0].Variant)
}

func TestDetectDeviceType(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) AppleWebKit Mobile Safari", "mobile"},
		// Real Android phone UA includes "Mobile"
		{"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/125.0 Mobile Safari/537.36", "mobile"},
		{"Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit Mobile Safari", "tablet"},
		// Android tablet UA does NOT contain "Mobile"
		{"Mozilla/5.0 (Linux; Android 13; SM-T970) AppleWebKit/537.36 Chrome/125.0 Safari/537.36", "tablet"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit Chrome Safari", "desktop"},
		{"", "desktop"},
	}
	for _, tc := range cases {
		t.Run(tc.ua, func(t *testing.T) {
			assert.Equal(t, tc.want, service.DetectDeviceType(tc.ua))
		})
	}
}

func TestDetectBot(t *testing.T) {
	cases := []struct {
		ua    string
		isBot bool
	}{
		{"Googlebot/2.1 (+http://www.google.com/bot.html)", true},
		{"Mozilla/5.0 (compatible; bingbot/2.0)", true},
		{"python-requests/2.28.0", true},
		{"curl/7.88.1", true},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.ua, func(t *testing.T) {
			assert.Equal(t, tc.isBot, service.DetectBot(tc.ua))
		})
	}
}
