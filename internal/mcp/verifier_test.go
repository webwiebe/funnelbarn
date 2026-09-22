package mcp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	fbauth "github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

const (
	testResourceURL = "https://funnelbarn.test/api/v1/mcp"
	testClientID    = "fb_dashboard"
	testSub         = "sub-alice"
)

// fakeIssuer is an in-process IAMBarn: discovery, an Ed25519 JWKS and a
// userinfo endpoint that counts its calls.
type fakeIssuer struct {
	srv  *httptest.Server
	priv ed25519.PrivateKey
	kid  string

	userinfo      atomic.Value // map[string]any served by /userinfo
	userinfoCalls atomic.Int32
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	f := &fakeIssuer{priv: priv, kid: "k1"}
	f.userinfo.Store(map[string]any{"sub": testSub})

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
			"userinfo_endpoint":                     issuer + "/oauth2/userinfo",
			"jwks_uri":                              issuer + "/oauth2/jwks.json",
			"id_token_signing_alg_values_supported": []string{"EdDSA"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/oauth2/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "OKP", "crv": "Ed25519", "kid": f.kid, "alg": "EdDSA", "use": "sig",
			"x": base64.RawURLEncoding.EncodeToString(pub),
		}}})
	})
	mux.HandleFunc("/oauth2/userinfo", func(w http.ResponseWriter, r *http.Request) {
		f.userinfoCalls.Add(1)
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.userinfo.Load())
	})
	return f
}

// signWith produces an EdDSA-signed compact JWT with the given key.
func signWith(t *testing.T, priv ed25519.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "at+jwt", "kid": kid})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	msg := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	return msg + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(msg)))
}

// accessToken builds an MCP access token; extra overrides claims, and a nil
// value removes one.
func (f *fakeIssuer) accessToken(t *testing.T, extra map[string]any) string {
	t.Helper()
	claims := map[string]any{
		"iss":                f.srv.URL,
		"sub":                testSub,
		"aud":                []string{testResourceURL},
		"exp":                time.Now().Add(15 * time.Minute).Unix(),
		"iat":                time.Now().Unix(),
		"scope":              "mcp:read mcp:write",
		"token_use":          "access_token",
		"client_id":          "dcr-client-1",
		"preferred_username": "alice",
		"groups":             []string{"funnelbarn-users"},
		"roles":              []string{},
	}
	for k, v := range extra {
		if v == nil {
			delete(claims, k)
			continue
		}
		claims[k] = v
	}
	return signWith(t, f.priv, f.kid, claims)
}

func (f *fakeIssuer) client() *fbauth.OIDCClient {
	return fbauth.NewOIDCClient(fbauth.OIDCConfig{
		Issuer:       f.srv.URL + "/",
		ClientID:     testClientID,
		ClientSecret: "sek",
		RedirectURL:  "https://funnelbarn.test/api/v1/oidc/callback",
	})
}

func openStore(t *testing.T) *repository.Store {
	t.Helper()
	store, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestTokenVerifier_Rejects(t *testing.T) {
	f := newFakeIssuer(t)
	_, otherKey, _ := ed25519.GenerateKey(rand.Reader)

	cases := []struct {
		name  string
		token func() string
	}{
		{"expired", func() string {
			return f.accessToken(t, map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})
		}},
		{"missing exp", func() string { return f.accessToken(t, map[string]any{"exp": nil}) }},
		{"wrong issuer", func() string { return f.accessToken(t, map[string]any{"iss": "https://evil.example"}) }},
		{"dashboard client audience", func() string {
			return f.accessToken(t, map[string]any{"aud": []string{testClientID}})
		}},
		{"audience without the resource", func() string {
			return f.accessToken(t, map[string]any{"aud": []string{"https://funnelbarn.test/api/v1/mcp/other", testClientID}})
		}},
		{"audience with trailing slash", func() string {
			return f.accessToken(t, map[string]any{"aud": []string{testResourceURL + "/"}})
		}},
		{"id token", func() string { return f.accessToken(t, map[string]any{"token_use": "id_token"}) }},
		{"no token_use", func() string { return f.accessToken(t, map[string]any{"token_use": nil}) }},
		{"no subject", func() string { return f.accessToken(t, map[string]any{"sub": nil}) }},
		{"outside required group", func() string {
			return f.accessToken(t, map[string]any{"groups": []string{"other"}, "roles": []string{"member"}})
		}},
		{"signed by another key", func() string {
			return signWith(t, otherKey, f.kid, map[string]any{
				"iss": f.srv.URL, "sub": testSub, "aud": []string{testResourceURL},
				"exp": time.Now().Add(time.Minute).Unix(), "token_use": "access_token",
				"groups": []string{"funnelbarn-users"},
			})
		}},
		{"garbage", func() string { return "not-a-jwt" }},
	}
	verify := NewTokenVerifier(f.client(), testResourceURL, openStore(t), quietLogger())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := verify(context.Background(), tc.token(), nil)
			if err == nil {
				t.Fatalf("expected rejection, got %+v", info)
			}
			if !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("error %v does not wrap auth.ErrInvalidToken (would not be a 401)", err)
			}
			t.Logf("rejected: %v", err)
		})
	}
}

func TestTokenVerifier_ValidToken(t *testing.T) {
	f := newFakeIssuer(t)
	store := openStore(t)
	verify := NewTokenVerifier(f.client(), testResourceURL, store, quietLogger())

	exp := time.Now().Add(10 * time.Minute).Truncate(time.Second)
	info, err := verify(context.Background(), f.accessToken(t, map[string]any{
		"exp": exp.Unix(),
		// aud may list more than the resource; it only has to contain it.
		"aud": []string{"https://other.example", testResourceURL},
	}), nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if info.UserID != testSub {
		t.Errorf("UserID = %q, want %q", info.UserID, testSub)
	}
	if !slices.Equal(info.Scopes, []string{"mcp:read", "mcp:write"}) {
		t.Errorf("Scopes = %v", info.Scopes)
	}
	if !info.Expiration.Equal(exp) {
		t.Errorf("Expiration = %v, want %v", info.Expiration, exp)
	}
	if got := info.Extra[ExtraUsername]; got != "alice" {
		t.Errorf("username = %v, want alice", got)
	}
	u, err := store.FindUserByIAMBarnSub(context.Background(), testSub)
	if err != nil {
		t.Fatalf("user not created on first call: %v", err)
	}
	if u.Username != "alice" {
		t.Errorf("created user = %q, want alice", u.Username)
	}
	if n := f.userinfoCalls.Load(); n != 0 {
		t.Errorf("userinfo called %d times for a token with a groups claim", n)
	}
}

func TestTokenVerifier_ExistingUserKeepsName(t *testing.T) {
	f := newFakeIssuer(t)
	store := openStore(t)
	if _, err := store.CreateIAMBarnUser(context.Background(), testSub, "alice-dashboard"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	verify := NewTokenVerifier(f.client(), testResourceURL, store, quietLogger())

	// Access tokens often carry no preferred_username; the verifier must not
	// rename the user to the subject.
	info, err := verify(context.Background(), f.accessToken(t, map[string]any{"preferred_username": nil}), nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got := info.Extra[ExtraUsername]; got != "alice-dashboard" {
		t.Errorf("username = %v, want alice-dashboard", got)
	}
	u, _ := store.FindUserByIAMBarnSub(context.Background(), testSub)
	if u.Username != "alice-dashboard" {
		t.Errorf("stored username changed to %q", u.Username)
	}
}

func TestTokenVerifier_RoleBypassesGroup(t *testing.T) {
	f := newFakeIssuer(t)
	verify := NewTokenVerifier(f.client(), testResourceURL, nil, quietLogger())
	info, err := verify(context.Background(), f.accessToken(t, map[string]any{
		"groups": []string{}, "roles": []string{"owner"},
	}), nil)
	if err != nil {
		t.Fatalf("owner rejected: %v", err)
	}
	if info.Extra[ExtraUsername] != "alice" {
		t.Errorf("username without a user store = %v, want alice", info.Extra[ExtraUsername])
	}
	if n := f.userinfoCalls.Load(); n != 0 {
		t.Errorf("an empty groups claim must not trigger userinfo; got %d calls", n)
	}
}

func TestTokenVerifier_GroupsFromUserinfo(t *testing.T) {
	f := newFakeIssuer(t)
	f.userinfo.Store(map[string]any{"sub": testSub, "groups": []string{"funnelbarn-users"}})
	verify := NewTokenVerifier(f.client(), testResourceURL, nil, quietLogger())

	tok := f.accessToken(t, map[string]any{"groups": nil, "roles": nil})
	for i := range 3 {
		if _, err := verify(context.Background(), tok, nil); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if n := f.userinfoCalls.Load(); n != 1 {
		t.Errorf("userinfo called %d times, want 1 (cached per token)", n)
	}

	// A different token is a new cache entry.
	if _, err := verify(context.Background(), f.accessToken(t, map[string]any{"groups": nil, "jti": "2"}), nil); err != nil {
		t.Fatalf("second token: %v", err)
	}
	if n := f.userinfoCalls.Load(); n != 2 {
		t.Errorf("userinfo called %d times after a second token, want 2", n)
	}
}

func TestTokenVerifier_UserinfoWithoutGroupRejects(t *testing.T) {
	f := newFakeIssuer(t)
	f.userinfo.Store(map[string]any{"sub": testSub, "groups": []string{"other"}})
	verify := NewTokenVerifier(f.client(), testResourceURL, nil, quietLogger())
	_, err := verify(context.Background(), f.accessToken(t, map[string]any{"groups": nil}), nil)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("err = %v, want auth.ErrInvalidToken", err)
	}
}

func TestTokenVerifier_IssuerUnavailableIsNot401(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // nothing listens here any more
	c := fbauth.NewOIDCClient(fbauth.OIDCConfig{Issuer: srv.URL, ClientID: testClientID, ClientSecret: "s", RedirectURL: "https://x/cb"})
	verify := NewTokenVerifier(c, testResourceURL, nil, quietLogger())
	_, err := verify(context.Background(), "a.b.c", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("an unreachable issuer must not read as a bad token: %v", err)
	}
}
