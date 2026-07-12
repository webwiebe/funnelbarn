package api

// Fake-IdP harness driving the full OIDC relying-party lifecycle end-to-end:
// login redirect (PKCE) → callback (token exchange, session row) → middleware
// (access-token expiry → refresh_token grant → claims re-snapshot) →
// invalid_grant revocation → transient-outage grace → back-channel logout →
// server-driven logout (revocation + end-session URL).

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

const (
	fakeIdPClientID = "fb_conf"
	fakeIdPSub      = "user-sub-1"
	fakeIdPSid      = "idp-sid-1"
)

// fakeIdP is a minimal in-process OIDC issuer: discovery, an ed25519 JWKS,
// a scriptable token endpoint, and a recording revocation endpoint.
type fakeIdP struct {
	srv  *httptest.Server
	priv ed25519.PrivateKey
	kid  string

	mu           sync.Mutex
	tokenHandler http.HandlerFunc
	revokedForms []url.Values
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	f := &fakeIdP{priv: priv, kid: "test-key"}

	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	issuer := f.srv.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/oauth2/authorize",
			"token_endpoint":                        issuer + "/oauth2/token",
			"jwks_uri":                              issuer + "/jwks",
			"revocation_endpoint":                   issuer + "/oauth2/revoke",
			"end_session_endpoint":                  issuer + "/oauth2/end-session",
			"id_token_signing_alg_values_supported": []string{"EdDSA"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "OKP",
				"crv": "Ed25519",
				"kid": f.kid,
				"alg": "EdDSA",
				"use": "sig",
				"x":   base64.RawURLEncoding.EncodeToString(pub),
			}},
		})
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		h := f.tokenHandler
		f.mu.Unlock()
		if h == nil {
			http.Error(w, "no token handler scripted", http.StatusInternalServerError)
			return
		}
		h(w, r)
	})
	mux.HandleFunc("/oauth2/revoke", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.revokedForms = append(f.revokedForms, r.PostForm)
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	return f
}

func (f *fakeIdP) setTokenHandler(h http.HandlerFunc) {
	f.mu.Lock()
	f.tokenHandler = h
	f.mu.Unlock()
}

func (f *fakeIdP) revocations() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.revokedForms...)
}

// sign produces an EdDSA-signed compact JWT over claims.
func (f *fakeIdP) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": f.kid})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	msg := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(f.priv, []byte(msg))
	return msg + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// idToken builds a standard ID token for the fake user; extra overrides/adds claims.
func (f *fakeIdP) idToken(t *testing.T, nonce string, extra map[string]any) string {
	t.Helper()
	claims := map[string]any{
		"iss":                f.srv.URL,
		"sub":                fakeIdPSub,
		"aud":                fakeIdPClientID,
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
		"sid":                fakeIdPSid,
		"preferred_username": "alice",
		"groups":             []string{"funnelbarn-users"},
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	for k, v := range extra {
		claims[k] = v
	}
	return f.sign(t, claims)
}

// logoutToken builds a back-channel logout token; extra overrides/adds claims.
func (f *fakeIdP) logoutToken(t *testing.T, extra map[string]any) string {
	t.Helper()
	claims := map[string]any{
		"iss": f.srv.URL,
		"aud": fakeIdPClientID,
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
	return f.sign(t, claims)
}

// tokenResponse writes a standard token-endpoint JSON response.
func tokenResponse(w http.ResponseWriter, access, refresh, idToken string, expiresIn int) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"access_token": access,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
	}
	if refresh != "" {
		resp["refresh_token"] = refresh
	}
	if idToken != "" {
		resp["id_token"] = idToken
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// oidcFlowServer wires a full API server against the fake IdP.
func oidcFlowServer(t *testing.T, f *fakeIdP, mutate func(cfg *ServerConfig)) (*Server, *repository.Store) {
	t.Helper()
	return fullServer(t, func(cfg *ServerConfig) {
		cfg.OIDC = auth.NewOIDCClient(auth.OIDCConfig{
			Issuer:                f.srv.URL,
			ClientID:              fakeIdPClientID,
			ClientSecret:          "sek",
			RedirectURL:           "http://funnelbarn.test/api/v1/oidc/callback",
			PostLogoutRedirectURI: "http://funnelbarn.test/api/v1/auth/oidc/logged-out",
		})
		if mutate != nil {
			mutate(cfg)
		}
	})
}

// insertOIDCSession seeds a web_sessions row directly, as the callback would.
func insertOIDCSession(t *testing.T, srv *Server, ws repository.WebSession) (*http.Cookie, string) {
	t.Helper()
	token, expires, err := srv.sessionManager.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ws.IDHash = auth.HashSessionToken(token)
	if ws.AuthMethod == "" {
		ws.AuthMethod = "oidc"
	}
	if ws.CreatedAt == 0 {
		ws.CreatedAt = time.Now().Unix()
	}
	if ws.AbsoluteExpiresAt == 0 {
		ws.AbsoluteExpiresAt = expires.Unix()
	}
	if err := srv.webSessions.CreateWebSession(context.Background(), ws); err != nil {
		t.Fatalf("CreateWebSession: %v", err)
	}
	return auth.SessionCookie(token, expires, false), ws.IDHash
}

// ---------------------------------------------------------------------------
// Login redirect + full callback flow
// ---------------------------------------------------------------------------

func TestOIDCLogin_RedirectCarriesPKCEAndOfflineAccess(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := oidcFlowServer(t, f, nil)

	w := getJSON(t, srv, "/api/v1/oidc/login", nil)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d (body: %s)", w.Code, w.Body.String())
	}
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}
	q := loc.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}
	if q.Get("code_challenge") == "" {
		t.Error("authorize URL missing code_challenge")
	}
	if !strings.Contains(q.Get("scope"), "offline_access") {
		t.Errorf("scope %q missing offline_access", q.Get("scope"))
	}
	// The state cookie stores "state|verifier"; the challenge on the URL must
	// be the S256 hash of that verifier.
	var stateCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == oidcConfStateCookie {
			stateCookie = c
		}
	}
	if stateCookie == nil {
		t.Fatal("state cookie not set")
	}
	state, verifier, ok := parseStateCookie(stateCookie.Value)
	if !ok {
		t.Fatalf("malformed state cookie %q", stateCookie.Value)
	}
	if state != q.Get("state") {
		t.Error("state cookie does not match authorize URL state")
	}
	sum := sha256.Sum256([]byte(verifier))
	if got := base64.RawURLEncoding.EncodeToString(sum[:]); got != q.Get("code_challenge") {
		t.Error("code_challenge is not the S256 hash of the cookie verifier")
	}
}

func TestOIDCFullFlow_CallbackPersistsTokenBoundSession(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, func(cfg *ServerConfig) {
		cfg.IAMBarnUsers = cfg.WebSessions.(*repository.Store)
	})

	// 1. Login: capture state/nonce cookies + authorize params.
	w := getJSON(t, srv, "/api/v1/oidc/login", nil)
	if w.Code != http.StatusFound {
		t.Fatalf("login: expected 302, got %d", w.Code)
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	authQ := loc.Query()
	cookies := w.Result().Cookies()
	var state, nonce, verifier string
	for _, c := range cookies {
		switch c.Name {
		case oidcConfStateCookie:
			state, verifier, _ = parseStateCookie(c.Value)
		case oidcConfNonceCookie:
			nonce = c.Value
		}
	}

	// 2. Script the token endpoint: assert PKCE verifier arrives, return the
	// full token set with a signed ID token bound to the nonce.
	var gotVerifier string
	f.setTokenHandler(func(tw http.ResponseWriter, tr *http.Request) {
		_ = tr.ParseForm()
		gotVerifier = tr.PostFormValue("code_verifier")
		tokenResponse(tw, "at-1", "rt-1", f.idToken(t, nonce, nil), 900)
	})

	// 3. Callback with the state + cookies.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/oidc/callback?state="+url.QueryEscape(state)+"&code=code-1", nil)
	for _, c := range cookies {
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback: expected 302, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/dashboard" {
		t.Errorf("callback redirect: got %q, want /dashboard", loc)
	}
	if gotVerifier != verifier {
		t.Errorf("token endpoint got code_verifier %q, want the cookie verifier %q", gotVerifier, verifier)
	}
	if authQ.Get("client_id") != fakeIdPClientID {
		t.Errorf("authorize client_id = %q", authQ.Get("client_id"))
	}

	// 4. The session cookie is an opaque handle whose row carries the tokens.
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "funnelbarn_session" && c.Value != "" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session cookie issued")
	}
	if strings.Contains(session.Value, ".") {
		t.Errorf("session cookie must be an opaque handle, got %q", session.Value)
	}
	ws, err := store.GetWebSession(context.Background(), auth.HashSessionToken(session.Value))
	if err != nil {
		t.Fatalf("session row not found: %v", err)
	}
	if ws.AuthMethod != "oidc" || ws.AccessToken != "at-1" || ws.RefreshToken != "rt-1" ||
		ws.IdpSub != fakeIdPSub || ws.IdpSid != fakeIdPSid || ws.IDToken == "" {
		t.Errorf("session row incomplete: %+v", ws)
	}
	if ws.AccessExpiresAt == 0 {
		t.Error("access_expires_at not set")
	}
	if !strings.Contains(ws.ClaimsJSON, "funnelbarn-users") {
		t.Errorf("claims snapshot missing groups: %q", ws.ClaimsJSON)
	}

	// 5. The user record was upserted keyed on sub.
	if u, err := store.FindUserByIAMBarnSub(context.Background(), fakeIdPSub); err != nil || u.Username != "alice" {
		t.Errorf("user upsert: got (%+v, %v)", u, err)
	}

	// 6. The session authenticates /api/v1/me.
	me := getJSON(t, srv, "/api/v1/me", session)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), "alice") {
		t.Errorf("/me with fresh session: got %d %s", me.Code, me.Body.String())
	}
}

func TestOIDCCallback_GroupDenied(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := oidcFlowServer(t, f, nil)

	w := getJSON(t, srv, "/api/v1/oidc/login", nil)
	loc, _ := url.Parse(w.Header().Get("Location"))
	cookies := w.Result().Cookies()
	var nonce string
	for _, c := range cookies {
		if c.Name == oidcConfNonceCookie {
			nonce = c.Value
		}
	}
	f.setTokenHandler(func(tw http.ResponseWriter, _ *http.Request) {
		tokenResponse(tw, "at", "rt", f.idToken(t, nonce, map[string]any{"groups": []string{"other"}}), 900)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/oidc/callback?state="+url.QueryEscape(loc.Query().Get("state"))+"&code=c", nil)
	for _, c := range cookies {
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong group, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Middleware: expiry → refresh → claims re-snapshot
// ---------------------------------------------------------------------------

func TestSessionRefresh_ExpiredAccessTokenRotatesAndResnapshotsClaims(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)

	cookie, idHash := insertOIDCSession(t, srv, repository.WebSession{
		Username:        "alice",
		IdpSub:          fakeIdPSub,
		IdpSid:          fakeIdPSid,
		AccessToken:     "at-old",
		RefreshToken:    "rt-old",
		AccessExpiresAt: time.Now().Add(-time.Minute).Unix(),
		ClaimsJSON:      `{"groups":["funnelbarn-users"]}`,
	})

	var gotGrant, gotRefresh string
	f.setTokenHandler(func(tw http.ResponseWriter, tr *http.Request) {
		_ = tr.ParseForm()
		gotGrant = tr.PostFormValue("grant_type")
		gotRefresh = tr.PostFormValue("refresh_token")
		// Refresh responses carry a fresh id_token (no nonce) with updated
		// claims — the middleware must re-snapshot them.
		tokenResponse(tw, "at-new", "rt-new", f.idToken(t, "", map[string]any{
			"groups": []string{"funnelbarn-users", "new-central-group"},
		}), 900)
	})

	w := getJSON(t, srv, "/api/v1/me", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after refresh, got %d (body: %s)", w.Code, w.Body.String())
	}
	if gotGrant != "refresh_token" || gotRefresh != "rt-old" {
		t.Errorf("refresh grant: got (%q, %q)", gotGrant, gotRefresh)
	}
	ws, err := store.GetWebSession(context.Background(), idHash)
	if err != nil {
		t.Fatalf("row gone after refresh: %v", err)
	}
	if ws.AccessToken != "at-new" || ws.RefreshToken != "rt-new" {
		t.Errorf("tokens not rotated: %+v", ws)
	}
	if !strings.Contains(ws.ClaimsJSON, "new-central-group") {
		t.Errorf("claims not re-snapshotted: %q", ws.ClaimsJSON)
	}
	if ws.LastRefreshAt == 0 || ws.RefreshFailingSince != 0 {
		t.Errorf("refresh bookkeeping wrong: last=%d failing=%d", ws.LastRefreshAt, ws.RefreshFailingSince)
	}
}

func TestSessionRefresh_InvalidGrantKillsSession(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)

	cookie, idHash := insertOIDCSession(t, srv, repository.WebSession{
		Username:        "alice",
		IdpSub:          fakeIdPSub,
		RefreshToken:    "rt-dead",
		AccessExpiresAt: time.Now().Add(-time.Minute).Unix(),
	})
	f.setTokenHandler(func(tw http.ResponseWriter, _ *http.Request) {
		tw.Header().Set("Content-Type", "application/json")
		tw.WriteHeader(http.StatusBadRequest)
		_, _ = tw.Write([]byte(`{"error":"invalid_grant"}`))
	})

	w := getJSON(t, srv, "/api/v1/me", cookie)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on invalid_grant, got %d", w.Code)
	}
	if _, err := store.GetWebSession(context.Background(), idHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected row deleted on invalid_grant, got err=%v", err)
	}
}

func TestSessionRefresh_TransientFailureServesStaleWithinGrace(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)

	cookie, idHash := insertOIDCSession(t, srv, repository.WebSession{
		Username:        "alice",
		IdpSub:          fakeIdPSub,
		RefreshToken:    "rt-1",
		AccessExpiresAt: time.Now().Add(-time.Minute).Unix(),
	})
	f.setTokenHandler(func(tw http.ResponseWriter, _ *http.Request) {
		http.Error(tw, `{"error":"server_error"}`, http.StatusInternalServerError)
	})

	// First failing refresh: still served (stale), failure stamped.
	w := getJSON(t, srv, "/api/v1/me", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected stale-serve 200 within grace, got %d (body: %s)", w.Code, w.Body.String())
	}
	ws, err := store.GetWebSession(context.Background(), idHash)
	if err != nil {
		t.Fatalf("row must survive transient failure: %v", err)
	}
	if ws.RefreshFailingSince == 0 {
		t.Error("refresh_failing_since not stamped on first transient failure")
	}
}

func TestSessionRefresh_GraceExceededCutsOff(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)

	// Failing since 2h ago with the default 1h grace → cut off.
	cookie, idHash := insertOIDCSession(t, srv, repository.WebSession{
		Username:            "alice",
		IdpSub:              fakeIdPSub,
		RefreshToken:        "rt-1",
		AccessExpiresAt:     time.Now().Add(-2 * time.Hour).Unix(),
		RefreshFailingSince: time.Now().Add(-2 * time.Hour).Unix(),
	})
	f.setTokenHandler(func(tw http.ResponseWriter, _ *http.Request) {
		http.Error(tw, `{"error":"server_error"}`, http.StatusInternalServerError)
	})

	w := getJSON(t, srv, "/api/v1/me", cookie)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 past grace ceiling, got %d", w.Code)
	}
	if _, err := store.GetWebSession(context.Background(), idHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected row deleted past grace, got err=%v", err)
	}
}

func TestSession_AbsoluteExpiryEnforced(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)

	cookie, idHash := insertOIDCSession(t, srv, repository.WebSession{
		Username:          "alice",
		IdpSub:            fakeIdPSub,
		AccessToken:       "at",
		RefreshToken:      "rt",
		AccessExpiresAt:   time.Now().Add(10 * time.Minute).Unix(),
		AbsoluteExpiresAt: time.Now().Add(-time.Minute).Unix(), // past hard cap
	})

	w := getJSON(t, srv, "/api/v1/me", cookie)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 past absolute cap even with a live access token, got %d", w.Code)
	}
	if _, err := store.GetWebSession(context.Background(), idHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected expired row pruned, got err=%v", err)
	}
}

// ---------------------------------------------------------------------------
// Back-channel logout
// ---------------------------------------------------------------------------

func postBackchannel(t *testing.T, srv *Server, logoutToken string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{}
	if logoutToken != "" {
		form.Set("logout_token", logoutToken)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oidc/backchannel-logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestBackchannelLogout_BySid(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)

	_, idHash := insertOIDCSession(t, srv, repository.WebSession{
		Username: "alice", IdpSub: fakeIdPSub, IdpSid: fakeIdPSid,
		AccessToken: "at", RefreshToken: "rt",
		AccessExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	})
	// A session for another sid must survive.
	_, otherHash := insertOIDCSession(t, srv, repository.WebSession{
		Username: "bob", IdpSub: "other-sub", IdpSid: "other-sid",
		AccessToken: "at2", RefreshToken: "rt2",
		AccessExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	})

	w := postBackchannel(t, srv, f.logoutToken(t, map[string]any{"sub": fakeIdPSub, "sid": fakeIdPSid}))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("back-channel response must be Cache-Control: no-store")
	}
	if _, err := store.GetWebSession(context.Background(), idHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("sid-targeted session must be deleted, got err=%v", err)
	}
	if _, err := store.GetWebSession(context.Background(), otherHash); err != nil {
		t.Errorf("unrelated session must survive, got err=%v", err)
	}
}

func TestBackchannelLogout_BySubWhenNoSid(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)

	// Two sessions for the same subject, different sids — both must die.
	_, h1 := insertOIDCSession(t, srv, repository.WebSession{
		Username: "alice", IdpSub: fakeIdPSub, IdpSid: "sid-a",
		AccessExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	})
	_, h2 := insertOIDCSession(t, srv, repository.WebSession{
		Username: "alice", IdpSub: fakeIdPSub, IdpSid: "sid-b",
		AccessExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	})

	w := postBackchannel(t, srv, f.logoutToken(t, map[string]any{"sub": fakeIdPSub}))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	for _, h := range []string{h1, h2} {
		if _, err := store.GetWebSession(context.Background(), h); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("sub-targeted session %s must be deleted, got err=%v", h, err)
		}
	}
}

func TestBackchannelLogout_RejectsBadTokens(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := oidcFlowServer(t, f, nil)

	cases := map[string]string{
		"missing token": "",
		"with nonce (spec forbids)": f.logoutToken(t, map[string]any{
			"sub": fakeIdPSub, "nonce": "nope",
		}),
		"missing events claim": f.logoutToken(t, map[string]any{
			"sub": fakeIdPSub, "events": nil,
		}),
		"stale iat": f.logoutToken(t, map[string]any{
			"sub": fakeIdPSub, "iat": time.Now().Add(-10 * time.Minute).Unix(),
		}),
		"neither sub nor sid": f.logoutToken(t, nil),
		"garbage":             "not.a.jwt",
	}
	for name, tok := range cases {
		if w := postBackchannel(t, srv, tok); w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d (body: %s)", name, w.Code, w.Body.String())
		}
	}
}

func TestBackchannelLogout_NotConfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	w := postBackchannel(t, srv, "whatever")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without OIDC, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Server-driven logout
// ---------------------------------------------------------------------------

func TestLogout_OIDCSessionRevokesAndReturnsEndSessionURL(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)

	idToken := f.idToken(t, "", nil)
	cookie, idHash := insertOIDCSession(t, srv, repository.WebSession{
		Username: "alice", IdpSub: fakeIdPSub, IdpSid: fakeIdPSid,
		IDToken: idToken, AccessToken: "at", RefreshToken: "rt-logout",
		AccessExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	logoutURL := resp["logout_url"]
	if !strings.Contains(logoutURL, "/oauth2/end-session") {
		t.Errorf("logout_url missing end-session endpoint: %q", logoutURL)
	}
	u, err := url.Parse(logoutURL)
	if err != nil {
		t.Fatalf("parse logout_url: %v", err)
	}
	if u.Query().Get("id_token_hint") != idToken {
		t.Error("logout_url missing id_token_hint")
	}
	if u.Query().Get("client_id") != fakeIdPClientID {
		t.Error("logout_url missing client_id")
	}
	if u.Query().Get("post_logout_redirect_uri") == "" {
		t.Error("logout_url missing post_logout_redirect_uri")
	}

	// The refresh token was revoked at the issuer (best-effort but here it works).
	revs := f.revocations()
	if len(revs) != 1 || revs[0].Get("token") != "rt-logout" || revs[0].Get("token_type_hint") != "refresh_token" {
		t.Errorf("revocation endpoint calls: %+v", revs)
	}

	// The row is gone — the handle is dead.
	if _, err := store.GetWebSession(context.Background(), idHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected session row deleted on logout, got err=%v", err)
	}
}

func TestLogout_LocalSessionHasNoLogoutURL(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)
	cookie, idHash := sessionWithRow(t, srv, "localadmin")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "logout_url") {
		t.Errorf("local session logout must not carry a logout_url: %s", w.Body.String())
	}
	if _, err := store.GetWebSession(context.Background(), idHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected session row deleted, got err=%v", err)
	}
	if len(f.revocations()) != 0 {
		t.Error("local session logout must not hit the revocation endpoint")
	}
}

// TestLocalLogin_CreatesSessionRow verifies password login also produces a
// server-side row (auth_method=local, NULL tokens): one middleware, one
// revocation story.
func TestLocalLogin_CreatesSessionRow(t *testing.T) {
	srv, store := newAuthedServer(t)

	w := postJSON(t, srv, "/api/v1/login", map[string]string{
		"username": "admin", "password": "password",
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "funnelbarn_session" && c.Value != "" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session cookie issued")
	}
	ws, err := store.GetWebSession(context.Background(), auth.HashSessionToken(session.Value))
	if err != nil {
		t.Fatalf("session row not found: %v", err)
	}
	if ws.AuthMethod != "local" || ws.Username != "admin" {
		t.Errorf("row: %+v", ws)
	}
	if ws.AccessToken != "" || ws.RefreshToken != "" || ws.IDToken != "" {
		t.Errorf("local session must not carry tokens: %+v", ws)
	}

	// And the cookie authenticates until the row is deleted.
	if me := getJSON(t, srv, "/api/v1/me", session); me.Code != http.StatusOK {
		t.Fatalf("/me: expected 200, got %d", me.Code)
	}
	if err := store.DeleteWebSession(context.Background(), ws.IDHash); err != nil {
		t.Fatal(err)
	}
	if me := getJSON(t, srv, "/api/v1/me", session); me.Code != http.StatusUnauthorized {
		t.Errorf("/me after row deletion: expected 401, got %d", me.Code)
	}
}
