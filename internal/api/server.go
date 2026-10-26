package api

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// Pinger is satisfied by any type with a Ping method (e.g. *repository.Store).
type Pinger interface {
	Ping(ctx context.Context) error
}

// ServerConfig holds all configuration for the API server.
type ServerConfig struct {
	Ingest         *ingest.Handler
	Projects       service.Projects
	Funnels        service.Funnels
	ABTests        service.ABTests
	Flags          service.Flags
	Events         service.Events
	Overview       service.Overview
	Sessions       service.Sessions
	APIKeys        service.APIKeys
	Widgets        service.Widgets
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

	IAMBarnUsers IAMBarnUserRepo

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
	WebSessions WebSessionStore

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

	InstanceSettings  InstanceSettingsRepo
	GeoAnonymizer     GeoAnonymizer
	Segments          service.Segments
	Distributions     DistributionRepo
	Recordings        service.Recordings
	RecordingSettings ProjectRecordingSettingsRepo
	ProjectHealth     service.ProjectHealth
	Agents            service.Agents

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
}

// Server is the main HTTP API server.
type Server struct {
	mux                 *http.ServeMux
	db                  Pinger
	projects            service.Projects
	funnels             service.Funnels
	abtests             service.ABTests
	flags               service.Flags
	events              service.Events
	overview            service.Overview
	sessions            service.Sessions
	apikeys             service.APIKeys
	widgets             service.Widgets
	ingest              *ingest.Handler
	userAuth            *auth.UserAuthenticator
	sessionManager      *auth.SessionManager
	allowedOrigins      []string
	publicURL           string
	metricsToken        string
	version             string
	commit              string
	buildTime           string
	bugbarnEndpoint     string
	bugbarnIngestKey    string
	bugbarnProject      string
	dogfoodAPIKey       string
	dogfoodProject      string
	trustedProxies      []string
	iambarnUsers        IAMBarnUserRepo
	postLogoutRedirect  string
	oidc                *auth.OIDCClient
	webSessions         WebSessionStore
	environment         string
	oidcRefreshGrace    time.Duration
	refreshGroup        singleflight.Group
	localUsersExist     bool
	instanceSettings    InstanceSettingsRepo
	geoAnonymizer       GeoAnonymizer
	segments            service.Segments
	distributions       DistributionRepo
	recordings          service.Recordings
	recordingSettings   ProjectRecordingSettingsRepo
	projectHealth       service.ProjectHealth
	agents              service.Agents
	flagAutoRegisterMax int
	commands            command.Bus
	readPoolWait        func() time.Duration
	ready               func(ctx context.Context) error
	forwarder           *writeForwarder // nil unless this is a reader
	internalToken       string
	slow                slowEvaluate
	evalLimits          evaluateLimits
	spanRelay           *tracing.SpanRelay
	mcpResourceURL      string

	loginLimiter  *rateLimiter
	eventsLimiter *rateLimiter
	apiLimiter    *rateLimiter
	setupLimiter  *rateLimiter
	mcpLimiter    *rateLimiter // per MCP token subject

	// securityMW is the precomputed security-headers middleware; its CSP
	// whitelists the IAMBarn origin (if configured) for the hosted components.
	securityMW func(http.Handler) http.Handler
}

// NewServer creates the API server and registers all routes.
func NewServer(cfg ServerConfig) *Server {
	setupRate := cfg.SetupRatePerMinute
	if setupRate == 0 {
		setupRate = 10
	}
	setupBurst := cfg.SetupRateBurst
	if setupBurst == 0 {
		setupBurst = 5
	}
	mcpRate := cfg.MCPRatePerMinute
	if mcpRate == 0 {
		mcpRate = mcpRatePerMinute
	}
	mcpBurst := cfg.MCPRateBurst
	if mcpBurst == 0 {
		mcpBurst = mcpRateBurst
	}

	s := &Server{
		mux:                 http.NewServeMux(),
		db:                  cfg.DB,
		version:             cfg.Version,
		commit:              cfg.Commit,
		buildTime:           cfg.BuildTime,
		projects:            cfg.Projects,
		funnels:             cfg.Funnels,
		abtests:             cfg.ABTests,
		flags:               cfg.Flags,
		events:              cfg.Events,
		overview:            cfg.Overview,
		sessions:            cfg.Sessions,
		apikeys:             cfg.APIKeys,
		widgets:             cfg.Widgets,
		ingest:              cfg.Ingest,
		userAuth:            cfg.UserAuth,
		sessionManager:      cfg.SessionManager,
		allowedOrigins:      cfg.AllowedOrigins,
		publicURL:           cfg.PublicURL,
		bugbarnEndpoint:     cfg.BugbarnEndpoint,
		bugbarnIngestKey:    cfg.BugbarnIngestKey,
		bugbarnProject:      cfg.BugbarnProject,
		dogfoodAPIKey:       cfg.DogfoodAPIKey,
		dogfoodProject:      cfg.DogfoodProject,
		trustedProxies:      cfg.TrustedProxies,
		loginLimiter:        newRateLimiter(cfg.LoginRatePerMinute, cfg.LoginRateBurst),
		eventsLimiter:       newRateLimiter(cfg.IngestRatePerMinute, cfg.IngestRateBurst),
		apiLimiter:          newRateLimiter(cfg.APIRatePerMinute, cfg.APIRateBurst),
		setupLimiter:        newRateLimiter(setupRate, setupBurst),
		mcpLimiter:          newRateLimiter(mcpRate, mcpBurst),
		mcpResourceURL:      cfg.MCPResourceURL,
		iambarnUsers:        cfg.IAMBarnUsers,
		postLogoutRedirect:  cfg.PostLogoutRedirectURI,
		oidc:                cfg.OIDC,
		webSessions:         cfg.WebSessions,
		environment:         environment.Normalize(cfg.Environment),
		oidcRefreshGrace:    cfg.OIDCRefreshGrace,
		localUsersExist:     cfg.LocalUsersExist,
		instanceSettings:    cfg.InstanceSettings,
		geoAnonymizer:       cfg.GeoAnonymizer,
		segments:            cfg.Segments,
		distributions:       cfg.Distributions,
		recordings:          cfg.Recordings,
		recordingSettings:   cfg.RecordingSettings,
		projectHealth:       cfg.ProjectHealth,
		agents:              cfg.Agents,
		flagAutoRegisterMax: cfg.FlagAutoRegisterMax,
		commands:            cfg.Commands,
		readPoolWait:        cfg.ReadPoolWait,
		ready:               cfg.Ready,
		internalToken:       cfg.InternalToken,
		slow:                defaultSlowEvaluate,
		spanRelay:           cfg.SpanRelay,
	}
	if cfg.WriterURL != "" {
		s.forwarder = newWriteForwarder(cfg.WriterURL, cfg.InternalToken)
	}
	if s.oidcRefreshGrace <= 0 {
		s.oidcRefreshGrace = time.Hour
	}
	s.securityMW = securityHeaders(originOf(s.iambarnIssuer()))
	s.registerRoutes()
	return s
}

// WebSessionStore is the narrow interface over the web_sessions table used by
// the session middleware, the OIDC callback, and the logout paths.
type WebSessionStore interface {
	CreateWebSession(ctx context.Context, ws repository.WebSession) error
	GetWebSession(ctx context.Context, idHash string) (repository.WebSession, error)
	UpdateWebSessionTokens(ctx context.Context, idHash, idToken, accessToken, refreshToken string, accessExpiresAt int64, claimsJSON string, lastRefreshAt int64) error
	MarkWebSessionRefreshFailing(ctx context.Context, idHash string, since int64) error
	DeleteWebSession(ctx context.Context, idHash string) error
	DeleteWebSessionsByIdpSid(ctx context.Context, sid string) (int64, error)
	DeleteWebSessionsByIdpSub(ctx context.Context, sub string) (int64, error)
}

// StartCleanup begins periodic rate limiter cleanup goroutines.
// Call this once with a context that is cancelled on shutdown.
func (s *Server) StartCleanup(ctx context.Context) {
	s.loginLimiter.startCleanup(ctx)
	s.eventsLimiter.startCleanup(ctx)
	s.apiLimiter.startCleanup(ctx)
	s.setupLimiter.startCleanup(ctx)
	s.mcpLimiter.startCleanup(ctx)
}

func (s *Server) registerRoutes() {
	// Metrics — open when no token configured, bearer-protected otherwise.
	s.mux.Handle("GET /metrics", s.metricsHandler())

	// Public
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/ready", s.handleReady)
	s.mux.HandleFunc("GET /api/v1/version", s.handleVersion)
	s.mux.Handle("GET /api/v1/client-config", s.limit(s.eventsLimiter, http.HandlerFunc(s.handleClientConfig)))
	s.mux.HandleFunc("GET "+themeManifestPath, s.handleThemeManifest)

	// Ingest (API key required)
	s.mux.Handle("POST /api/v1/events", s.limit(s.eventsLimiter, s.ingest))
	s.mux.Handle("POST /api/v1/recordings/chunk", s.limit(s.eventsLimiter, http.HandlerFunc(s.handleIngestRecordingChunk)))
	// Cross-stack trace lookup (API key required): trace_id -> session/recording.
	// Consumed by SpanBarn/BugBarn deep-links and the replay CLI.
	s.mux.HandleFunc("GET /api/v1/traces/{trace_id}", s.handleLookupTrace)
	// API-key-authed replay-read for programmatic consumers (the replay CLI).
	s.mux.HandleFunc("GET /api/v1/recordings/{rid}/chunks/{index}", s.handleGetRecordingChunkByKey)

	// Auth
	s.mux.Handle("POST /api/v1/login", s.limit(s.loginLimiter, http.HandlerFunc(s.handleLogin)))
	s.mux.HandleFunc("POST /api/v1/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/v1/me", s.requireSession(s.handleMe))

	// Setup guide. It used to hand out a working ingest key for any slug, so
	// it needs a dashboard session like the API key routes it points to.
	s.mux.Handle("GET /api/v1/setup/{slug}", s.limit(s.setupLimiter, s.requireSession(s.handleSetup)))

	// Landing endpoint for IAMBarn RP-initiated logout — clears the local
	// session, then redirects to /login. Registered as post_logout_redirect_uri
	// (path kept stable so the IdP allowlist doesn't change).
	s.mux.HandleFunc("GET /api/v1/auth/oidc/logged-out", s.handleOIDCLoggedOut)

	// OIDC confidential-client flow with PKCE (gated by FUNNELBARN_OIDC_* env
	// vars) — the single OIDC login flow.
	s.mux.Handle("GET /api/v1/oidc/login", s.limit(s.loginLimiter, http.HandlerFunc(s.handleOIDCConfidentialLogin)))
	s.mux.HandleFunc("GET /api/v1/oidc/callback", s.handleOIDCConfidentialCallback)
	// Back-channel logout: iambarn pushes a signed logout token; the token
	// itself authenticates the request (public, rate-limited, no CSRF).
	s.mux.Handle("POST /api/v1/oidc/backchannel-logout", s.limit(s.loginLimiter, http.HandlerFunc(s.handleBackchannelLogout)))

	// Projects. The list is also reachable with an analytics:read token, which
	// narrows it to that token's own project — a scripted readout has to be
	// able to resolve its project without a dashboard session.
	s.mux.HandleFunc("GET /api/v1/projects", s.requireSessionOrToken(repository.APIKeyScopeAnalyticsRead, s.handleListProjects))
	s.mux.HandleFunc("POST /api/v1/projects", s.requireSession(s.handleCreateProject))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}", s.requireSession(s.handleUpdateProject))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}", s.requireSession(s.handleDeleteProject))

	// Dashboard & analytics. The read-only routes a scripted readout needs also
	// accept a project-scoped analytics:read token — see internal/api/api_tokens.go.
	s.mux.HandleFunc("GET /api/v1/projects/{id}/dashboard", s.requireSessionOrToken(repository.APIKeyScopeAnalyticsRead, s.handleDashboard))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flows", s.requireSession(s.handlePageFlows))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/events", s.requireSessionOrToken(repository.APIKeyScopeAnalyticsRead, s.handleListEvents))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/event-names", s.requireSessionOrToken(repository.APIKeyScopeAnalyticsRead, s.handleEventNames))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/event-counts", s.requireSessionOrToken(repository.APIKeyScopeAnalyticsRead, s.handleEventCounts))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/event-properties", s.requireSession(s.handleEventProperties))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/event-property-values", s.requireSession(s.handleEventPropertyValues))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/environments", s.requireSession(s.handleEnvironments))

	// Cross-project overview (instance-wide analytics; no project in the path).
	s.mux.HandleFunc("GET /api/v1/overview", s.requireSession(s.handleOverview))
	s.mux.HandleFunc("GET /api/v1/overview/events", s.requireSession(s.handleOverviewEvents))

	// Canonical event catalog (instance-level vocabulary).
	s.mux.HandleFunc("GET /api/v1/canonical-events", s.requireSession(s.handleListCanonicalEvents))
	s.mux.HandleFunc("POST /api/v1/canonical-events", s.requireSession(s.handleCreateCanonicalEvent))
	s.mux.HandleFunc("PUT /api/v1/canonical-events/{key}", s.requireSession(s.handleUpdateCanonicalEvent))
	s.mux.HandleFunc("DELETE /api/v1/canonical-events/{key}", s.requireSession(s.handleDeleteCanonicalEvent))

	// Per-project raw→canonical event mappings + suggestions.
	s.mux.HandleFunc("GET /api/v1/projects/{id}/event-mappings", s.requireSession(s.handleListMappings))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}/event-mappings", s.requireSession(s.handleSetMappings))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}/event-mappings/{raw}", s.requireSession(s.handleDeleteMapping))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/event-mappings/suggestions", s.requireSession(s.handleMappingSuggestions))

	// Cross-project canonical funnels.
	s.mux.HandleFunc("GET /api/v1/overview/funnels", s.requireSession(s.handleListCanonicalFunnels))
	s.mux.HandleFunc("POST /api/v1/overview/funnels", s.requireSession(s.handleCreateCanonicalFunnel))
	s.mux.HandleFunc("PUT /api/v1/overview/funnels/{id}", s.requireSession(s.handleUpdateCanonicalFunnel))
	s.mux.HandleFunc("DELETE /api/v1/overview/funnels/{id}", s.requireSession(s.handleDeleteCanonicalFunnel))
	s.mux.HandleFunc("GET /api/v1/overview/funnels/{id}/analysis", s.requireSession(s.handleCanonicalFunnelAnalysis))

	// Funnels. Listing and step conversion also accept an analytics:read token;
	// creating, editing and deleting a funnel stay session-only.
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels", s.requireSessionOrToken(repository.APIKeyScopeAnalyticsRead, s.handleListFunnels))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/funnels", s.requireSession(s.handleCreateFunnel))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}/funnels/{fid}", s.requireSession(s.handleUpdateFunnel))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}/funnels/{fid}", s.requireSession(s.handleDeleteFunnel))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels/{fid}/analysis", s.requireSessionOrToken(repository.APIKeyScopeAnalyticsRead, s.handleFunnelAnalysis))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels/{fid}/segments", s.requireSession(s.handleFunnelSegments))

	// Feature Flags. Reads and updates also accept a project-scoped API token
	// (scope flags:read / flags:write) so a service can report and toggle a
	// flag without a browser session — see internal/api/flag_tokens.go.
	// Creation and deletion stay session-only: auto-registration already covers
	// creation, and a token that can turn an outbound-email gate off should not
	// also be able to delete the gate.
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flags", s.requireSessionOrToken(repository.APIKeyScopeFlagsRead, s.handleListFlags))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/flags", s.requireSession(s.handleCreateFlag))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flags/{fid}", s.requireSessionOrToken(repository.APIKeyScopeFlagsRead, s.handleGetFlag))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}/flags/{fid}", s.requireSessionOrToken(repository.APIKeyScopeFlagsWrite, s.handleUpdateFlag))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}/flags/{fid}", s.requireSession(s.handleDeleteFlag))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flags/{fid}/analysis", s.requireSessionOrToken(repository.APIKeyScopeFlagsRead, s.handleFlagAnalysis))
	s.registerFlagInsightRoutes()
	// Dogfooding playground — same flag-eval logic as the customer endpoint
	// but session-authed so the dashboard can call it without an API key in the browser.
	s.mux.HandleFunc("POST /api/v1/projects/{id}/flags/evaluate", s.requireSession(s.handlePlaygroundEvaluateFlag))

	// Dashboard self-instrumentation. Session-authed so the browser never holds
	// a telemetry or BugBarn key — see internal/api/telemetry.go.
	s.mux.HandleFunc("POST /api/v1/telemetry", s.requireSession(s.handleTelemetry))
	s.mux.HandleFunc("POST /api/v1/client-errors", s.requireSession(s.handleClientError))

	// Flag evaluation (API key required, like ingest)
	s.mux.Handle("POST /api/v1/evaluate", s.limit(s.eventsLimiter, http.HandlerFunc(s.handleEvaluateFlag)))

	// A/B Tests
	s.mux.HandleFunc("GET /api/v1/projects/{id}/abtests", s.requireSession(s.handleListABTests))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/abtests", s.requireSession(s.handleCreateABTest))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/abtests/{abid}/analysis", s.requireSession(s.handleABTestAnalysis))

	// Widgets (Insights)
	s.mux.HandleFunc("GET /api/v1/projects/{id}/widgets", s.requireSession(s.handleListWidgets))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/widgets", s.requireSession(s.handleCreateWidget))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}/widgets/{wid}", s.requireSession(s.handleUpdateWidget))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}/widgets/{wid}", s.requireSession(s.handleDeleteWidget))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/widgets/{wid}/breakdown", s.requireSession(s.handleWidgetBreakdown))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/widgets/breakdowns", s.requireSession(s.handleBatchBreakdowns))

	// Sessions
	s.mux.HandleFunc("GET /api/v1/projects/{id}/sessions", s.requireSession(s.handleListSessions))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/sessions/active", s.requireSession(s.handleActiveSessionCount))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/session-distributions", s.requireSession(s.handleSessionDistributions))
	s.registerAgentRoutes()

	// Recordings
	s.mux.HandleFunc("GET /api/v1/projects/{id}/recordings", s.requireSession(s.handleListRecordings))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/recordings/{rid}/chunks/{index}", s.requireSession(s.handleGetRecordingChunk))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}/recordings/{rid}", s.requireSession(s.handleDeleteRecording))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/recordings/{rid}/flags", s.requireSession(s.handleGetRecordingFlags))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/recordings/{rid}/traces", s.requireSession(s.handleGetRecordingTraces))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels/{fid}/steps/{step}/sessions", s.requireSession(s.handleFunnelStepSessions))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flows/sessions", s.requireSession(s.handleFlowPageSessions))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/recording-settings", s.requireSession(s.handleGetProjectRecordingSettings))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}/recording-settings", s.requireSession(s.handleUpdateProjectRecordingSettings))

	// SDK recording config (API key auth, public — returns effective config for the project)
	s.mux.Handle("GET /api/v1/recording-config", s.limit(s.eventsLimiter, http.HandlerFunc(s.handleGetRecordingConfig)))

	// API keys
	s.mux.HandleFunc("GET /api/v1/apikeys", s.requireSession(s.handleListAPIKeys))
	s.mux.HandleFunc("POST /api/v1/apikeys", s.requireSession(s.handleCreateAPIKey))
	s.mux.HandleFunc("DELETE /api/v1/apikeys/{kid}", s.requireSession(s.handleDeleteAPIKey))

	// Project approval
	s.mux.HandleFunc("POST /api/v1/projects/{id}/approve", s.requireSession(s.handleApproveProject))

	// Segments
	s.mux.HandleFunc("GET /api/v1/projects/{id}/segments", s.requireSession(s.handleListSegments))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/segments", s.requireSession(s.handleCreateSegment))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}/segments/{sid}", s.requireSession(s.handleUpdateSegment))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}/segments/{sid}", s.requireSession(s.handleDeleteSegment))

	// Project integration health
	s.mux.HandleFunc("GET /api/v1/projects/{id}/health", s.requireSession(s.handleGetProjectHealth))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/health/reset", s.requireSession(s.handleResetProjectHealth))

	// Instance settings
	s.mux.HandleFunc("GET /api/v1/instance-settings", s.requireSession(s.handleGetInstanceSettings))
	s.mux.HandleFunc("PUT /api/v1/instance-settings", s.requireSession(s.handlePutInstanceSettings))

	// Geo anonymization
	s.mux.HandleFunc("POST /api/v1/admin/anonymize-geo", s.requireSession(s.handleAnonymizeGeo))

	// MCP endpoint + protected-resource metadata (only with OIDC configured).
	s.registerMCPRoutes()
}

// ServeHTTP adds CORS headers and dispatches to the router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The MCP protected-resource metadata is public discovery data with its own
	// open CORS handling (preflight included). It must skip the credentialed
	// dashboard CORS below and the doubled-prefix redirect, which would read
	// its "/api/v1/" suffix as a misroute and 308 it to /api/v1/mcp. Without
	// MCP configured the mux has no route for it and answers 404.
	if r.URL.Path == mcpResourceMetaPath {
		tracing.Middleware(requestLogger(s.securityMW(s.mux))).ServeHTTP(w, r)
		return
	}
	s.setCORSHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if redirectVanityHost(w, r) {
		return
	}
	// A client that concatenated its base URL with the ingest path posts to
	// /api/v1/events/api/v1/events and gets a silent 404. Production sees ~189
	// of those a week against ~110 correct posts, so the misconfigured client
	// alone loses more events than every working client sends. Send it to the
	// right place instead of dropping it, and say so in the log — a 308
	// preserves the method and body, so the event survives the round trip.
	if canonical, ok := canonicalAPIPath(r.URL.Path); ok {
		logMisroutedRequest(r, canonical)
		metrics.MisroutedRequests.WithLabelValues(canonical, "redirected").Inc()
		target := canonical
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
		return
	}

	// Apply a reasonable body limit to all routes. Ingest and recording-chunk
	// endpoints have their own per-handler limits because their payloads can
	// legitimately exceed the default cap (rrweb full snapshots are routinely
	// several MB, well above 256 KiB).
	if r.Body != nil && r.URL.Path != "/api/v1/events" && r.URL.Path != "/api/v1/recordings/chunk" {
		r.Body = http.MaxBytesReader(w, r.Body, 256<<10) // 256 KiB
	}
	// Apply middleware: requestLogger (innermost) → securityHeaders → tracing → dispatch.
	tracing.Middleware(requestLogger(s.securityMW(http.HandlerFunc(s.dispatch)))).ServeHTTP(w, r)
}

// dispatch serves r from this process, or forwards it to the writer when this
// is a reader and r writes the database.
func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	if s.forwarder != nil && forwardsToWriter(r) {
		s.forward(w, r)
		return
	}
	s.mux.ServeHTTP(w, r)
}

// accessTokenSkew is subtracted from access_expires_at so a token is refreshed
// slightly BEFORE it actually expires — a request must never go out the door
// authorized by a token that dies mid-flight.
const accessTokenSkew = 30 * time.Second

func isMutating(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete || method == http.MethodPatch
}
