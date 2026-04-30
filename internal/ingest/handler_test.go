package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// newTestSpool creates a spool in a temp directory cleaned up after the test.
func newTestSpool(t *testing.T) *spool.Spool {
	t.Helper()
	dir, err := os.MkdirTemp("", "ingest-spool-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("spool.New: %v", err)
	}
	t.Cleanup(func() { sp.Close() })
	return sp
}

// ---------------------------------------------------------------------------
// NewHandler
// ---------------------------------------------------------------------------

func TestNewHandler_Defaults(t *testing.T) {
	sp := newTestSpool(t)
	a := auth.New("test-key")
	h := NewHandler(a, sp, 0) // 0 → default 1 MiB
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.maxBodyBytes != 1<<20 {
		t.Errorf("maxBodyBytes: want %d, got %d", 1<<20, h.maxBodyBytes)
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP — method not allowed
// ---------------------------------------------------------------------------

func TestServeHTTP_MethodNotAllowed(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("key"), sp, 0)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/v1/events", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", method, w.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP — unauthorized (wrong key)
// ---------------------------------------------------------------------------

func TestServeHTTP_Unauthorized(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("correct-key"), sp, 0)

	body := `{"name":"pageview","url":"https://example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, "wrong-key")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP — accepted
// ---------------------------------------------------------------------------

func TestServeHTTP_Accepted(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("mykey"), sp, 0)

	// Use a fixed idFn to get deterministic ingest ID.
	h.idFn = func() string { return "test-ingest-id-001" }

	body := `{"name":"pageview","url":"https://example.com/page"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, "mykey")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp["accepted"] != true {
		t.Errorf("expected accepted=true in response")
	}
	if resp["ingestId"] != "test-ingest-id-001" {
		t.Errorf("ingestId: want test-ingest-id-001, got %v", resp["ingestId"])
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP — body too large
// ---------------------------------------------------------------------------

func TestServeHTTP_BodyTooLarge(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("key"), sp, 10) // max 10 bytes

	bigBody := bytes.Repeat([]byte("x"), 100)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(bigBody))
	req.Header.Set(auth.HeaderAPIKey, "key")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP — nil handler
// ---------------------------------------------------------------------------

func TestServeHTTP_Nil(t *testing.T) {
	var h *Handler
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// ValidAPIKey
// ---------------------------------------------------------------------------

func TestValidAPIKey(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("secret-key"), sp, 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	req.Header.Set(auth.HeaderAPIKey, "secret-key")
	if !h.ValidAPIKey(req) {
		t.Error("expected ValidAPIKey=true for correct key")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	req2.Header.Set(auth.HeaderAPIKey, "bad-key")
	if h.ValidAPIKey(req2) {
		t.Error("expected ValidAPIKey=false for wrong key")
	}
}

// ---------------------------------------------------------------------------
// APIKeyProjectScope
// ---------------------------------------------------------------------------

func TestAPIKeyProjectScope_NoAuth(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(nil, sp, 0) // nil authorizer

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	_, scope, ok := h.APIKeyProjectScope(req)
	if !ok {
		t.Error("expected ok=true with nil authorizer")
	}
	if scope != "full" {
		t.Errorf("scope: want full, got %q", scope)
	}
}

// ---------------------------------------------------------------------------
// Start — drains queue
// ---------------------------------------------------------------------------

func TestStart_DrainOnCancel(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("key"), sp, 0)

	ctx, cancel := context.WithCancel(context.Background())

	// Enqueue some records before starting.
	for i := 0; i < 5; i++ {
		body := `{"name":"pageview"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
		req.Header.Set(auth.HeaderAPIKey, "key")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}

	done := make(chan struct{})
	go func() {
		h.Start(ctx)
		close(done)
	}()

	cancel()
	<-done // should complete promptly after cancel
}
