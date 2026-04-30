package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/storage"
)

// Server is the main HTTP API server.
type Server struct {
	mux            *http.ServeMux
	store          *storage.Store
	ingest         *ingest.Handler
	userAuth       *auth.UserAuthenticator
	sessionManager *auth.SessionManager
	allowedOrigins []string
	sessionSecret  string
	publicURL      string

	loginLimiter  *rateLimiter
	eventsLimiter *rateLimiter
}

// NewServer creates the API server and registers all routes.
func NewServer(
	ingestHandler *ingest.Handler,
	store *storage.Store,
	userAuth *auth.UserAuthenticator,
	sessionManager *auth.SessionManager,
	allowedOrigins []string,
	sessionSecret string,
	publicURL string,
) *Server {
	s := &Server{
		mux:            http.NewServeMux(),
		store:          store,
		ingest:         ingestHandler,
		userAuth:       userAuth,
		sessionManager: sessionManager,
		allowedOrigins: allowedOrigins,
		sessionSecret:  sessionSecret,
		publicURL:      publicURL,
		loginLimiter:   newRateLimiter(5, 5),
		eventsLimiter:  newRateLimiter(500, 100),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Metrics (no auth required)
	s.mux.Handle("GET /metrics", promhttp.Handler())

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
	// Apply middleware: requestLogger (innermost) → securityHeaders → dispatch.
	requestLogger(securityHeaders(s.mux)).ServeHTTP(w, r)
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
func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.sessionManager == nil || !s.userAuth.Enabled() {
			// No auth configured — allow through.
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

		slog.Debug("session valid", "username", username)
		next(w, r)
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
