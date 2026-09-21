// Package mcp serves FunnelBarn over the Model Context Protocol, so an AI
// assistant working in a project's repository can read that project's
// analytics and manage its funnels, segments and feature flags.
//
// It is a transport adapter at the same layer as internal/api: tools call the
// services in internal/service and never touch storage directly. Transport
// security (IAMBarn bearer tokens, protected-resource metadata) is applied by
// internal/api when it mounts NewHandler; tools read the verified caller from
// the request's TokenInfo.
//
// Tool conventions (every register*Tools function follows them):
//
//   - Register tools with addTool, which checks the scope, rate-limits per
//     user, opens a span and maps errors. Read tools use scopeRead and set
//     ReadOnlyHint; write tools use scopeWrite; delete tools also set
//     DestructiveHint.
//   - Every project-level tool takes an optional `project` argument (slug or
//     ID) and resolves it with Call.Project, which falls back to the
//     x-funnelbarn-project header the repository's .mcp.json sets.
//   - A tool that takes an object ID (funnel, segment, flag) loads the object
//     and returns domain.ErrNotFound unless obj.ProjectID equals the resolved
//     project. Service Get* methods take a bare ID, so skipping this check
//     lets a call scoped to project A change an object in project B.
//   - Time ranges are `from` / `to` in RFC 3339 or YYYY-MM-DD, parsed with
//     parseRange (default: the last 7 days).
//   - Return invalidInput(...) for bad arguments (logged at Warn). Service
//     errors pass through unchanged; toolError maps domain kinds to readable
//     messages and logs anything unexpected at Error so selflog reports it.
//   - Keep list payloads small: cap them and say in the description how to
//     page.
package mcp

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// ProjectHeader is the request header that picks the default project. Ingest
// uses the same header.
const ProjectHeader = "x-funnelbarn-project"

// Deps holds what the tools need. Services are the interfaces from
// internal/service; nil services leave their tools unusable, so production
// wiring sets all of them.
type Deps struct {
	Projects      service.Projects
	Events        service.Events
	Funnels       service.Funnels
	Segments      service.Segments
	Flags         service.Flags
	ProjectHealth service.ProjectHealth

	// SetupDoc renders a project's setup guide, the document served at
	// GET /api/v1/setup/{slug}. Nil makes get_setup_guide return an error.
	SetupDoc func(ctx context.Context, project repository.Project) (string, error)

	// Allow reports whether the caller identified by key (the token subject)
	// may make another call now. Nil disables rate limiting.
	Allow func(key string) bool

	Logger  *slog.Logger
	Version string
}

// NewServer builds the MCP server with every tool registered.
func NewServer(deps Deps) *mcp.Server {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "funnelbarn", Version: deps.Version}, &mcp.ServerOptions{
		Instructions: serverInstructions,
	})
	d := &deps
	registerProjectTools(s, d)
	registerAnalyticsTools(s, d)
	registerFunnelTools(s, d)
	registerSegmentTools(s, d)
	registerFlagTools(s, d)
	registerSetupTools(s, d)
	return s
}

// NewHandler serves the MCP server over stateless Streamable HTTP with JSON
// responses, so a pod restart never breaks a connected client. The caller
// wraps it in bearer-token authentication.
func NewHandler(deps Deps) http.Handler {
	srv := NewServer(deps)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		Logger:       deps.Logger,
	})
}

const serverInstructions = `FunnelBarn is a self-hosted product analytics service: events, funnels, segments and feature flags per project.

Every project tool works on the repository's default project (the x-funnelbarn-project header in .mcp.json) unless you pass a "project" argument (slug or ID). Call list_projects to see the projects you can reach and get_project to check which one is the default.

Before defining a funnel step or a segment rule, call list_event_names and list_event_properties so names match real events. Write tools need the mcp:write scope; if a call says the scope is missing, the user has to remove and re-add this server granting mcp:write.`
