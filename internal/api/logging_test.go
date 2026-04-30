package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// --------------------------------------------------------------------------
// captureHandler is a slog.Handler that records all log records in memory.
// --------------------------------------------------------------------------

type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
	attrs   []slog.Attr
	groups  []string
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := &captureHandler{records: h.records}
	clone.attrs = append(clone.attrs, h.attrs...)
	clone.attrs = append(clone.attrs, attrs...)
	return clone
}

func (h *captureHandler) WithGroup(name string) slog.Handler {
	clone := &captureHandler{records: h.records}
	clone.groups = append(clone.groups, h.groups...)
	clone.groups = append(clone.groups, name)
	return clone
}

// loggedText returns all logged messages and attributes concatenated.
func (h *captureHandler) loggedText() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var sb strings.Builder
	for _, r := range h.records {
		sb.WriteString(r.Message)
		sb.WriteString(" ")
		r.Attrs(func(a slog.Attr) bool {
			sb.WriteString(a.Key)
			sb.WriteString("=")
			sb.WriteString(a.Value.String())
			sb.WriteString(" ")
			return true
		})
	}
	return sb.String()
}

// --------------------------------------------------------------------------
// TestRequestLoggerDoesNotLogPassword verifies that password fields submitted
// in a login request body are never written to the log output.
// --------------------------------------------------------------------------

func TestRequestLoggerDoesNotLogPassword(t *testing.T) {
	// Install a capturing slog handler as the global logger.
	capture := &captureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(orig) })

	srv, _ := newTestServer(t)

	const secretPassword = "super-secret-password-12345"

	body, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": secretPassword,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	logged := capture.loggedText()
	if strings.Contains(logged, secretPassword) {
		t.Errorf("password found in log output: %q", logged)
	}
}

// TestRequestIDPropagation verifies that the request_id is present in the
// context after requestLogger runs and can be retrieved by handlers.
func TestRequestIDPropagation(t *testing.T) {
	var capturedID string

	// A simple handler that reads the request ID from context.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := requestLogger(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedID == "" {
		t.Error("expected request_id in context, got empty string")
	}
	if len(capturedID) != 16 {
		t.Errorf("expected 16-char hex request_id, got %q (len %d)", capturedID, len(capturedID))
	}
}
