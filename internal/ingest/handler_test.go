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
	"time"

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
	// The static env-var key is instance-wide and resolves with no project, so
	// the event has to name one itself.
	req.Header.Set("x-funnelbarn-project", "my-site")

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

// ---------------------------------------------------------------------------
// ServeHTTP — unattributable events
// ---------------------------------------------------------------------------

// An instance-wide key with no x-funnelbarn-project header leaves the event
// with no project. events.project_id is NOT NULL REFERENCES projects(id), so
// the worker could only ever fail its insert on the foreign key and
// dead-letter the record — long after the client saw a 202. Reject it here.
func TestServeHTTP_RejectsUnattributedEvent(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("mykey"), sp, 0)

	body := `{"name":"pageview","url":"https://example.com/page"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, "mykey")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an event with no resolvable project, got %d (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "x-funnelbarn-project") {
		t.Errorf("error message should name the fix, got %s", w.Body.String())
	}
}

// The header is only trusted when the key is not bound to a project; a
// project-scoped key attributes the event on its own.
func TestServeHTTP_ProjectScopedKeyNeedsNoHeader(t *testing.T) {
	sp := newTestSpool(t)
	lookup := func(_ context.Context, _ string) (string, string, bool, error) {
		return "proj-123", "ingest", true, nil
	}
	h := NewHandler(auth.New("static").WithDBLookup(lookup, nil), sp, 0)

	body := `{"name":"pageview"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, "project-key")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for a project-scoped key, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// The alert escalates to error level once per window and stays at warn in
// between, so one misconfigured client cannot flood the issue tracker.
func TestLogUnattributed_ThrottlesEscalation(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("mykey"), sp, 0)

	now := time.Unix(1_700_000_000, 0)
	h.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader("{}"))

	h.logUnattributed(req)
	first := h.lastUnattributedAt
	if first.IsZero() {
		t.Fatal("first occurrence should escalate and stamp the throttle")
	}

	now = now.Add(unattributedReAlert / 2)
	h.logUnattributed(req)
	if !h.lastUnattributedAt.Equal(first) {
		t.Error("must not re-escalate inside the re-alert window")
	}

	now = now.Add(unattributedReAlert)
	h.logUnattributed(req)
	if h.lastUnattributedAt.Equal(first) {
		t.Error("expected a fresh escalation after the re-alert window elapsed")
	}
}

// ---------------------------------------------------------------------------
// ServeHTTP — User-Agent capture
// ---------------------------------------------------------------------------

// A browser SDK posts over XHR and cannot put user_agent in the payload, so the
// header is the only source of browser/os/device_type. Carry it on the spool
// record for the worker to fall back to.
func TestServeHTTP_RecordsUserAgentHeader(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("mykey"), sp, 0)

	const ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/124.0"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(`{"name":"pageview"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, "mykey")
	req.Header.Set("x-funnelbarn-project", "my-site")
	req.Header.Set("User-Agent", ua)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (body: %s)", w.Code, w.Body.String())
	}

	select {
	case rec := <-h.queue:
		if rec.UserAgent != ua {
			t.Errorf("record UserAgent: want %q, got %q", ua, rec.UserAgent)
		}
	case <-time.After(time.Second):
		t.Fatal("no record enqueued")
	}
}

// No User-Agent header leaves the field empty rather than inventing one.
func TestServeHTTP_NoUserAgentHeaderLeavesFieldEmpty(t *testing.T) {
	sp := newTestSpool(t)
	h := NewHandler(auth.New("mykey"), sp, 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(`{"name":"pageview"}`))
	req.Header.Set(auth.HeaderAPIKey, "mykey")
	req.Header.Set("x-funnelbarn-project", "my-site")
	req.Header.Del("User-Agent")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (body: %s)", w.Code, w.Body.String())
	}

	select {
	case rec := <-h.queue:
		if rec.UserAgent != "" {
			t.Errorf("record UserAgent: want empty, got %q", rec.UserAgent)
		}
	case <-time.After(time.Second):
		t.Fatal("no record enqueued")
	}
}
