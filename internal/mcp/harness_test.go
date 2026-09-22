package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// Shared test harness for every tools_*_test.go file. It serves the real MCP
// handler over HTTP with a real SQLite store; a fake bearer verifier turns
// the token string into the granted scopes, so a test picks its scopes by
// picking its token.

const (
	tokenRead      = "read"       // mcp:read
	tokenReadWrite = "read-write" // mcp:read mcp:write
)

// testEnv is a running MCP server backed by an in-memory store.
type testEnv struct {
	Store *repository.Store
	Deps  Deps
	URL   string
}

// newTestEnv starts the MCP server over a fresh in-memory store. mutate, when
// non-nil, can adjust Deps before the server is built.
func newTestEnv(t *testing.T, mutate func(*Deps)) *testEnv {
	t.Helper()
	store, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	deps := Deps{
		Projects:      service.NewProjectService(store),
		Events:        service.NewEventService(store),
		Funnels:       service.NewFunnelService(store),
		Segments:      service.NewSegmentService(store),
		Flags:         service.NewFlagService(store),
		ProjectHealth: service.NewProjectHealthService(store),
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if mutate != nil {
		mutate(&deps)
	}

	return &testEnv{Store: store, Deps: deps, URL: serveTestServer(t, NewServer(deps))}
}

// serveTestServer serves s over stateless Streamable HTTP behind the fake
// bearer verifier and returns its URL.
func serveTestServer(t *testing.T, s *mcp.Server) string {
	t.Helper()
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	srv := httptest.NewServer(auth.RequireBearerToken(fakeVerify, nil)(h))
	t.Cleanup(srv.Close)
	return srv.URL
}

func fakeVerify(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	info := &auth.TokenInfo{UserID: "sub-test", Expiration: time.Now().Add(time.Hour),
		Extra: map[string]any{ExtraUsername: "tester"}}
	switch token {
	case tokenRead:
		info.Scopes = []string{scopeRead}
	case tokenReadWrite:
		info.Scopes = []string{scopeRead, scopeWrite}
	default:
		return nil, auth.ErrInvalidToken
	}
	return info, nil
}

// createProject inserts an active project and returns it.
func (e *testEnv) createProject(t *testing.T, name, slug string) repository.Project {
	t.Helper()
	p, err := e.Store.CreateProject(context.Background(), name, slug)
	if err != nil {
		t.Fatalf("CreateProject(%s): %v", slug, err)
	}
	return p
}

// connect opens a client session with token and, when project is non-empty,
// the x-funnelbarn-project header set to it.
func (e *testEnv) connect(t *testing.T, token, project string) *mcp.ClientSession {
	t.Helper()
	hdr := http.Header{"Authorization": {"Bearer " + token}}
	if project != "" {
		hdr.Set(ProjectHeader, project)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             e.URL,
		HTTPClient:           &http.Client{Transport: headerTransport{hdr}},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

type headerTransport struct{ h http.Header }

func (ht headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range ht.h {
		r.Header[k] = v
	}
	return http.DefaultTransport.RoundTrip(r)
}

// callTool calls a tool and returns its result. It fails the test on a
// protocol error; tool errors come back with IsError set.
func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

// callOK calls a tool, requires success and decodes its structured output
// into out.
func callOK(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, out any) {
	t.Helper()
	res := callTool(t, cs, name, args)
	if res.IsError {
		t.Fatalf("%s returned a tool error: %s", name, resultText(res))
	}
	if out == nil {
		return
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("%s: marshal structured content: %v", name, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s: decode structured content %s: %v", name, raw, err)
	}
}

// callErr calls a tool, requires a tool error and returns its message.
func callErr(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res := callTool(t, cs, name, args)
	if !res.IsError {
		t.Fatalf("%s: expected a tool error, got success: %s", name, resultText(res))
	}
	return resultText(res)
}

func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
