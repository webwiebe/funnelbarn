package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Without recording storage every recording route answers in a fixed way: a
// 503 for most, an empty 200 for the three list routes, and the ingest route
// keeps its own message. The guard runs before key authentication on the
// API-key routes and after session authentication on the dashboard routes.
func TestRecordingRoutes_WhenNotConfigured(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, _ := store.CreateProject(context.Background(), "NoRec", "norec")
	cookie, csrf := sessionAndCSRF(t, srv, "u")
	base := "/api/v1/projects/" + p.ID

	cases := []struct {
		name, method, path, body string
		session                  bool
		wantCode                 int
		wantBody                 string
	}{
		{"list", http.MethodGet, base + "/recordings", "", true, http.StatusOK, `"recordings":[]`},
		{"chunk", http.MethodGet, base + "/recordings/r1/chunks/0", "", true, http.StatusServiceUnavailable, "session recording not configured"},
		{"delete", http.MethodDelete, base + "/recordings/r1", "", true, http.StatusServiceUnavailable, "session recording not configured"},
		{"flags", http.MethodGet, base + "/recordings/r1/flags", "", true, http.StatusServiceUnavailable, "session recording not configured"},
		{"traces", http.MethodGet, base + "/recordings/r1/traces", "", true, http.StatusServiceUnavailable, "session recording not configured"},
		{"funnel step sessions", http.MethodGet, base + "/funnels/f1/steps/1/sessions", "", true, http.StatusOK, `"session_ids":[]`},
		{"flow page sessions", http.MethodGet, base + "/flows/sessions?page=/", "", true, http.StatusOK, `"session_ids":[]`},
		{"trace lookup", http.MethodGet, "/api/v1/traces/abc", "", false, http.StatusServiceUnavailable, "session recording not configured"},
		{"chunk by key", http.MethodGet, "/api/v1/recordings/r1/chunks/0", "", false, http.StatusServiceUnavailable, "session recording not configured"},
		{"ingest", http.MethodPost, "/api/v1/recordings/chunk", `{}`, false, http.StatusServiceUnavailable, "(R2 credentials missing)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			if tc.session {
				req.AddCookie(cookie)
				req.Header.Set("X-FunnelBarn-CSRF", csrf)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Fatalf("want %d, got %d (body: %s)", tc.wantCode, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Errorf("body %q does not contain %q", w.Body.String(), tc.wantBody)
			}
		})
	}
}
