package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

// cancelingStorage cancels the request context during Put, the way a browser
// closing the tab mid-request does, and fails the upload if the context it was
// handed is cancelled.
type cancelingStorage struct {
	*memStorage
	cancel context.CancelFunc
}

func (s *cancelingStorage) Put(ctx context.Context, key string, data []byte) error {
	s.cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.memStorage.Put(ctx, key, data)
}

// A client that disconnects while its chunk is being stored (tab closed, page
// navigated away) cancels the request context. The upload used to inherit that
// context, fail with "context canceled" and answer 500, losing the chunk and
// raising an error-level issue for every closed tab. The chunk is already read
// in full at that point, so it is stored regardless of the disconnect.
func TestIngestRecordingChunk_ClientDisconnectStillStoresChunk(t *testing.T) {
	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	storage := &cancelingStorage{memStorage: newMemStorage(), cancel: cancel}
	srv, store := newRecordingServerWithStorage(t, storage)
	_, key := projectKey(t, store, "Disconnect", "disconnect")

	body, err := json.Marshal(map[string]any{
		"recording_id": "recDisc",
		"session_id":   "sessDisc",
		"chunk_index":  0,
		"events":       json.RawMessage(`[{"type":2,"data":{}}]`),
		"started_at":   time.Now().UTC(),
		"duration_ms":  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recordings/chunk", bytes.NewReader(body)).WithContext(reqCtx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, key)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(storage.m) != 1 {
		t.Fatalf("want the chunk stored, got %d objects", len(storage.m))
	}
}
