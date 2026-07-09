package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

// oidcConfServer builds a server whose confidential OIDC client points at a
// local issuer that fails discovery fast (returns 404), so token exchange /
// authorize-url building error out deterministically without real network.
func oidcConfServer(t *testing.T) *Server {
	t.Helper()
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no discovery here", http.StatusNotFound)
	}))
	t.Cleanup(issuer.Close)
	client := auth.NewOIDCClient(auth.OIDCConfig{
		Issuer:       issuer.URL,
		ClientID:     "cid",
		ClientSecret: "secret",
		RedirectURL:  "http://localhost/api/v1/oidc/callback",
	})
	srv, _ := fullServer(t, func(cfg *ServerConfig) { cfg.OIDC = client })
	return srv
}

func TestHandleOIDCConfidentialLogin_NotConfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	w := getJSON(t, srv, "/api/v1/oidc/login", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleOIDCConfidentialCallback_NotConfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	w := getJSON(t, srv, "/api/v1/oidc/callback?state=x&code=y", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleOIDCConfidentialLogin_DiscoveryFails(t *testing.T) {
	srv := oidcConfServer(t)
	// AuthorizeURL -> ensureReady -> discovery fails -> 503.
	w := getJSON(t, srv, "/api/v1/oidc/login", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleOIDCConfidentialCallback_StateMismatch(t *testing.T) {
	srv := oidcConfServer(t)
	// No state cookie at all -> mismatch -> 400.
	w := getJSON(t, srv, "/api/v1/oidc/callback?state=abc&code=xyz", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for state mismatch, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleOIDCConfidentialCallback_MissingNonce(t *testing.T) {
	srv := oidcConfServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/oidc/callback?state=abc&code=xyz", nil)
	req.AddCookie(&http.Cookie{Name: oidcConfStateCookie, Value: "abc"})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing nonce, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleOIDCConfidentialCallback_MissingCode(t *testing.T) {
	srv := oidcConfServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/oidc/callback?state=abc", nil)
	req.AddCookie(&http.Cookie{Name: oidcConfStateCookie, Value: "abc"})
	req.AddCookie(&http.Cookie{Name: oidcConfNonceCookie, Value: "nnn"})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing code, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleOIDCConfidentialCallback_ExchangeFails(t *testing.T) {
	srv := oidcConfServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/oidc/callback?state=abc&code=xyz", nil)
	req.AddCookie(&http.Cookie{Name: oidcConfStateCookie, Value: "abc"})
	req.AddCookie(&http.Cookie{Name: oidcConfNonceCookie, Value: "nnn"})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	// Exchange -> ensureReady -> discovery fails -> 401.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for exchange failure, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestOIDCConfRandomToken(t *testing.T) {
	if a, b := oidcConfRandomToken(), oidcConfRandomToken(); a == "" || a == b {
		t.Errorf("expected distinct non-empty tokens, got %q and %q", a, b)
	}
}
