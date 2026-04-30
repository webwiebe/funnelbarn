package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/middleware"
)

func TestRequestID_GeneratesID(t *testing.T) {
	var capturedID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.FromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	middleware.RequestID(inner).ServeHTTP(w, req)

	if capturedID == "" {
		t.Error("expected non-empty request ID in context")
	}
	if w.Header().Get("X-Request-ID") != capturedID {
		t.Errorf("X-Request-ID header %q != context value %q", w.Header().Get("X-Request-ID"), capturedID)
	}
}

func TestRequestID_ReusesIncomingID(t *testing.T) {
	const incomingID = "test-correlation-id"
	var capturedID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.FromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", incomingID)
	w := httptest.NewRecorder()
	middleware.RequestID(inner).ServeHTTP(w, req)

	if capturedID != incomingID {
		t.Errorf("want %q, got %q", incomingID, capturedID)
	}
	if w.Header().Get("X-Request-ID") != incomingID {
		t.Errorf("response header should echo incoming ID")
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	ids := make([]string, 0, 5)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids = append(ids, middleware.FromContext(r.Context()))
	})
	h := middleware.RequestID(inner)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate request ID generated: %q", id)
		}
		seen[id] = true
	}
}

func TestFromContext_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if id := middleware.FromContext(req.Context()); id != "" {
		t.Errorf("expected empty string for context without request ID, got %q", id)
	}
}
