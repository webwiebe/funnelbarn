package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// memStorage is an in-memory RecordingStorage for tests.
type memStorage struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newMemStorage() *memStorage { return &memStorage{m: map[string][]byte{}} }

func (s *memStorage) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	s.m[key] = cp
	return nil
}

func (s *memStorage) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return v, nil
}

func (s *memStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

// newRecordingServer builds a server with session recording enabled and a
// DB-backed API key authorizer (so a key can be bound to a real project).
func newRecordingServer(t *testing.T) (*Server, *repository.Store) {
	t.Helper()
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	authz := auth.New("").WithDBLookup(store.ValidAPIKeySHA256, store.TouchAPIKey)
	ingestHandler := ingest.NewHandler(authz, sp, 0)
	sm := auth.NewSessionManager("test-secret", time.Hour)
	userAuth, _ := auth.NewUserAuthenticator("", "", "")
	recSvc := service.NewRecordingService(store, store, store, newMemStorage())

	srv := NewServer(ServerConfig{
		Ingest:              ingestHandler,
		Projects:            service.NewProjectService(store),
		Funnels:             service.NewFunnelService(store),
		ABTests:             service.NewABTestService(store),
		Flags:               service.NewFlagService(store),
		Events:              service.NewEventService(store),
		Overview:            service.NewOverviewService(store),
		Sessions:            service.NewSessionService(store),
		APIKeys:             service.NewAPIKeyService(store),
		Widgets:             service.NewWidgetService(store),
		Recordings:          recSvc,
		UserAuth:            userAuth,
		SessionManager:      sm,
		SessionSecret:       "test-secret",
		PublicURL:           "http://localhost",
		LoginRatePerMinute:  1000,
		LoginRateBurst:      1000,
		APIRatePerMinute:    1000,
		APIRateBurst:        1000,
		IngestRatePerMinute: 1000,
		IngestRateBurst:     1000,
		DB:                  store,
		Version:             "test",
	})
	return srv, store
}

// projectKey creates a project and an ingest-scoped API key bound to it,
// returning the project ID and the plaintext key.
func projectKey(t *testing.T, store *repository.Store, name, slug string) (projectID, plaintextKey string) {
	t.Helper()
	ctx := context.Background()
	p, err := store.CreateProject(ctx, name, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	plaintextKey = "key-" + slug
	sum := sha256.Sum256([]byte(plaintextKey))
	if _, err := store.CreateAPIKey(ctx, "test", p.ID, hex.EncodeToString(sum[:]), "ingest"); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	return p.ID, plaintextKey
}

func postChunk(t *testing.T, srv *Server, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recordings/chunk", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, key)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func getTrace(t *testing.T, srv *Server, key, traceID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/"+traceID, nil)
	if key != "" {
		req.Header.Set(auth.HeaderAPIKey, key)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestLookupTrace_EndToEnd(t *testing.T) {
	srv, store := newRecordingServer(t)
	projectID, key := projectKey(t, store, "Trace E2E", "trace-e2e")

	start := time.Now().UTC().Truncate(time.Second)
	traceTime := start.Add(4 * time.Second)

	// Ingest a recording chunk carrying a trace link.
	w := postChunk(t, srv, key, map[string]any{
		"recording_id": "recE2E",
		"session_id":   "sessE2E",
		"chunk_index":  0,
		"events":       json.RawMessage(`[{"type":2,"data":{}}]`),
		"started_at":   start,
		"duration_ms":  5000,
		"page_url":     "https://shop.example/checkout",
		"traces": []map[string]any{
			{"trace_id": "trace-xyz", "span_id": "span-1", "url": "https://shop.example/api/pay", "occurred_at": traceTime},
		},
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("chunk ingest: expected 202, got %d (body: %s)", w.Code, w.Body.String())
	}

	// Resolve the trace back to the recording.
	wl := getTrace(t, srv, key, "trace-xyz")
	if wl.Code != http.StatusOK {
		t.Fatalf("lookup: expected 200, got %d (body: %s)", wl.Code, wl.Body.String())
	}
	var got repository.TraceLookup
	if err := json.Unmarshal(wl.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.RecordingID != "recE2E" || got.SessionID != "sessE2E" {
		t.Errorf("lookup mismatch: %+v", got)
	}
	if got.ProjectID != projectID {
		t.Errorf("project: want %s, got %s", projectID, got.ProjectID)
	}
	if got.OffsetMs != 4000 {
		t.Errorf("offset: want 4000ms, got %d", got.OffsetMs)
	}
	// Chunk range + metadata travel with the lookup so a replay client can fetch
	// without a second round-trip.
	if got.ChunkCount != 1 || got.FirstChunkIndex != 0 || got.LastChunkIndex != 0 {
		t.Errorf("chunk range: got first=%d last=%d count=%d", got.FirstChunkIndex, got.LastChunkIndex, got.ChunkCount)
	}
	if got.PageURL != "https://shop.example/checkout" {
		t.Errorf("page_url: got %q", got.PageURL)
	}

	// The same API key can fetch the recording's chunks for replay.
	wc := getChunkByKey(t, srv, key, "recE2E", 0)
	if wc.Code != http.StatusOK {
		t.Fatalf("chunk fetch: expected 200, got %d (body: %s)", wc.Code, wc.Body.String())
	}
	var events []map[string]any
	if err := json.Unmarshal(wc.Body.Bytes(), &events); err != nil {
		t.Fatalf("chunk unmarshal: %v (body: %s)", err, wc.Body.String())
	}
	if len(events) != 1 || events[0]["type"] != float64(2) {
		t.Errorf("expected one full-snapshot event, got %v", events)
	}
}

func getChunkByKey(t *testing.T, srv *Server, key, rid string, index int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings/"+rid+"/chunks/"+strconv.Itoa(index), nil)
	if key != "" {
		req.Header.Set(auth.HeaderAPIKey, key)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestGetRecordingChunkByKey_CrossProjectDenied(t *testing.T) {
	srv, store := newRecordingServer(t)
	_, keyA := projectKey(t, store, "Owner", "owner-proj")
	_, keyB := projectKey(t, store, "Other", "other-proj")

	start := time.Now().UTC().Truncate(time.Second)
	w := postChunk(t, srv, keyA, map[string]any{
		"recording_id": "recOwned",
		"session_id":   "sessOwned",
		"chunk_index":  0,
		"events":       json.RawMessage(`[{"type":2,"data":{}}]`),
		"started_at":   start,
		"duration_ms":  1000,
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("ingest: expected 202, got %d", w.Code)
	}

	// Project B must not read project A's recording chunk.
	wc := getChunkByKey(t, srv, keyB, "recOwned", 0)
	if wc.Code != http.StatusNotFound {
		t.Errorf("cross-project chunk read: expected 404, got %d", wc.Code)
	}

	// No key at all -> 401.
	wn := getChunkByKey(t, srv, "", "recOwned", 0)
	if wn.Code != http.StatusUnauthorized {
		t.Errorf("no key chunk read: expected 401, got %d", wn.Code)
	}
}

func TestLookupTrace_Unauthorized(t *testing.T) {
	srv, _ := newRecordingServer(t)
	w := getTrace(t, srv, "", "anything")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no key: expected 401, got %d", w.Code)
	}
}

func TestLookupTrace_NotFound(t *testing.T) {
	srv, store := newRecordingServer(t)
	_, key := projectKey(t, store, "Trace NF", "trace-nf")
	w := getTrace(t, srv, key, "unknown-trace")
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown trace: expected 404, got %d (body: %s)", w.Code, w.Body.String())
	}
}
