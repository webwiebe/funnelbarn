package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// newTestIssuer spins up a minimal OIDC issuer (discovery + JWKS + optional
// token/revocation endpoints) signing EdDSA tokens, mirroring iambarn, so the
// client can be exercised without a live issuer.
type testIssuer struct {
	srv  *httptest.Server
	priv ed25519.PrivateKey
	kid  string
}

func newTestIssuer(t *testing.T, tokenHandler http.HandlerFunc, revokeHandler http.HandlerFunc) *testIssuer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	it := &testIssuer{priv: priv, kid: "k1"}

	mux := http.NewServeMux()
	it.srv = httptest.NewServer(mux)
	t.Cleanup(it.srv.Close)
	issuer := it.srv.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/oauth2/authorize",
			"token_endpoint":                        issuer + "/oauth2/token",
			"jwks_uri":                              issuer + "/jwks",
			"revocation_endpoint":                   issuer + "/oauth2/revoke",
			"id_token_signing_alg_values_supported": []string{"EdDSA"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "OKP", "crv": "Ed25519", "kid": it.kid, "alg": "EdDSA", "use": "sig",
				"x": base64.RawURLEncoding.EncodeToString(pub),
			}},
		})
	})
	if tokenHandler != nil {
		mux.HandleFunc("/oauth2/token", tokenHandler)
	}
	if revokeHandler != nil {
		mux.HandleFunc("/oauth2/revoke", revokeHandler)
	}
	return it
}

// sign produces an EdDSA-signed compact JWT over claims.
func (it *testIssuer) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": it.kid})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	msg := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(it.priv, []byte(msg))
	return msg + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (it *testIssuer) client() *OIDCClient {
	return NewOIDCClient(OIDCConfig{
		Issuer:       it.srv.URL,
		ClientID:     "funnelbarn-web",
		ClientSecret: "sek",
		RedirectURL:  "https://funnelbarn.example.com/api/v1/oidc/callback",
	})
}

func TestAuthorizeURLRequestsOfflineAccessAndPKCE(t *testing.T) {
	it := newTestIssuer(t, nil, nil)
	oc := it.client()

	verifier := oauth2.GenerateVerifier()
	raw, err := oc.AuthorizeURL("state1", "nonce1", verifier)
	if err != nil {
		t.Fatalf("AuthorizeURL: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}
	scopes := strings.Fields(u.Query().Get("scope"))
	want := map[string]bool{"openid": false, "profile": false, "email": false, "offline_access": false}
	for _, s := range scopes {
		if _, ok := want[s]; ok {
			want[s] = true
		}
	}
	for scope, got := range want {
		if !got {
			t.Errorf("authorize URL scope %q missing from %q", scope, u.Query().Get("scope"))
		}
	}
	// PKCE is sent even though FunnelBarn is a confidential client.
	if got := u.Query().Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", got)
	}
	if got := u.Query().Get("code_challenge"); got == "" || got == verifier {
		t.Errorf("code_challenge = %q, want a non-empty S256 hash of the verifier", got)
	}
}

func TestOIDCClientRefreshSuccessRotatesAndParsesClaims(t *testing.T) {
	var it *testIssuer
	var gotForm url.Values
	it = newTestIssuer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    900,
			"id_token": it.sign(t, map[string]any{
				"iss": it.srv.URL, "aud": "funnelbarn-web", "sub": "u1",
				"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
				"sid": "sid-9", "groups": []string{"fresh-group"},
			}),
		})
	}, nil)
	oc := it.client()

	result, err := oc.Refresh(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.AccessToken != "new-access" || result.RefreshToken != "new-refresh" {
		t.Errorf("tokens = (%q, %q)", result.AccessToken, result.RefreshToken)
	}
	if result.ExpiresAt.Before(time.Now().Add(14 * time.Minute)) {
		t.Errorf("ExpiresAt = %v, want ~15 minutes out", result.ExpiresAt)
	}
	if gotForm.Get("grant_type") != "refresh_token" || gotForm.Get("refresh_token") != "old-refresh" {
		t.Errorf("grant sent: %v", gotForm)
	}
	// The fresh id_token's claims must be surfaced for re-snapshotting.
	if result.Claims == nil || result.IDToken == "" {
		t.Fatal("expected refreshed claims from id_token")
	}
	if result.Claims.SessionID != "sid-9" || len(result.Claims.Groups) != 1 || result.Claims.Groups[0] != "fresh-group" {
		t.Errorf("claims = %+v", result.Claims)
	}
}

func TestOIDCClientRefreshInvalidGrant(t *testing.T) {
	it := newTestIssuer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "invalid_grant",
			"error_description": "refresh token revoked or already used",
		})
	}, nil)
	oc := it.client()

	_, err := oc.Refresh(context.Background(), "dead-refresh")
	if !errors.Is(err, ErrRefreshInvalid) {
		t.Fatalf("Refresh error = %v, want ErrRefreshInvalid", err)
	}
}

func TestOIDCClientRefreshServerErrorIsNotInvalidGrant(t *testing.T) {
	it := newTestIssuer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server_error"}`))
	}, nil)
	oc := it.client()

	_, err := oc.Refresh(context.Background(), "old-refresh")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if errors.Is(err, ErrRefreshInvalid) {
		t.Error("a transient server_error must not be classified as ErrRefreshInvalid — the caller must not treat it as a dead token")
	}
}

func TestRevokeRefreshToken(t *testing.T) {
	var gotForm url.Values
	it := newTestIssuer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
	})
	oc := it.client()

	if err := oc.RevokeRefreshToken(context.Background(), "rt-dead"); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}
	if gotForm.Get("token") != "rt-dead" || gotForm.Get("token_type_hint") != "refresh_token" {
		t.Errorf("revocation form: %v", gotForm)
	}
	if gotForm.Get("client_id") != "funnelbarn-web" || gotForm.Get("client_secret") != "sek" {
		t.Error("revocation must be client-authenticated")
	}

	// Empty token is a silent no-op — nothing to revoke.
	gotForm = nil
	if err := oc.RevokeRefreshToken(context.Background(), ""); err != nil {
		t.Fatalf("RevokeRefreshToken(\"\"): %v", err)
	}
	if gotForm != nil {
		t.Error("empty refresh token must not hit the revocation endpoint")
	}
}

func TestEndSessionURL(t *testing.T) {
	it := newTestIssuer(t, nil, nil)
	oc := NewOIDCClient(OIDCConfig{
		Issuer:                it.srv.URL,
		ClientID:              "funnelbarn-web",
		ClientSecret:          "sek",
		RedirectURL:           "https://funnelbarn.example.com/api/v1/oidc/callback",
		PostLogoutRedirectURI: "https://funnelbarn.example.com/api/v1/auth/oidc/logged-out",
	})

	raw, err := oc.EndSessionURL("the-id-token")
	if err != nil {
		t.Fatalf("EndSessionURL: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// The minimal discovery doc omits end_session_endpoint, so the client
	// must fall back to {issuer}/oauth2/end-session.
	if u.Path != "/oauth2/end-session" {
		t.Errorf("path = %q, want /oauth2/end-session", u.Path)
	}
	q := u.Query()
	if q.Get("id_token_hint") != "the-id-token" || q.Get("client_id") != "funnelbarn-web" {
		t.Errorf("query = %v", q)
	}
	if q.Get("post_logout_redirect_uri") != "https://funnelbarn.example.com/api/v1/auth/oidc/logged-out" {
		t.Errorf("post_logout_redirect_uri = %q", q.Get("post_logout_redirect_uri"))
	}
}

// ---------------------------------------------------------------------------
// Back-channel logout token verification
// ---------------------------------------------------------------------------

func logoutClaims(it *testIssuer, extra map[string]any) map[string]any {
	claims := map[string]any{
		"iss": it.srv.URL,
		"aud": "funnelbarn-web",
		"sub": "user-1",
		"sid": "sid-1",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(2 * time.Minute).Unix(),
		"jti": "jti-1",
		"events": map[string]any{
			"http://schemas.openid.net/event/backchannel-logout": map[string]any{},
		},
	}
	for k, v := range extra {
		if v == nil {
			delete(claims, k)
			continue
		}
		claims[k] = v
	}
	return claims
}

func TestVerifyLogoutToken_Valid(t *testing.T) {
	it := newTestIssuer(t, nil, nil)
	oc := it.client()

	got, err := oc.VerifyLogoutToken(context.Background(), it.sign(t, logoutClaims(it, nil)))
	if err != nil {
		t.Fatalf("VerifyLogoutToken: %v", err)
	}
	if got.Subject != "user-1" || got.SessionID != "sid-1" {
		t.Errorf("claims = %+v", got)
	}

	// sid-only is valid too (spec allows either).
	got, err = oc.VerifyLogoutToken(context.Background(), it.sign(t, logoutClaims(it, map[string]any{"sub": nil})))
	if err != nil {
		t.Fatalf("VerifyLogoutToken sid-only: %v", err)
	}
	if got.SessionID != "sid-1" || got.Subject != "" {
		t.Errorf("sid-only claims = %+v", got)
	}
}

func TestVerifyLogoutToken_Rejections(t *testing.T) {
	it := newTestIssuer(t, nil, nil)
	oc := it.client()

	cases := map[string]map[string]any{
		"nonce present":       {"nonce": "n"},
		"missing events":      {"events": nil},
		"stale iat":           {"iat": time.Now().Add(-10 * time.Minute).Unix()},
		"future iat":          {"iat": time.Now().Add(10 * time.Minute).Unix()},
		"neither sub nor sid": {"sub": nil, "sid": nil},
		"wrong audience":      {"aud": "someone-else"},
		"wrong issuer":        {"iss": "https://evil.example"},
	}
	for name, extra := range cases {
		if _, err := oc.VerifyLogoutToken(context.Background(), it.sign(t, logoutClaims(it, extra))); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}

	// A token signed by a different key must fail signature verification.
	other := newTestIssuer(t, nil, nil)
	foreign := other.sign(t, logoutClaims(it, nil))
	if _, err := oc.VerifyLogoutToken(context.Background(), foreign); err == nil {
		t.Error("foreign-signed token: expected rejection")
	}
}
