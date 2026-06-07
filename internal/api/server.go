package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/iambarn"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
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
	Sessions       service.Sessions
	APIKeys        service.APIKeys
	Widgets        service.Widgets
	UserAuth       *auth.UserAuthenticator
	SessionManager *auth.SessionManager
	AllowedOrigins []string
	SessionSecret  string
	PublicURL      string
	DB             Pinger
	Version        string
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

	IAMBarnProvider    *iambarn.Provider
	IAMBarnUsers       IAMBarnUserRepo
	IAMBarnFlagProject string // dogfood project slug to read the iambarn-enabled flag from

	// OIDC, when non-nil, enables the bugbarn-style confidential-client OIDC
	// login flow at /api/v1/oidc/{login,callback}. Independent of IAMBarnProvider.
	OIDC *auth.OIDCClient

	InstanceSettings InstanceSettingsRepo
	GeoAnonymizer    GeoAnonymizer
	Segments         service.Segments
	Distributions    DistributionRepo
	Recordings       service.Recordings
}

// DistributionRepo provides session field distribution data.
type DistributionRepo interface {
	SessionDistributions(ctx context.Context, projectID string) (map[string][]repository.DistributionEntry, error)
}

// InstanceSettingsRepo is the narrow interface for reading/writing instance-level settings.
type InstanceSettingsRepo interface {
	GetAllInstanceSettings(ctx context.Context) (map[string]string, error)
	SetInstanceSetting(ctx context.Context, key, value string) error
}

// GeoAnonymizer can zero out geo fields on sessions.
type GeoAnonymizer interface {
	AnonymizeSessionGeo(ctx context.Context, sessionID string) error
	AnonymizeSessionsByIP(ctx context.Context, ip string) (int64, error)
}

// Server is the main HTTP API server.
type Server struct {
	mux                *http.ServeMux
	db                 Pinger
	projects           service.Projects
	funnels            service.Funnels
	abtests            service.ABTests
	flags              service.Flags
	events             service.Events
	sessions           service.Sessions
	apikeys            service.APIKeys
	widgets            service.Widgets
	ingest             *ingest.Handler
	userAuth           *auth.UserAuthenticator
	sessionManager     *auth.SessionManager
	allowedOrigins     []string
	sessionSecret      string
	publicURL          string
	metricsToken       string
	version            string
	bugbarnEndpoint    string
	bugbarnIngestKey   string
	bugbarnProject     string
	dogfoodAPIKey      string
	dogfoodProject     string
	trustedProxies     []string
	iambarnProvider    *iambarn.Provider
	iambarnUsers       IAMBarnUserRepo
	iambarnFlagProject string
	oidc               *auth.OIDCClient
	instanceSettings   InstanceSettingsRepo
	geoAnonymizer      GeoAnonymizer
	segments           service.Segments
	distributions      DistributionRepo
	recordings         service.Recordings

	loginLimiter  *rateLimiter
	eventsLimiter *rateLimiter
	apiLimiter    *rateLimiter
	setupLimiter  *rateLimiter
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

	s := &Server{
		mux:                http.NewServeMux(),
		db:                 cfg.DB,
		version:            cfg.Version,
		projects:           cfg.Projects,
		funnels:            cfg.Funnels,
		abtests:            cfg.ABTests,
		flags:              cfg.Flags,
		events:             cfg.Events,
		sessions:           cfg.Sessions,
		apikeys:            cfg.APIKeys,
		widgets:            cfg.Widgets,
		ingest:             cfg.Ingest,
		userAuth:           cfg.UserAuth,
		sessionManager:     cfg.SessionManager,
		allowedOrigins:     cfg.AllowedOrigins,
		sessionSecret:      cfg.SessionSecret,
		publicURL:          cfg.PublicURL,
		bugbarnEndpoint:    cfg.BugbarnEndpoint,
		bugbarnIngestKey:   cfg.BugbarnIngestKey,
		bugbarnProject:     cfg.BugbarnProject,
		dogfoodAPIKey:      cfg.DogfoodAPIKey,
		dogfoodProject:     cfg.DogfoodProject,
		trustedProxies:     cfg.TrustedProxies,
		loginLimiter:       newRateLimiter(cfg.LoginRatePerMinute, cfg.LoginRateBurst),
		eventsLimiter:      newRateLimiter(cfg.IngestRatePerMinute, cfg.IngestRateBurst),
		apiLimiter:         newRateLimiter(cfg.APIRatePerMinute, cfg.APIRateBurst),
		setupLimiter:       newRateLimiter(setupRate, setupBurst),
		iambarnProvider:    cfg.IAMBarnProvider,
		iambarnUsers:       cfg.IAMBarnUsers,
		iambarnFlagProject: cfg.IAMBarnFlagProject,
		oidc:               cfg.OIDC,
		instanceSettings:   cfg.InstanceSettings,
		geoAnonymizer:      cfg.GeoAnonymizer,
		segments:           cfg.Segments,
		distributions:      cfg.Distributions,
		recordings:         cfg.Recordings,
	}
	s.registerRoutes()
	return s
}

// StartCleanup begins periodic rate limiter cleanup goroutines.
// Call this once with a context that is cancelled on shutdown.
func (s *Server) StartCleanup(ctx context.Context) {
	s.loginLimiter.startCleanup(ctx)
	s.eventsLimiter.startCleanup(ctx)
	s.apiLimiter.startCleanup(ctx)
	s.setupLimiter.startCleanup(ctx)
}

func (s *Server) registerRoutes() {
	// Metrics — open when no token configured, bearer-protected otherwise.
	s.mux.Handle("GET /metrics", s.metricsHandler())

	// Public
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.Handle("GET /api/v1/setup/{slug}", s.setupLimiter.middleware(http.HandlerFunc(s.handleSetup)))
	s.mux.HandleFunc("GET /api/v1/client-config", s.handleClientConfig)
	s.mux.HandleFunc("GET /.well-known/iambarn-theme.json", s.handleThemeManifest)

	// Ingest (API key required)
	s.mux.Handle("POST /api/v1/events", s.eventsLimiter.middleware(s.ingest))
	s.mux.Handle("POST /api/v1/recordings/chunk", s.eventsLimiter.middleware(http.HandlerFunc(s.handleIngestRecordingChunk)))

	// Auth
	s.mux.Handle("POST /api/v1/login", s.loginLimiter.middleware(http.HandlerFunc(s.handleLogin)))
	s.mux.HandleFunc("POST /api/v1/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/v1/me", s.requireSession(s.handleMe))

	// OIDC (gated by the iambarn-enabled feature flag in handlers)
	s.mux.Handle("GET /api/v1/auth/oidc/login", s.loginLimiter.middleware(http.HandlerFunc(s.handleOIDCLogin)))
	s.mux.HandleFunc("GET /api/v1/auth/oidc/callback", s.handleOIDCCallback)

	// OIDC confidential-client flow (gated by FUNNELBARN_OIDC_* env vars).
	// Independent of the IAMBarn PKCE flow above; both can coexist.
	s.mux.Handle("GET /api/v1/oidc/login", s.loginLimiter.middleware(http.HandlerFunc(s.handleOIDCConfidentialLogin)))
	s.mux.HandleFunc("GET /api/v1/oidc/callback", s.handleOIDCConfidentialCallback)

	// Projects
	s.mux.HandleFunc("GET /api/v1/projects", s.requireSession(s.handleListProjects))
	s.mux.HandleFunc("POST /api/v1/projects", s.requireSession(s.handleCreateProject))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}", s.requireSession(s.handleUpdateProject))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}", s.requireSession(s.handleDeleteProject))

	// Dashboard & analytics (session required)
	s.mux.HandleFunc("GET /api/v1/projects/{id}/dashboard", s.requireSession(s.handleDashboard))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flows", s.requireSession(s.handlePageFlows))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/events", s.requireSession(s.handleListEvents))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/event-names", s.requireSession(s.handleEventNames))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/event-properties", s.requireSession(s.handleEventProperties))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/event-property-values", s.requireSession(s.handleEventPropertyValues))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/environments", s.requireSession(s.handleEnvironments))

	// Funnels
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels", s.requireSession(s.handleListFunnels))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/funnels", s.requireSession(s.handleCreateFunnel))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}/funnels/{fid}", s.requireSession(s.handleUpdateFunnel))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}/funnels/{fid}", s.requireSession(s.handleDeleteFunnel))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels/{fid}/analysis", s.requireSession(s.handleFunnelAnalysis))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels/{fid}/segments", s.requireSession(s.handleFunnelSegments))

	// Feature Flags (session required)
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flags", s.requireSession(s.handleListFlags))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/flags", s.requireSession(s.handleCreateFlag))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flags/{fid}", s.requireSession(s.handleGetFlag))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}/flags/{fid}", s.requireSession(s.handleUpdateFlag))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}/flags/{fid}", s.requireSession(s.handleDeleteFlag))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flags/{fid}/analysis", s.requireSession(s.handleFlagAnalysis))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flags/context-keys", s.requireSession(s.handleFlagContextKeys))
	// Dogfooding playground — same flag-eval logic as the customer endpoint
	// but session-authed so the dashboard can call it without an API key in the browser.
	s.mux.HandleFunc("POST /api/v1/projects/{id}/flags/evaluate", s.requireSession(s.handlePlaygroundEvaluateFlag))

	// Flag evaluation (API key required, like ingest)
	s.mux.Handle("POST /api/v1/evaluate", s.eventsLimiter.middleware(http.HandlerFunc(s.handleEvaluateFlag)))

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

	// Recordings
	s.mux.HandleFunc("GET /api/v1/projects/{id}/recordings", s.requireSession(s.handleListRecordings))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/recordings/{rid}/chunks/{index}", s.requireSession(s.handleGetRecordingChunk))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/recordings/{rid}/flags", s.requireSession(s.handleGetRecordingFlags))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels/{fid}/steps/{step}/sessions", s.requireSession(s.handleFunnelStepSessions))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flows/sessions", s.requireSession(s.handleFlowPageSessions))

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

	// Instance settings
	s.mux.HandleFunc("GET /api/v1/instance-settings", s.requireSession(s.handleGetInstanceSettings))
	s.mux.HandleFunc("PUT /api/v1/instance-settings", s.requireSession(s.handlePutInstanceSettings))

	// Geo anonymization
	s.mux.HandleFunc("POST /api/v1/admin/anonymize-geo", s.requireSession(s.handleAnonymizeGeo))
}

// ServeHTTP adds CORS headers and dispatches to the router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.setCORSHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Redirect GET / on f.* subdomains back to their root domain.
	// e.g. f.example.com → https://example.com
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		host := r.Host
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
		if strings.HasPrefix(host, "f.") {
			http.Redirect(w, r, "https://"+strings.TrimPrefix(host, "f."), http.StatusMovedPermanently)
			return
		}
	}
	// Apply a reasonable body limit to all routes (ingest has its own per-handler limit).
	if r.Body != nil && r.URL.Path != "/api/v1/events" {
		r.Body = http.MaxBytesReader(w, r.Body, 256<<10) // 256 KiB
	}
	// Apply middleware: requestLogger (innermost) → securityHeaders → tracing → dispatch.
	tracing.Middleware(requestLogger(securityHeaders(s.mux))).ServeHTTP(w, r)
}

// SetMetricsToken configures a bearer token required to access /metrics.
// Call this after NewServer; an empty string means open access (backwards compatible).
func (s *Server) SetMetricsToken(token string) {
	s.metricsToken = token
	// Re-register the metrics route with the updated token.
	s.mux = http.NewServeMux()
	s.registerRoutes()
}

// metricsHandler returns the Prometheus metrics handler, optionally
// protected by a bearer token when metricsToken is non-empty.
func (s *Server) metricsHandler() http.Handler {
	promH := promhttp.Handler()
	if s.metricsToken == "" {
		return promH // no token configured — open access (backwards compatible)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+s.metricsToken {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		promH.ServeHTTP(w, r)
	})
}

func (s *Server) setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}

	open := len(s.allowedOrigins) == 0 || (len(s.allowedOrigins) == 1 && s.allowedOrigins[0] == "")
	allowed := open
	if !allowed {
		for _, o := range s.allowedOrigins {
			if o == "*" || strings.EqualFold(o, origin) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return
	}

	// Always reflect the origin (not "*") so credentialed requests work — browsers
	// reject Allow-Credentials: true combined with Allow-Origin: *.
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", "Origin")

	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	// Echo back any headers the client asked for in preflight; fall back to a
	// permissive default list so trackers using non-standard headers aren't blocked.
	reqHeaders := r.Header.Get("Access-Control-Request-Headers")
	if reqHeaders != "" {
		w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		w.Header().Add("Vary", "Access-Control-Request-Headers")
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, "+auth.HeaderAPIKey+", X-FunnelBarn-CSRF, X-Requested-With")
	}
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// requireSession wraps a handler to enforce session cookie authentication.
// It also applies the apiLimiter rate limit and CSRF validation on mutating methods.
func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authConfigured := s.userAuth.Enabled() || s.iambarnProvider != nil
		if s.sessionManager == nil || !authConfigured {
			if !s.apiLimiter.allow(s.clientIP(r)) {
				jsonError(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next(w, r)
			return
		}

		cookie, err := r.Cookie("funnelbarn_session")
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		username, ok := s.sessionManager.Valid(cookie.Value)
		if !ok {
			jsonError(w, "session expired", http.StatusUnauthorized)
			return
		}

		if isMutating(r.Method) {
			expected := auth.CSRFToken(cookie.Value)
			got := r.Header.Get("X-FunnelBarn-CSRF")
			if got == "" || got != expected {
				jsonError(w, "csrf token invalid", http.StatusForbidden)
				return
			}
		}

		if !s.apiLimiter.allow(s.clientIP(r)) {
			jsonError(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		slog.Debug("session valid", "username", username)
		next(w, r)
	}
}

// iambarnFlagEnabled evaluates the "iambarn-enabled" feature flag in the
// dogfood project. evalCtx is passed as-is to the flag evaluator so that
// targeting rules (e.g. user_agent contains "Chrome") can act on request data.
func (s *Server) iambarnFlagEnabled(ctx context.Context, evalCtx map[string]any) bool {
	if s.iambarnProvider == nil || s.iambarnFlagProject == "" {
		return false
	}
	proj, err := s.projects.GetProjectBySlug(ctx, s.iambarnFlagProject)
	if err != nil {
		return false
	}
	result, err := s.flags.EvaluateFlag(ctx, proj.ID, "iambarn-enabled", evalCtx)
	if err != nil {
		return false
	}
	if result.Variant == "on" {
		return true
	}
	v, ok := result.Value.(bool)
	return ok && v
}

func isMutating(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete || method == http.MethodPatch
}

// mapServiceError maps domain/service errors to appropriate HTTP status codes.
// It never leaks internal error details to the client.
// Expected errors (not-found, conflict, validation) are logged at Warn level with handled=true.
// Unexpected errors are logged at Error level with handled=false.
func mapServiceError(w http.ResponseWriter, err error, op string) {
	switch {
	case domain.IsNotFound(err):
		slog.Warn("service error: not found", "op", op, "error", err, "handled", true)
		jsonError(w, "not found", http.StatusNotFound)
	case domain.IsConflict(err):
		slog.Warn("service error: conflict", "op", op, "error", err, "handled", true)
		jsonError(w, "already exists", http.StatusConflict)
	case domain.IsValidation(err):
		slog.Warn("service error: validation", "op", op, "error", err, "handled", true)
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			jsonError(w, ve.Error(), http.StatusUnprocessableEntity)
		} else {
			jsonError(w, "invalid request", http.StatusUnprocessableEntity)
		}
	default:
		slog.Error("unexpected service error", "op", op, "error", err, "handled", false)
		jsonError(w, "internal server error", http.StatusInternalServerError)
	}
}

// --------------------------------------------------------------------------
// Helper utilities
// --------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", "err", err)
	}
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
