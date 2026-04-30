package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := newRateLimiter(5, time.Minute)
	for i := 0; i < 5; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		rl.allow("10.0.0.1")
	}
	if rl.allow("10.0.0.1") {
		t.Error("4th request should be blocked")
	}
}

func TestRateLimiter_IndependentPerIP(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	rl.allow("192.168.0.1")
	rl.allow("192.168.0.1")

	// Different IP should still be allowed.
	if !rl.allow("192.168.0.2") {
		t.Error("different IP should not be blocked")
	}
}

func TestRateLimiter_ResetsAfterWindow(t *testing.T) {
	rl := newRateLimiter(1, time.Millisecond)
	rl.allow("5.5.5.5")

	time.Sleep(5 * time.Millisecond)
	if !rl.allow("5.5.5.5") {
		t.Error("request should be allowed after window expires")
	}
}

func TestRateLimiter_WrapReturns429(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "1.1.1.1:12345"

	// First request allowed.
	w := httptest.NewRecorder()
	rl.wrap(inner).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("first request: want 200, got %d", w.Code)
	}

	// Second request blocked.
	w = httptest.NewRecorder()
	rl.wrap(inner).ServeHTTP(w, req)
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
