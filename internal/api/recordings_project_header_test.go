package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The 202 for a chunk names the project the chunk was filed under, and warns
// when x-funnelbarn-project names another project. The key wins either way
// (#564).
func TestIngestRecordingChunk_NamesTheProjectAndWarnsOnAMismatch(t *testing.T) {
	srv, store := newRecordingServer(t)
	_, key := projectKey(t, store, "Rec Hdr", "rec-hdr")

	cases := []struct {
		id, header string
		wantWarn   bool
	}{
		{"rec-hdr-absent", "", false},
		{"rec-hdr-match", "rec-hdr", false},
		{"rec-hdr-other", "somewhere-else", true},
	}
	for _, c := range cases {
		req := chunkRequest(t, key, validChunk(c.id), "")
		if c.header != "" {
			req.Header.Set("x-funnelbarn-project", c.header)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Fatalf("%s: got %d: %s", c.id, w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v", c.id, err)
		}
		if body["project"] != "rec-hdr" {
			t.Errorf("%s: project %v, want rec-hdr", c.id, body["project"])
		}
		if _, warned := body["warning"]; warned != c.wantWarn {
			t.Errorf("%s: warning present %v, want %v (body %s)", c.id, warned, c.wantWarn, w.Body.String())
		}
	}
}
