package api

import (
	"net/http/httptest"
	"testing"
)

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
