package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestParseRange(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name, from, to   string
		wantFrom, wantTo time.Time
		wantErr          bool
	}{
		{name: "default seven days", wantFrom: now.Add(-7 * 24 * time.Hour), wantTo: now},
		{name: "dates", from: "2026-09-01", to: "2026-09-02",
			wantFrom: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			wantTo:   time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)},
		{name: "rfc3339", from: "2026-09-01T10:00:00Z", to: "2026-09-01T14:00:00+02:00",
			wantFrom: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
			wantTo:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)},
		{name: "only to", to: "2026-09-10T00:00:00Z",
			wantFrom: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC),
			wantTo:   time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)},
		{name: "bad from", from: "yesterday", wantErr: true},
		{name: "bad to", to: "09/01/2026", wantErr: true},
		{name: "reversed", from: "2026-09-05", to: "2026-09-01", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to, err := parseRange(tc.from, tc.to, now)
			if tc.wantErr {
				var in *inputError
				if !errors.As(err, &in) {
					t.Fatalf("want inputError, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRange: %v", err)
			}
			if !from.Equal(tc.wantFrom) || !to.Equal(tc.wantTo) {
				t.Errorf("got %s..%s, want %s..%s", from, to, tc.wantFrom, tc.wantTo)
			}
		})
	}
}

func TestToolError(t *testing.T) {
	env := newTestEnv(t, nil)
	log := env.Deps.Logger
	ctx := context.Background()
	cases := []struct {
		err  error
		want string
	}{
		{invalidInput("limit: must be positive"), "limit: must be positive"},
		{&domain.ValidationError{Field: "name", Message: "required"}, "name: required"},
		{domain.ErrNotFound, "not found"},
		{domain.ErrConflict, "already exists"},
		{errors.New("sqlite: database is locked"), "internal error"},
	}
	for _, tc := range cases {
		if got := toolError(ctx, log, "t", tc.err).Error(); got != tc.want {
			t.Errorf("toolError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// TestAddTool_ScopeAndRateLimit checks the guards every tool runs through,
// using throwaway tools on a test server.
func TestAddTool_ScopeAndRateLimit(t *testing.T) {
	allowed := true
	env := newTestEnv(t, func(d *Deps) { d.Allow = func(string) bool { return allowed } })

	// Register throwaway tools on a separate server that shares the harness.
	s := mcp.NewServer(&mcp.Implementation{Name: "t"}, nil)
	type in struct{}
	type out struct {
		Sub string `json:"sub"`
	}
	addTool(s, &env.Deps, &mcp.Tool{Name: "read_tool"}, scopeRead, func(_ context.Context, c *Call, _ in) (out, error) {
		return out{Sub: c.Caller.Sub}, nil
	})
	addTool(s, &env.Deps, &mcp.Tool{Name: "write_tool"}, scopeWrite, func(_ context.Context, c *Call, _ in) (out, error) {
		return out{Sub: c.Caller.Sub}, nil
	})
	url := serveTestServer(t, s)
	cs := (&testEnv{URL: url}).connect(t, tokenRead, "")

	var got out
	callOK(t, cs, "read_tool", nil, &got)
	if got.Sub != "sub-test" {
		t.Errorf("caller sub = %q", got.Sub)
	}
	if msg := callErr(t, cs, "write_tool", nil); !strings.Contains(msg, "mcp:write") {
		t.Errorf("scope error should name mcp:write, got %q", msg)
	}
	allowed = false
	if msg := callErr(t, cs, "read_tool", nil); !strings.Contains(msg, "rate limit") {
		t.Errorf("want rate limit error, got %q", msg)
	}
}
