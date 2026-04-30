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
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// Pinger is satisfied by any type with a Ping method (e.g. *repository.Store).
type Pinger interface {
	Ping(ctx context.Context) error
}

// Server is the main HTTP API server.
type Server struct {
	mux            *http.ServeMux
	db             Pinger
	projects       service.Projects
	funnels        service.Funnels
	abtests        service.ABTests
	events         service.Events
	sessions       service.Sessions
	apikeys        service.APIKeys
	ingest         *ingest.Handler
	userAuth       *auth.UserAuthenticator
	sessionManager *auth.SessionManager
	allowedOrigins []string
	sessionSecret  string
	publicURL      string
	metricsToken   string

	loginLimiter  *rateLimiter
	eventsLimiter *rateLimiter
	apiLimiter    *rateLimiter // 300 req/min, burst 60 — for all session-authenticated endpoints
}

// NewServer creates the API server and registers all routes.
func NewServer(
	ingestHandler *ingest.Handler,
	projects service.Projects,
	funnels service.Funnels,
	abtests service.ABTests,
	events service.Events,
	sessions service.Sessions,
	apikeys service.APIKeys,
	userAuth *auth.UserAuthenticator,
	sessionManager *auth.SessionManager,
	allowedOrigins []string,
	sessionSecret string,
	publicURL string,
	loginRatePerMinute float64,
	loginRateBurst float64,
	apiRatePerMinute float64,
	apiRateBurst float64,
	db Pinger,
) *Server {
	s := &Server{
		mux:            http.NewServeMux(),
		db:             db,
		projects:       projects,
		funnels:        funnels,
		abtests:        abtests,
		events:         events,
		sessions:       sessions,
		apikeys:        apikeys,
		ingest:         ingestHandler,
		userAuth:       userAuth,
		sessionManager: sessionManager,
		allowedOrigins: allowedOrigins,
		sessionSecret:  sessionSecret,
		publicURL:      publicURL,
		loginLimiter:   newRateLimiter(loginRatePerMinute, loginRateBurst),
		eventsLimiter:  newRateLimiter(500, 100),
		apiLimiter:     newRateLimiter(apiRatePerMinute, apiRateBurst),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Metrics — open when no token configured, bearer-protected otherwise.
	s.mux.Handle("GET /metrics", s.metricsHandler())

	// Public
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/setup/{slug}", s.handleSetup)

	// Ingest (API key required)
	s.mux.Handle("POST /api/v1/events", s.eventsLimiter.middleware(s.ingest))

	// Auth
	s.mux.Handle("POST /api/v1/login", s.loginLimiter.middleware(http.HandlerFunc(s.handleLogin)))
	s.mux.HandleFunc("POST /api/v1/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/v1/me", s.requireSession(s.handleMe))

	// Projects
	s.mux.HandleFunc("GET /api/v1/projects", s.requireSession(s.handleListProjects))
	s.mux.HandleFunc("POST /api/v1/projects", s.requireSession(s.handleCreateProject))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}", s.requireSession(s.handleUpdateProject))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}", s.requireSession(s.handleDeleteProject))

	// Dashboard & analytics (session required)
	s.mux.HandleFunc("GET /api/v1/projects/{id}/dashboard", s.requireSession(s.handleDashboard))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/events", s.requireSession(s.handleListEvents))

	// Funnels
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels", s.requireSession(s.handleListFunnels))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/funnels", s.requireSession(s.handleCreateFunnel))
	s.mux.HandleFunc("PUT /api/v1/projects/{id}/funnels/{fid}", s.requireSession(s.handleUpdateFunnel))
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}/funnels/{fid}", s.requireSession(s.handleDeleteFunnel))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels/{fid}/analysis", s.requireSession(s.handleFunnelAnalysis))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/funnels/{fid}/segments", s.requireSession(s.handleFunnelSegments))

	// A/B Tests
	s.mux.HandleFunc("GET /api/v1/projects/{id}/abtests", s.requireSession(s.handleListABTests))
	s.mux.HandleFunc("POST /api/v1/projects/{id}/abtests", s.requireSession(s.handleCreateABTest))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/abtests/{abid}/analysis", s.requireSession(s.handleABTestAnalysis))

	// Sessions
	s.mux.HandleFunc("GET /api/v1/projects/{id}/sessions", s.requireSession(s.handleListSessions))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/sessions/active", s.requireSession(s.handleActiveSessionCount))

	// API keys
	s.mux.HandleFunc("GET /api/v1/apikeys", s.requireSession(s.handleListAPIKeys))
	s.mux.HandleFunc("POST /api/v1/apikeys", s.requireSession(s.handleCreateAPIKey))
	s.mux.HandleFunc("DELETE /api/v1/apikeys/{kid}", s.requireSession(s.handleDeleteAPIKey))

	// Project approval
	s.mux.HandleFunc("POST /api/v1/projects/{id}/approve", s.requireSession(s.handleApproveProject))
}

// ServeHTTP adds CORS headers and dispatches to the router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.setCORSHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Apply a reasonable body limit to all routes (ingest has its own per-handler limit).
	if r.Body != nil && r.URL.Path != "/api/v1/events" {
		r.Body = http.MaxBytesReader(w, r.Body, 256<<10) // 256 KiB
	}
	// Apply middleware: requestLogger (innermost) → securityHeaders → dispatch.
	requestLogger(securityHeaders(s.mux)).ServeHTTP(w, r)
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

	if len(s.allowedOrigins) == 0 {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		for _, allowed := range s.allowedOrigins {
			if allowed == "*" || strings.EqualFold(allowed, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				break
			}
		}
	}

	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+auth.HeaderAPIKey+", X-FunnelBarn-CSRF")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// requireSession wraps a handler to enforce session cookie authentication.
// It also applies the apiLimiter rate limit for authenticated endpoints.
func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.sessionManager == nil || !s.userAuth.Enabled() {
			// No auth configured — apply rate limit and allow through.
			if !s.apiLimiter.allow(clientIP(r)) {
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

		if !s.apiLimiter.allow(clientIP(r)) {
			jsonError(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		slog.Debug("session valid", "username", username)
		next(w, r)
	}
}

// mapServiceError maps domain/service errors to appropriate HTTP status codes.
// It never leaks internal error details to the client.
func mapServiceError(w http.ResponseWriter, err error, op string) {
	switch {
	case domain.IsNotFound(err):
		jsonError(w, "not found", http.StatusNotFound)
	case domain.IsConflict(err):
		jsonError(w, "already exists", http.StatusConflict)
	case domain.IsValidation(err):
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			jsonError(w, ve.Error(), http.StatusUnprocessableEntity)
		} else {
			jsonError(w, "invalid request", http.StatusUnprocessableEntity)
		}
	default:
		slog.Error("unexpected service error", "op", op, "err", err)
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
