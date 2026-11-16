package api

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/features/abtests"
	"github.com/wiebe-xyz/funnelbarn/internal/features/agents"
	"github.com/wiebe-xyz/funnelbarn/internal/features/segments"
	"github.com/wiebe-xyz/funnelbarn/internal/features/widgets"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// ServerConfig holds all configuration for the API server.
type ServerConfig struct {
	Ingest         *ingest.Handler
	Projects       service.Projects
	Funnels        service.Funnels
	ABTests        abtests.API
	Flags          service.Flags
	Events         service.Events
	Overview       service.Overview
	Sessions       service.Sessions
	APIKeys        service.APIKeys
	Widgets        widgets.API
	UserAuth       *auth.UserAuthenticator
	SessionManager *auth.SessionManager
	AllowedOrigins []string
	PublicURL      string
	DB             Pinger
	Version        string
	Commit         string
	BuildTime      string
	TrustedProxies []string

	LoginRatePerMinute  float64
	LoginRateBurst      float64
	APIRatePerMinute    float64
	APIRateBurst        float64
	IngestRatePerMinute float64
	IngestRateBurst     float64
	SetupRatePerMinute  float64
	SetupRateBurst      float64

	MetricsToken     string
	BugbarnEndpoint  string
	BugbarnIngestKey string
	BugbarnProject   string
	DogfoodAPIKey    string
	DogfoodProject   string
	// DogfoodEnvironment is the deploy environment the dashboard tags its own
	// analytics events with, so one project and key serve every environment.
	DogfoodEnvironment string

	IAMBarnUsers service.IAMBarnUsers

	// PostLogoutRedirectURI is exposed to the frontend so the hosted IAMBarn
	// components can pass it to /oauth2/end-session. It should point at the
	// local session-clearing endpoint (/api/v1/auth/oidc/logged-out).
	PostLogoutRedirectURI string

	// OIDC, when non-nil, enables the confidential-client OIDC login flow at
	// /api/v1/oidc/{login,callback} — the single OIDC flow.
	OIDC *auth.OIDCClient

	// WebSessions is the server-side session store. Required whenever any
	// authentication mechanism is configured: sessions are rows, and deleting
	// a row revokes the session.
	WebSessions service.WebSessions

	// Environment is the normalized deployment tier (internal/environment).
	// Outside development, cookies default to Secure=true even when the TLS
	// terminator's X-Forwarded-Proto is missing or untrusted.
	Environment string

	// OIDCRefreshGrace bounds how long a session with a transiently failing
	// refresh (IdP outage, network) is served stale before it is cut off.
	// invalid_grant never gets grace. Zero means the 1h default.
	OIDCRefreshGrace time.Duration

	// LocalUsersExist reports whether any local/DB users exist at startup. It is
	// one of the signals that authentication is configured: if true, DB-user
	// login is a real mechanism and session-authed routes must fail closed.
	LocalUsersExist bool

	InstanceSettings  service.InstanceSettings
	Privacy           service.Privacy
	Segments          segments.API
	Recordings        service.Recordings
	RecordingSettings service.RecordingSettings
	ProjectHealth     service.ProjectHealth
	Agents            agents.API

	// SpanRelay forwards the dashboard's own browser spans to SpanBarn. Nil when
	// telemetry export is not configured; the endpoint then accepts and drops.
	SpanRelay *tracing.SpanRelay

	// FlagAutoRegisterMax caps auto-created flags per project on the SDK evaluate
	// endpoint (0 disables auto-registration).
	FlagAutoRegisterMax int

	// Commands, when set, takes the evaluate endpoint's bookkeeping writes
	// (spec 012). Nil keeps the old fire-and-forget goroutines.
	Commands command.Bus
	// ReadPoolWait returns the read pool's cumulative connection wait
	// (sql.DBStats.WaitDuration). The evaluate span reports its delta.
	ReadPoolWait func() time.Duration
	// Ready, when set, gates GET /api/v1/ready: a non-nil error answers 503
	// with that reason. A reader process uses it to wait for the writer's
	// migrations. Nil means ready whenever the database answers.
	Ready func(ctx context.Context) error
	// WriterURL, when set, makes this server a reader: it forwards every
	// request that writes the database to the writer (spec 012).
	WriterURL string
	// InternalToken authenticates a reader's forwarded requests to the writer.
	InternalToken string

	// MCPResourceURL is the OAuth resource identifier of the MCP endpoint and
	// the audience its IAMBarn access tokens must carry. The MCP endpoint and
	// its protected-resource metadata are served only when this and OIDC are
	// both set.
	MCPResourceURL string

	// MCPRatePerMinute / MCPRateBurst bound MCP tool calls per user (token
	// subject). Zero means 120/min with a burst of 30.
	MCPRatePerMinute float64
	MCPRateBurst     float64

	// GitHubWebhookSecret signs GitHub webhook deliveries; empty leaves the
	// route unregistered (404). GitHubWebhookProject is the slug they are
	// filed under.
	GitHubWebhookSecret  string
	GitHubWebhookProject string
}
