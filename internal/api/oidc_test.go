package api

import (
	"net/http"
	"testing"
)

// The test harness wires no IAMBarn provider / flag project, so the
// feature-flag gate reports disabled and both handlers return 404. This
// exercises the reachable (not-configured) path.

func TestHandleOIDCLogin_NotConfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	w := getJSON(t, srv, "/api/v1/auth/oidc/login", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleOIDCCallback_NotConfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	w := getJSON(t, srv, "/api/v1/auth/oidc/callback?state=x&code=y", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestParseStateCookie(t *testing.T) {
	tests := []struct {
		in           string
		state, verif string
		ok           bool
	}{
		{"abc|def", "abc", "def", true},
		{"nopipe", "", "", false},
		{"|def", "", "", false},
		{"abc|", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range tests {
		state, verif, ok := parseStateCookie(tc.in)
		if ok != tc.ok || state != tc.state || verif != tc.verif {
			t.Errorf("parseStateCookie(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tc.in, state, verif, ok, tc.state, tc.verif, tc.ok)
		}
	}
}

func TestGenerateOpaqueToken(t *testing.T) {
	a, err := generateOpaqueToken()
	if err != nil {
		t.Fatalf("generateOpaqueToken: %v", err)
	}
	b, _ := generateOpaqueToken()
	if a == "" || a == b {
		t.Errorf("expected two distinct non-empty tokens, got %q and %q", a, b)
	}
}
