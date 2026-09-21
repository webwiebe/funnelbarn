package api

// Full-stack tests for the MCP endpoint: the real API server over real SQLite,
// the fake IAMBarn from oidc_flow_test.go signing the access tokens.

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

const (
	testMCPResource = "http://funnelbarn.test/api/v1/mcp"
	testMCPMetaURL  = "http://funnelbarn.test/.well-known/oauth-protected-resource/api/v1/mcp"
	initializeBody  = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`
)

// mcpServer is an API server with OIDC against f and the MCP resource URL set.
func mcpServer(t *testing.T, f *fakeIdP, mutate func(cfg *ServerConfig)) (*Server, *repository.Store) {
	t.Helper()
	return oidcFlowServer(t, f, func(cfg *ServerConfig) {
		cfg.MCPResourceURL = testMCPResource
		if mutate != nil {
			mutate(cfg)
		}
	})
}

// mcpAccessToken signs an IAMBarn access token for the MCP resource; extra
// overrides claims.
func (f *fakeIdP) mcpAccessToken(t *testing.T, extra map[string]any) string {
	t.Helper()
	claims := map[string]any{
		"iss":                f.srv.URL,
		"sub":                fakeIdPSub,
		"aud":                []string{testMCPResource},
		"exp":                time.Now().Add(15 * time.Minute).Unix(),
		"iat":                time.Now().Unix(),
		"scope":              "mcp:read",
		"token_use":          "access_token",
		"preferred_username": "alice",
		"groups":             []string{"funnelbarn-users"},
		"roles":              []string{},
	}
	for k, v := range extra {
		claims[k] = v
	}
	return f.sign(t, claims)
}

// postMCP sends a raw JSON-RPC initialize to /api/v1/mcp with the given
// headers.
func postMCP(t *testing.T, srv *Server, hdr http.Header, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, mcpPath, strings.NewReader(initializeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range hdr {
		req.Header[k] = v
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func requireMCPUnauthorized(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body: %s)", w.Code, w.Body.String())
	}
	got := w.Header().Get("WWW-Authenticate")
	if !strings.Contains(got, `resource_metadata="`+testMCPMetaURL+`"`) {
		t.Errorf("WWW-Authenticate = %q, want resource_metadata=%q", got, testMCPMetaURL)
	}
}

func TestMCP_NoTokenIs401WithResourceMetadata(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := mcpServer(t, f, nil)
	requireMCPUnauthorized(t, postMCP(t, srv, nil, nil))

	// GET (the SSE stream) is behind the same gate.
	w := getJSON(t, srv, mcpPath, nil)
	requireMCPUnauthorized(t, w)
}

func TestMCP_SessionCookieAloneIs401(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := mcpServer(t, f, nil)
	// A live dashboard session, holding a token that would itself be valid.
	cookie, _ := insertOIDCSession(t, srv, repository.WebSession{
		Username:        "alice",
		IdpSub:          fakeIdPSub,
		AccessToken:     f.mcpAccessToken(t, nil),
		AccessExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	requireMCPUnauthorized(t, postMCP(t, srv, nil, cookie))
}

func TestMCP_APIKeyAloneIs401(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := mcpServer(t, f, nil)
	p, err := store.CreateProject(context.Background(), "Demo", "demo")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	key := scopedKey(t, store, p.ID, "fb_mcp_test_key", repository.APIKeyScopeAnalyticsRead)
	for _, hdr := range []http.Header{
		{http.CanonicalHeaderKey(auth.HeaderAPIKey): {key}},
		{http.CanonicalHeaderKey(auth.HeaderAPIKey): {"test-key"}}, // the ingest key
		{"Authorization": {"Bearer " + key}},
	} {
		requireMCPUnauthorized(t, postMCP(t, srv, hdr, nil))
	}
}

func TestMCP_RejectedTokens(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := mcpServer(t, f, nil)
	cases := map[string]string{
		"dashboard client audience": f.idToken(t, "", nil),
		"dashboard aud as array":    f.mcpAccessToken(t, map[string]any{"aud": []string{fakeIdPClientID}}),
		"expired":                   f.mcpAccessToken(t, map[string]any{"exp": time.Now().Add(-time.Minute).Unix()}),
		"wrong issuer":              f.mcpAccessToken(t, map[string]any{"iss": "https://evil.example"}),
		"id token use":              f.mcpAccessToken(t, map[string]any{"token_use": "id_token"}),
		"outside group":             f.mcpAccessToken(t, map[string]any{"groups": []string{"other"}}),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			requireMCPUnauthorized(t, postMCP(t, srv, http.Header{"Authorization": {"Bearer " + tok}}, nil))
		})
	}
}

// mcpClient connects an MCP client to srv served over real HTTP with token.
func mcpClient(t *testing.T, srv *Server, token string) *mcp.ClientSession {
	t.Helper()
	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             hs.URL + mcpPath,
		HTTPClient:           &http.Client{Transport: bearerTransport(token)},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

type bearerTransport string

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(r)
}

func TestMCP_ValidTokenInitializesAndListsTools(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := mcpServer(t, f, nil)
	cs := mcpClient(t, srv, f.mcpAccessToken(t, nil))

	if got := cs.InitializeResult().ServerInfo.Name; got != "funnelbarn" {
		t.Errorf("server name = %q, want funnelbarn", got)
	}
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("tools/list returned no tools")
	}
}

// A request that reaches the pod through the ingress arrives on the pod IP
// with the public Host header. The SDK's DNS-rebinding guard only rejects a
// loopback local address paired with a foreign Host, so this must pass.
func TestMCP_IngressHostIsAccepted(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := mcpServer(t, f, nil)
	req := httptest.NewRequest(http.MethodPost, mcpPath, strings.NewReader(initializeBody))
	req.Host = "funnelbarn.staging.wiebe.xyz"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+f.mcpAccessToken(t, nil))
	podAddr := &net.TCPAddr{IP: net.ParseIP("10.42.0.15"), Port: 8080}
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, podAddr))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
}

func TestMCP_PerUserRateLimit(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := mcpServer(t, f, func(cfg *ServerConfig) {
		cfg.MCPRatePerMinute = 0.001
		cfg.MCPRateBurst = 1
	})
	cs := mcpClient(t, srv, f.mcpAccessToken(t, nil))

	call := func() *mcp.CallToolResult {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
		if err != nil {
			t.Fatalf("tools/call: %v", err)
		}
		return res
	}
	call() // uses the one token in the bucket
	res := call()
	if !res.IsError || !strings.Contains(mcpResultText(res), "rate limit") {
		t.Fatalf("second call = %+v, want a rate-limit tool error", res)
	}

	// Another user has a bucket of their own.
	other := mcpClient(t, srv, f.mcpAccessToken(t, map[string]any{"sub": "user-sub-2"}))
	res, err := other.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("tools/call: %v", err)
	}
	if strings.Contains(mcpResultText(res), "rate limit") {
		t.Fatal("a second user was limited by the first user's calls")
	}
}

func mcpResultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestMCP_ProtectedResourceMetadata(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := mcpServer(t, f, nil)

	req := httptest.NewRequest(http.MethodGet, mcpResourceMetaPath, nil)
	req.Header.Set("Origin", "http://localhost:6274")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var meta struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
		BearerMethods        []string `json:"bearer_methods_supported"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode metadata: %v (%s)", err, w.Body.String())
	}
	if meta.Resource != testMCPResource {
		t.Errorf("resource = %q, want %q", meta.Resource, testMCPResource)
	}
	if len(meta.AuthorizationServers) != 1 || meta.AuthorizationServers[0] != f.srv.URL {
		t.Errorf("authorization_servers = %v, want [%s]", meta.AuthorizationServers, f.srv.URL)
	}
	if strings.Join(meta.ScopesSupported, " ") != "mcp:read mcp:write" {
		t.Errorf("scopes_supported = %v", meta.ScopesSupported)
	}
	if strings.Join(meta.BearerMethods, " ") != "header" {
		t.Errorf("bearer_methods_supported = %v", meta.BearerMethods)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("public metadata must not allow credentials, got %q", got)
	}

	// A browser-based client's preflight is answered by the metadata handler.
	pre := httptest.NewRequest(http.MethodOptions, mcpResourceMetaPath, nil)
	pre.Header.Set("Origin", "http://localhost:6274")
	pre.Header.Set("Access-Control-Request-Method", "GET")
	pw := httptest.NewRecorder()
	srv.ServeHTTP(pw, pre)
	if pw.Code != http.StatusNoContent || pw.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("preflight = %d ACAO=%q, want 204 *", pw.Code, pw.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestMCP_NotServedWithoutOIDC(t *testing.T) {
	check := func(t *testing.T, srv *Server) {
		t.Helper()
		if w := postMCP(t, srv, nil, nil); w.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404", mcpPath, w.Code)
		}
		if w := getJSON(t, srv, mcpResourceMetaPath, nil); w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 (Location %q)", mcpResourceMetaPath, w.Code, w.Header().Get("Location"))
		}
	}
	t.Run("oidc unset", func(t *testing.T) {
		srv, _ := fullServer(t, func(cfg *ServerConfig) { cfg.MCPResourceURL = testMCPResource })
		check(t, srv)
	})
	t.Run("resource url unset", func(t *testing.T) {
		srv, _ := oidcFlowServer(t, newFakeIdP(t), nil)
		check(t, srv)
	})
}

func TestMCP_LimiterCleanedUp(t *testing.T) {
	f := newFakeIdP(t)
	srv, _ := mcpServer(t, f, nil)
	if srv.mcpLimiter == nil {
		t.Fatal("mcpLimiter not built")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.StartCleanup(ctx) // must not panic; the mcp limiter is part of it
}
