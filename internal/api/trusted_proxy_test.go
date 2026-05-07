package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP_TrustedProxy_UsesXFF(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	srv := &Server{trustedProxies: []string{"10.0.0.1"}}
	got := srv.clientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("trusted proxy: want 203.0.113.50, got %q", got)
	}
}

func TestClientIP_UntrustedProxy_IgnoresXFF(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.100:1234"
	req.Header.Set("X-Forwarded-For", "spoofed-ip")

	srv := &Server{trustedProxies: []string{"10.0.0.1"}}
	got := srv.clientIP(req)
	if got != "192.168.1.100" {
		t.Errorf("untrusted proxy: want 192.168.1.100, got %q", got)
	}
}

func TestClientIP_NoTrustedProxiesConfigured_UsesXFF(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	srv := &Server{}
	got := srv.clientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("no proxies configured (backwards compat): want 203.0.113.50, got %q", got)
	}
}

func TestClientIP_NoXFF_UsesRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.100:1234"

	srv := &Server{}
	got := srv.clientIP(req)
	if got != "192.168.1.100" {
		t.Errorf("no XFF: want 192.168.1.100, got %q", got)
	}
}
