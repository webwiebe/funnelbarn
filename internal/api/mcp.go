package api

import (
	"log/slog"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/wiebe-xyz/funnelbarn/internal/mcp"
)

// MCP endpoint paths. The metadata path is the RFC 9728 well-known prefix
// followed by the MCP endpoint's path, which is where an MCP client looks for
// it after a 401.
const (
	mcpPath             = "/api/v1/mcp"
	mcpResourceMetaPath = "/.well-known/oauth-protected-resource" + mcpPath
)

// Default per-user MCP rate limit, keyed by the token subject.
const (
	mcpRatePerMinute = 120
	mcpRateBurst     = 30
)

// mcpEnabled reports whether the MCP endpoint is served: it needs IAMBarn
// (OIDC) as the authorization server and a resource URL for the token
// audience. With local password login only, neither route exists.
func (s *Server) mcpEnabled() bool {
	return s.oidc != nil && s.mcpResourceURL != ""
}

// registerMCPRoutes mounts the MCP endpoint behind IAMBarn bearer-token
// authentication, plus its protected-resource metadata. Session cookies and
// FunnelBarn API keys are not accepted: a cookie-authenticated MCP endpoint
// would be a CSRF target.
func (s *Server) registerMCPRoutes() {
	if !s.mcpEnabled() {
		return
	}
	issuer := strings.TrimRight(s.oidc.Config().Issuer, "/")

	var users mcp.UserResolver
	if s.iambarnUsers != nil {
		users = s.iambarnUsers
	}
	verifier := mcp.NewTokenVerifier(s.oidc, s.mcpResourceURL, users, slog.Default())

	handler := mcp.NewHandler(mcp.Deps{
		Projects:      s.projects,
		Events:        s.events,
		Funnels:       s.funnels,
		Segments:      s.segments,
		Flags:         s.flags,
		ProjectHealth: s.projectHealth,
		Allow:         func(sub string) bool { return s.mcpLimiter.allow("mcp:" + sub) },
		Logger:        slog.Default(),
		Version:       s.version,
	})
	protected := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: s.mcpResourceMetadataURL(),
	})(handler)
	// No method in the pattern: POST carries JSON-RPC, GET and DELETE are part
	// of Streamable HTTP and the SDK answers them itself.
	s.mux.Handle(mcpPath, protected)

	// No method in the pattern either: the SDK handler answers the CORS
	// preflight itself and rejects everything but GET.
	s.mux.Handle(mcpResourceMetaPath, auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               s.mcpResourceURL,
		AuthorizationServers:   []string{issuer},
		ScopesSupported:        []string{"mcp:read", "mcp:write"},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "FunnelBarn",
	}))
}

// mcpResourceMetadataURL is the absolute URL of the protected-resource
// metadata, sent in WWW-Authenticate on a 401. It lives on the same origin as
// the resource URL (RFC 9728 section 3.1); PublicURL is the fallback when the
// resource URL does not parse.
func (s *Server) mcpResourceMetadataURL() string {
	if u, err := url.Parse(s.mcpResourceURL); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host + mcpResourceMetaPath
	}
	return strings.TrimRight(s.publicURL, "/") + mcpResourceMetaPath
}
