package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A recording that does not exist, or belongs to another project, answers 404
// on every project-scoped recording route (#305). A missing id used to reach
// the API as sql.ErrNoRows and answer 500.
func TestRecordingRoutes_MissingRecordingIs404(t *testing.T) {
	srv, store := newRecordingServer(t)
	projectA, _ := projectKey(t, store, "NF A", "nf-a")
	_, keyB := projectKey(t, store, "NF B", "nf-b")
	w := postChunk(t, srv, keyB, map[string]any{
		"recording_id": "rec-of-b",
		"session_id":   "sess-of-b",
		"chunk_index":  0,
		"events":       json.RawMessage(`[{"type":2,"data":{}}]`),
		"started_at":   time.Now().UTC().Truncate(time.Second),
		"duration_ms":  1000,
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("chunk ingest: expected 202, got %d (body: %s)", w.Code, w.Body.String())
	}
	cookie := sessionCookieFor(t, srv, "admin")

	base := "/api/v1/projects/" + projectA + "/recordings/"
	for _, id := range []string{"rec-missing", "rec-of-b"} {
		for _, tc := range []struct{ method, path string }{
			{http.MethodDelete, base + id},
			{http.MethodGet, base + id + "/traces"},
			{http.MethodGet, base + id + "/flags"},
			{http.MethodGet, base + id + "/chunks/0"},
		} {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s: expected 404, got %d (body: %s)", tc.method, tc.path, rec.Code, rec.Body.String())
			}
		}
	}

	if _, err := store.GetRecording(t.Context(), "rec-of-b"); err != nil {
		t.Errorf("project B's recording must survive a delete through project A: %v", err)
	}
}
