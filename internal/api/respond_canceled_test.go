package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A client that closes the tab mid-request cancels the request context. The
// service error wraps context.Canceled; nobody is left to answer and nothing
// is wrong on our side, so it must not log at Error (FUN-67 to 71).
func TestMapServiceError_ClientDisconnectOpensNoIssue(t *testing.T) {
	capture := installCapture(t)
	w := httptest.NewRecorder()

	mapServiceError(w, fmt.Errorf("uniqueSessionCount: %w", context.Canceled), "handleDashboard.uniqueSessionCount")

	if w.Code != 499 {
		t.Errorf("status = %d, want 499 (client closed request)", w.Code)
	}
	for _, level := range levelsLogged(capture) {
		if level >= slog.LevelError {
			t.Fatalf("a client disconnect logged at %v; it would open a BugBarn issue", level)
		}
	}
}

// A deadline that expired on our side is a real slowness and stays an Error.
func TestMapServiceError_DeadlineExceededStaysAnError(t *testing.T) {
	capture := installCapture(t)
	w := httptest.NewRecorder()

	mapServiceError(w, fmt.Errorf("query: %w", context.DeadlineExceeded), "op")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	var sawError bool
	for _, level := range levelsLogged(capture) {
		sawError = sawError || level >= slog.LevelError
	}
	if !sawError {
		t.Error("a server-side deadline must still log at Error")
	}
}
