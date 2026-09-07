package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

// The production shapes from the audit. A client that joined its base URL with
// the ingest path again posts to a doubled path and was silently 404'd — ~189
// a week against ~110 correct posts.
func TestCanonicalAPIPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
		ok   bool
	}{
		{"/api/v1/events/api/v1/events", "/api/v1/events", true},
		{"/api/api/v1/evaluate", "/api/v1/evaluate", true},
		{"/api/v1/events/api/v1/recording-config", "/api/v1/recording-config", true},
		{"/api/v1/events/api/v1/events/api/v1/events", "/api/v1/events", true},

		// A single prefix is the normal case and must not be rewritten.
		{"/api/v1/events", "", false},
		{"/api/v1/recordings/chunk", "", false},
		{"/api/v1/projects/abc/funnels", "", false},
		{"/", "", false},
		{"", "", false},
		// The prefix at position 0 only — nothing to strip.
		{"/api/v1/", "", false},
	} {
		got, ok := canonicalAPIPath(tc.path)
		if ok != tc.ok || got != tc.want {
			t.Errorf("canonicalAPIPath(%q) = (%q, %v), want (%q, %v)", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

// The result must always be a path that cannot itself be rewritten, or a
// redirect would loop.
func TestCanonicalAPIPath_ResultIsStable(t *testing.T) {
	for _, p := range []string{
		"/api/v1/events/api/v1/events",
		"/api/api/v1/evaluate",
		"/api/v1/events/api/v1/events/api/v1/events",
	} {
		once, ok := canonicalAPIPath(p)
		if !ok {
			t.Fatalf("canonicalAPIPath(%q) did not rewrite", p)
		}
		if twice, ok := canonicalAPIPath(once); ok {
			t.Errorf("canonicalAPIPath(%q) = %q, which rewrites again to %q — a redirect would loop", p, once, twice)
		}
	}
}

// The counter's label set has to stay bounded: unmatched paths are chosen by
// the caller, so a scanner must not be able to mint a series per probe.
func TestCanonicalMetricPath_IsBounded(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/api/v1/events", "/api/v1/events"},
		{"/api/v1/events/api/v1/events", "/api/v1/events"},
		{"/api/api/v1/evaluate", "/api/v1/evaluate"},
		{"/api/v1/recordings/chunk", "/api/v1/recordings/chunk"},
		// Caller-controlled junk collapses to one series.
		{"/api/v1/../../etc/passwd", "other"},
		{"/api/v1/wp-admin.php", "other"},
		{"/api/v1/projects/some-uuid/funnels", "other"},
	} {
		if got := canonicalMetricPath(tc.path); got != tc.want {
			t.Errorf("canonicalMetricPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// End to end: a client posting to the doubled ingest path gets a 308 to the
// real one instead of a silent 404, and the redirect preserves the method so
// the event survives the round trip.
func TestServeHTTP_DoubledIngestPathRedirects(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/api/v1/events",
		strings.NewReader(`{"name":"pageview"}`))
	req.Header.Set(auth.HeaderAPIKey, "test-key")
	req.Header.Set("x-funnelbarn-project", "my-site")
	req.Header.Set("Origin", "https://example.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusPermanentRedirect {
		t.Fatalf("want 308, got %d (body: %s)", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/api/v1/events" {
		t.Errorf("Location: want /api/v1/events, got %q", loc)
	}
}

// The query string has to survive, or a redirected GET loses its parameters.
func TestServeHTTP_DoubledPathKeepsQuery(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/api/v1/recording-config?project=my-site", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusPermanentRedirect {
		t.Fatalf("want 308, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/api/v1/recording-config?project=my-site" {
		t.Errorf("Location: want the query preserved, got %q", loc)
	}
}

// A correctly-addressed request must not be redirected.
func TestServeHTTP_CorrectPathIsNotRedirected(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(`{"name":"pageview"}`))
	req.Header.Set(auth.HeaderAPIKey, "test-key")
	req.Header.Set("x-funnelbarn-project", "my-site")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code == http.StatusPermanentRedirect {
		t.Errorf("a correct path was redirected to %q", w.Header().Get("Location"))
	}
}
