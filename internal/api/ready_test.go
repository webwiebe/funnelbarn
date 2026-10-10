package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A reader whose database is older than its binary must stay out of the
// Service until the writer has migrated, without failing its liveness probe.
func TestReady_GateFailsWithItsReason(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.ready = func(context.Context) error { return errors.New("schema version 38, want 39") }

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), "schema version 38") {
		t.Errorf("ready body %q does not carry the reason", w.Body.String())
	}

	w = httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200 while only readiness fails", w.Code)
	}
}

func TestReady_PassesThroughWhenTheGateHolds(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.ready = func(context.Context) error { return nil }

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want 200", w.Code)
	}
}
