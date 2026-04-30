package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	// 60 req/min, burst of 5 → first 5 requests should be allowed immediately.
	rl := newRateLimiter(60, 5)
	for i := 0; i < 5; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverBurst(t *testing.T) {
	// burst of 2 → 3rd request should be blocked.
	rl := newRateLimiter(60, 2)
	rl.allow("10.0.0.1")
	rl.allow("10.0.0.1")
	if rl.allow("10.0.0.1") {
		t.Error("3rd request should be blocked (over burst capacity)")
	}
}

func TestRateLimiter_IndependentPerIP(t *testing.T) {
	rl := newRateLimiter(60, 1)
	rl.allow("192.168.0.1") // consumes burst for this IP

	// Different IP should still be allowed.
	if !rl.allow("192.168.0.2") {
		t.Error("different IP should not be blocked")
	}
}

func TestRateLimiter_MiddlewareReturns429(t *testing.T) {
	// burst of 1 → second request blocked.
	rl := newRateLimiter(60, 1)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "1.1.1.1:12345"

	// First request allowed.
	w := httptest.NewRecorder()
	rl.middleware(inner).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("first request: want 200, got %d", w.Code)
	}

	// Second request blocked.
	w = httptest.NewRecorder()
	rl.middleware(inner).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request: want 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("blocked response should include Retry-After header")
	}
}

func TestClientIP_XForwardedFor(t *testing.T) {
	// Test that X-Forwarded-For is used when present
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	req.RemoteAddr = "127.0.0.1:9999"

	got := clientIP(req)
	if got != "1.2.3.4" {
		t.Errorf("X-Forwarded-For: want 1.2.3.4, got %q", got)
	}
}

func TestClientIP_RemoteAddr_Fallback(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:8080"

	got := clientIP(req)
	if got != "10.0.0.1" {
		t.Errorf("RemoteAddr fallback: want 10.0.0.1, got %q", got)
	}
}
