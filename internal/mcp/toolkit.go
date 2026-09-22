package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// Scopes an IAMBarn access token can carry for this server. The names match
// BrandTrace's MCP server.
const (
	scopeRead  = "mcp:read"
	scopeWrite = "mcp:write"
)

// ExtraUsername is the TokenInfo.Extra key under which the token verifier
// stores the FunnelBarn username of the caller.
const ExtraUsername = "username"

// Caller is the verified user behind a tool call.
type Caller struct {
	Sub      string   // IAMBarn subject
	Username string   // FunnelBarn username (may be empty)
	Scopes   []string // granted OAuth scopes
}

// callerFrom reads the caller from the verified bearer token.
func callerFrom(info *auth.TokenInfo) (Caller, bool) {
	if info == nil || info.UserID == "" {
		return Caller{}, false
	}
	c := Caller{Sub: info.UserID, Scopes: info.Scopes}
	if u, ok := info.Extra[ExtraUsername].(string); ok {
		c.Username = u
	}
	return c, true
}

// HasScope reports whether the caller was granted scope.
func (c Caller) HasScope(scope string) bool { return slices.Contains(c.Scopes, scope) }

// Call is what a tool handler gets besides its input: the caller, the request
// headers and the dependencies.
type Call struct {
	Caller Caller
	Header http.Header
	Deps   *Deps

	span interface{ SetAttributes(...attribute.KeyValue) }
}

// Project resolves the project a tool works on: the explicit argument when
// set, else the x-funnelbarn-project header. See ResolveProject.
func (c *Call) Project(ctx context.Context, arg string) (repository.Project, error) {
	p, err := ResolveProject(ctx, c.Deps.Projects, arg, c.Header.Get(ProjectHeader))
	if err == nil && c.span != nil {
		c.span.SetAttributes(attribute.String("funnelbarn.project_id", p.ID))
	}
	return p, err
}

// addTool registers a tool whose handler runs after the scope check and the
// per-user rate limit, inside a span, with errors mapped by toolError.
func addTool[In, Out any](s *mcp.Server, d *Deps, t *mcp.Tool, scope string, h func(ctx context.Context, c *Call, in In) (Out, error)) {
	mcp.AddTool(s, t, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		var zero Out
		var info *auth.TokenInfo
		var header http.Header
		if req.Extra != nil {
			info, header = req.Extra.TokenInfo, req.Extra.Header
		}
		caller, ok := callerFrom(info)
		if !ok {
			return nil, zero, errors.New("unauthenticated: sign in with IAMBarn and try again")
		}
		ctx, span := tracing.StartSpan(ctx, "mcp.tool."+t.Name,
			attribute.String("mcp.tool", t.Name),
			attribute.String("enduser.id", caller.Sub),
		)
		defer span.End()

		if err := requireScope(caller, scope); err != nil {
			d.Logger.WarnContext(ctx, "mcp tool scope denied", "tool", t.Name, "sub", caller.Sub, "scope", scope)
			return nil, zero, err
		}
		if d.Allow != nil && !d.Allow(caller.Sub) {
			d.Logger.WarnContext(ctx, "mcp tool rate limited", "tool", t.Name, "sub", caller.Sub)
			return nil, zero, errors.New("rate limit exceeded: wait a minute and try again")
		}

		out, err := h(ctx, &Call{Caller: caller, Header: header, Deps: d, span: span}, in)
		if err != nil {
			tracing.RecordError(span, err)
			return nil, zero, toolError(ctx, d.Logger, t.Name, err)
		}
		return nil, out, nil
	})
}

// requireScope returns a tool error naming the missing scope.
func requireScope(c Caller, scope string) error {
	if c.HasScope(scope) {
		return nil
	}
	return fmt.Errorf("missing scope %s: remove and re-add the FunnelBarn MCP server, granting %s at sign-in", scope, scope)
}

// ptr returns a pointer to v, for the *bool fields of mcp.ToolAnnotations.
func ptr[T any](v T) *T { return &v }
