package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// levelsLogged returns the level of every record the handler captured.
func levelsLogged(h *captureHandler) []slog.Level {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Level, 0, len(h.records))
	for _, r := range h.records {
		out = append(out, r.Level)
	}
	return out
}

func installCapture(t *testing.T) *captureHandler {
	t.Helper()
	capture := &captureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return capture
}

// One database ping that outruns its budget (a node stall, FUN-59) answers 503
// and logs at Warn. Nothing reaches Error, so no issue opens; a stall that
// lasts is reported by the WorkloadUnavailable alerts.
func TestFailedPingThroughTheStackOpensNoIssue(t *testing.T) {
	capture := installCapture(t)
	srv, _ := newTestServer(t)
	srv.db = &ctxRecordingPinger{err: context.DeadlineExceeded}

	for _, path := range []string{"/api/v1/health", "/api/v1/ready"} {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s = %d, want 503", path, rr.Code)
		}
	}

	levels := levelsLogged(capture)
	if len(levels) == 0 {
		t.Fatal("nothing was logged for a failed ping")
	}
	for _, lvl := range levels {
		if lvl >= slog.LevelError {
			t.Errorf("logged at %v, want Warn or below so no issue opens: %s", lvl, capture.loggedText())
		}
	}
}

// The readiness gate and a failed ping are not the only 5xx a probe can see.
// Any 5xx from a probe route is the pod saying it is not ready.
func TestProbeRoutes5xxIsWarnNotError(t *testing.T) {
	for _, path := range []string{"/api/v1/health", "/api/v1/ready"} {
		capture := installCapture(t)
		h := requestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))

		for _, lvl := range levelsLogged(capture) {
			if lvl >= slog.LevelError {
				t.Errorf("%s: 503 logged at %v, want Warn", path, lvl)
			}
		}
	}
}

// Every other route keeps its Error envelope, so a real 5xx still opens an issue.
func TestOtherRoutes5xxStillLogsError(t *testing.T) {
	capture := installCapture(t)
	h := requestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))

	found := false
	for _, lvl := range levelsLogged(capture) {
		if lvl >= slog.LevelError {
			found = true
		}
	}
	if !found {
		t.Errorf("a 500 on /api/v1/events logged no Error record: %s", capture.loggedText())
	}
}

// A ping that fails for a reason other than the deadline is still answered 503.
func TestFailedPingStillAnswers503(t *testing.T) {
	installCapture(t)
	srv, _ := newTestServer(t)
	srv.db = &ctxRecordingPinger{err: errors.New("connection refused")}

	rr := httptest.NewRecorder()
	srv.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}
