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

func TestClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.5:54321"

	ip := clientIP(req)
	if ip != "203.0.113.5" {
		t.Errorf("want 203.0.113.5, got %q", ip)
	}
}

func TestClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "10.10.10.1, 172.16.0.1")

	ip := clientIP(req)
	if ip != "10.10.10.1" {
		t.Errorf("want first IP from XFF, got %q", ip)
	}
}
