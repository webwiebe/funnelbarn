package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/iambarn"
)

// TestOIDCLoggedOut verifies the IAMBarn RP-initiated logout landing endpoint
// clears the local session (revoking it server-side and expiring the cookies)
// and redirects to /login.
func TestOIDCLoggedOut(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := sessionCookieFor(t, srv, "alice")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/logged-out", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location: got %q, want /login", loc)
	}

	// The session, CSRF and auth-method cookies must be expired.
	cleared := map[string]bool{}
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 || (c.MaxAge == 0 && c.Value == "") {
			cleared[c.Name] = true
		}
	}
	for _, name := range []string{"funnelbarn_session", "funnelbarn_auth_method"} {
		if !cleared[name] {
			t.Errorf("expected cookie %q to be cleared", name)
		}
	}

	// The revoked session must no longer validate.
	if _, ok := srv.sessionManager.Valid(cookie.Value); ok {
		t.Errorf("expected session to be revoked after logout")
	}
}

// TestSecurityCSP_StrictWithoutIAMBarn checks the default CSP is locked to
// 'self' when no IAMBarn issuer is configured.
func TestSecurityCSP_StrictWithoutIAMBarn(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self';") {
		t.Errorf("expected strict script-src, got %q", csp)
	}
	if strings.Contains(csp, "https://") {
		t.Errorf("expected no external origins in CSP, got %q", csp)
	}
}

// TestSecurityCSP_AllowsIAMBarnOrigin checks the issuer origin is whitelisted in
// script-src/connect-src/img-src when IAMBarn is configured at construction.
func TestSecurityCSP_AllowsIAMBarnOrigin(t *testing.T) {
	srv, _ := newTestServer(t)
	// Rebuild the security middleware as NewServer would, with an issuer set.
	srv.iambarnProvider = iambarn.New("https://iam.test.wiebe.xyz/", "ibc", "https://fb/cb")
	srv.securityMW = securityHeaders(originOf(srv.iambarnIssuer()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	origin := "https://iam.test.wiebe.xyz"
	for _, directive := range []string{"script-src 'self' " + origin, "connect-src 'self' " + origin, "img-src 'self' data: " + origin} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP missing %q; got %q", directive, csp)
		}
	}
}

func TestBuildCSP(t *testing.T) {
	strict := buildCSP("")
	if strings.Contains(strict, "https://") {
		t.Errorf("empty origin should yield strict CSP, got %q", strict)
	}
	withOrigin := buildCSP("https://iam.example")
	if !strings.Contains(withOrigin, "script-src 'self' https://iam.example;") {
		t.Errorf("expected origin in script-src, got %q", withOrigin)
	}
}

func TestOriginOf(t *testing.T) {
	cases := map[string]string{
		"https://iam.wiebe.xyz":         "https://iam.wiebe.xyz",
		"https://iam.wiebe.xyz/":        "https://iam.wiebe.xyz",
		"https://iam.wiebe.xyz/admin#p": "https://iam.wiebe.xyz",
		"":                              "",
		"not-a-url-with-no-scheme":      "",
	}
	for in, want := range cases {
		if got := originOf(in); got != want {
			t.Errorf("originOf(%q) = %q, want %q", in, got, want)
		}
	}
}
