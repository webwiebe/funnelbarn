package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

func postTelemetry(t *testing.T, srv *Server, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-FunnelBarn-CSRF", csrf)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func validSpan() map[string]any {
	return map[string]any{
		"traceId":   "4bf92f3577b34da6a3ce929d0e0e4736",
		"spanId":    "00f067aa0ba902b7",
		"name":      "page /overview",
		"service":   "funnelbarn-web",
		"kind":      "INTERNAL",
		"status":    "OK",
		"startTime": 1700000000000000,
		"duration":  1234,
	}
}

// A browser must never hold a telemetry key, which is the entire reason these
// endpoints exist — so they must actually require the session.
func TestTelemetry_RequiresSession(t *testing.T) {
	srv, _ := newAuthedServer(t)
	for _, path := range []string{"/api/v1/telemetry", "/api/v1/client-errors"} {
		w := postTelemetry(t, srv, path, map[string]any{"spans": []any{}, "message": "x"}, nil, "")
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: want 401 without a session, got %d", path, w.Code)
		}
	}
}

func TestTelemetry_AcceptsSpanBatch(t *testing.T) {
	srv, _ := newAuthedServer(t)
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := postTelemetry(t, srv, "/api/v1/telemetry",
		map[string]any{"spans": []any{validSpan()}}, cookie, csrf)
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// Telemetry with no relay configured must still be accepted: the dashboard
// runs fine without SpanBarn, and a 4xx here would surface inside the very
// instrumentation that produced the request.
func TestTelemetry_AcceptedWithoutRelay(t *testing.T) {
	srv, _ := newAuthedServer(t)
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := postTelemetry(t, srv, "/api/v1/telemetry",
		map[string]any{"spans": []any{validSpan(), map[string]any{"traceId": "junk"}}}, cookie, csrf)
	if w.Code != http.StatusAccepted {
		t.Errorf("want 202, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestTelemetry_RejectsInvalidJSON(t *testing.T) {
	srv, _ := newAuthedServer(t)
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry", strings.NewReader(`{"spans":`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FunnelBarn-CSRF", csrf)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

// IDs are browser-supplied and end up in SpanBarn's index, so they are checked
// for shape rather than trusted.
func TestValidBrowserSpan(t *testing.T) {
	base := tracing.BrowserSpan{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7",
		Name: "page /x", StartTime: 1,
	}
	if !validBrowserSpan(base) {
		t.Fatal("a well-formed span should be valid")
	}

	withParent := base
	withParent.ParentSpanID = "0102030405060708"
	if !validBrowserSpan(withParent) {
		t.Error("a well-formed parent span ID should be valid")
	}

	cases := map[string]func(s *tracing.BrowserSpan){
		"short trace id":     func(s *tracing.BrowserSpan) { s.TraceID = "abcd" },
		"uppercase trace id": func(s *tracing.BrowserSpan) { s.TraceID = strings.ToUpper(s.TraceID) },
		"non-hex trace id":   func(s *tracing.BrowserSpan) { s.TraceID = strings.Repeat("z", 32) },
		"short span id":      func(s *tracing.BrowserSpan) { s.SpanID = "00f0" },
		"bad parent id":      func(s *tracing.BrowserSpan) { s.ParentSpanID = "nope" },
		"no name":            func(s *tracing.BrowserSpan) { s.Name = "" },
		"no start time":      func(s *tracing.BrowserSpan) { s.StartTime = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			sp := base
			mutate(&sp)
			if validBrowserSpan(sp) {
				t.Errorf("expected %+v to be rejected", sp)
			}
		})
	}
}

func TestClientError_RequiresMessage(t *testing.T) {
	srv, _ := newAuthedServer(t)
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := postTelemetry(t, srv, "/api/v1/client-errors",
		map[string]any{"message": "  ", "type": "TypeError"}, cookie, csrf)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422 for an empty message, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestClientError_Accepted(t *testing.T) {
	srv, _ := newAuthedServer(t)
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := postTelemetry(t, srv, "/api/v1/client-errors", map[string]any{
		"message":  "Cannot read properties of undefined",
		"type":     "TypeError",
		"stack":    "at Foo (bundle.js:1:2)",
		"url":      "https://funnelbarn.example.com/funnels",
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
	}, cookie, csrf)
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// A minified stack can be enormous, and this goes into a log line.
func TestTruncate(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("short strings pass through, got %q", got)
	}
	got := truncate(strings.Repeat("x", 20), 5)
	if len([]rune(got)) != 6 || !strings.HasSuffix(got, "…") {
		t.Errorf("truncate should cut and mark, got %q", got)
	}
}
