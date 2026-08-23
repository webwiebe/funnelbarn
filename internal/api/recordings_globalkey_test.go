package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// The global env-var key resolves with an empty project ID, which used to
// reach GetProject("") and come back as a bare 404 — indistinguishable from a
// deleted project. Recording chunks must name a project, so this is a 401 with
// an actionable message instead.
func TestIngestRecordingChunk_GlobalKeyRejected(t *testing.T) {
	srv, _ := newRecordingServer(t, "global-instance-key")

	w := postChunk(t, srv, "global-instance-key", map[string]any{
		"recording_id": "rec-1",
		"session_id":   "sess-1",
		"chunk_index":  0,
		"events":       []map[string]any{{"type": 2}},
		"started_at":   time.Now().UTC(),
		"duration_ms":  0,
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for global key, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error == "unauthorized" || body.Error == "not found" {
		t.Errorf("expected an actionable message naming the project-scoped key, got %q", body.Error)
	}
}
