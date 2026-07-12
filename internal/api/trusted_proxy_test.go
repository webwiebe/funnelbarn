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

// ---------------------------------------------------------------------------
// isSecureRequest — trusted-proxy-aware X-Forwarded-Proto + env default
// ---------------------------------------------------------------------------

func secureReq(remote, xfp string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	if xfp != "" {
		req.Header.Set("X-Forwarded-Proto", xfp)
	}
	return req
}

func TestIsSecureRequest_TrustedProxyXFPHonored(t *testing.T) {
	srv := &Server{trustedProxies: []string{"10.0.0.1"}}
	if !srv.isSecureRequest(secureReq("10.0.0.1:1234", "https")) {
		t.Error("XFP=https from a trusted proxy must count as secure")
	}
}

func TestIsSecureRequest_UntrustedProxyXFPIgnored(t *testing.T) {
	// Dev-tier server with trusted proxies configured: a direct client
	// spoofing XFP must NOT flip the cookie to Secure (nor, more importantly,
	// be able to observe trusted-proxy-only behaviour).
	srv := &Server{trustedProxies: []string{"10.0.0.1"}, environment: "development"}
	if srv.isSecureRequest(secureReq("192.168.1.50:1234", "https")) {
		t.Error("XFP from an untrusted peer must be ignored in development")
	}
}

func TestIsSecureRequest_NoProxiesConfiguredBackwardsCompatible(t *testing.T) {
	srv := &Server{}
	if !srv.isSecureRequest(secureReq("10.0.0.1:1234", "https")) {
		t.Error("without configured proxies, XFP=https keeps working (backwards compatible)")
	}
	if srv.isSecureRequest(secureReq("10.0.0.1:1234", "")) {
		t.Error("plain request without XFP on an env-less server is not secure")
	}
}

func TestIsSecureRequest_DefaultsSecureOutsideDev(t *testing.T) {
	for _, env := range []string{"production", "staging", "test"} {
		srv := &Server{environment: env}
		if !srv.isSecureRequest(secureReq("10.0.0.1:1234", "")) {
			t.Errorf("%s: cookies must default to Secure even without XFP", env)
		}
	}
	for _, env := range []string{"", "development"} {
		srv := &Server{environment: env}
		if srv.isSecureRequest(secureReq("10.0.0.1:1234", "")) {
			t.Errorf("%q: plain http in dev must not be forced Secure", env)
		}
	}
}
