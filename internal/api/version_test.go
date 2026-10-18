package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The deploy smoke compares this response with what it just rolled out, so the
// shape is a contract: three string fields, public, and the same version the
// health endpoint reports.
func TestVersionEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.version = "v1.2.3"
	srv.commit = "0123456789abcdef0123456789abcdef01234567"
	srv.buildTime = "2026-10-17T12:00:00Z"

	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/version", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without credentials", w.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	want := map[string]string{
		"version": "v1.2.3",
		"commit":  "0123456789abcdef0123456789abcdef01234567",
		"built":   "2026-10-17T12:00:00Z",
	}
	if len(got) != len(want) {
		t.Errorf("got %v, want exactly %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}

	h := httptest.NewRecorder()
	srv.mux.ServeHTTP(h, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	var health map[string]any
	if err := json.Unmarshal(h.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode health %q: %v", h.Body.String(), err)
	}
	if health["version"] != got["version"] {
		t.Errorf("health reports %v, version endpoint %q; both must read the same source", health["version"], got["version"])
	}
}
