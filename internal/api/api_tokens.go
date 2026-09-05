package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// Flag CRUD and analytics reads used to be reachable only with a browser
// session, so no service could read or toggle a flag, and no scheduled job
// could publish a funnel readout — an operator surface had to send people to
// the FunnelBarn dashboard to flip a switch or copy numbers out by hand.
// Splitting one deliberate action across two systems is how "we thought it was
// off" happens.
//
// These routes therefore accept a project-scoped API token as well as a
// session. The token is bound to one project, hashed at rest like every other
// API key, issued and revoked from the dashboard, and carries a last-used
// timestamp.

// tokenProjectCtxKey carries the project a request was authenticated for when
// it came in on an API token rather than a session.
type tokenProjectCtxKey struct{}

// tokenProjectFromContext returns the project an API token authenticated this
// request for. It is empty for session-authenticated requests, which is the
// difference between "show me my project" and "show me every project".
func tokenProjectFromContext(ctx context.Context) string {
	id, _ := ctx.Value(tokenProjectCtxKey{}).(string)
	return id
}

// requireSessionOrToken serves next to either a dashboard session or a
// project-scoped API token holding at least wantScope.
//
// When the request carries an API key it is authenticated as a token and
// nothing else: falling back to the session path on a bad token would turn a
// revoked credential into a silent success for whoever happened to be logged
// in on the same browser.
func (s *Server) requireSessionOrToken(wantScope string, next http.HandlerFunc) http.HandlerFunc {
	viaSession := s.requireSession(next)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(auth.HeaderAPIKey) == "" {
			viaSession(w, r)
			return
		}
		s.serveScopedToken(w, r, wantScope, next)
	}
}

func (s *Server) serveScopedToken(w http.ResponseWriter, r *http.Request, wantScope string, next http.HandlerFunc) {
	projectID, scope, ok := s.ingest.APIKeyProjectScope(r)
	if !ok {
		slog.WarnContext(r.Context(), "api token: unauthorized",
			"path", r.URL.Path,
			"request_id", RequestIDFromContext(r.Context()),
		)
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// The instance-wide env-var key resolves with an empty project ID, which
	// would make it a master key for every project's flags and analytics. A
	// scoped token must name the project it is allowed to touch.
	if projectID == "" {
		slog.WarnContext(r.Context(), "api token: instance-wide key cannot be used for scoped access",
			"path", r.URL.Path,
			"request_id", RequestIDFromContext(r.Context()),
		)
		jsonError(w, "this endpoint requires a project-scoped API key from the project settings page, not the global FUNNELBARN_API_KEY", http.StatusUnauthorized)
		return
	}

	// The whole point of the scoping: rapid-root's scherpstel token must not be
	// able to touch places.
	if routeProject := r.PathValue("id"); routeProject != "" && routeProject != projectID {
		jsonError(w, "api key is not scoped to this project", http.StatusForbidden)
		return
	}

	if !scopeAllows(scope, wantScope) {
		jsonError(w, "api key scope "+scope+" cannot "+wantScope, http.StatusForbidden)
		return
	}

	if !s.apiLimiter.allow(s.clientIP(r)) {
		jsonError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	// No CSRF check: this request is authenticated by a header the browser
	// never attaches on its own, so there is nothing to forge across origins.
	//
	// Routes with no {id} in the path (GET /api/v1/projects) cannot be checked
	// against the token's project above, so they read it back off the context
	// and narrow their own answer to it instead.
	next(w, r.WithContext(context.WithValue(r.Context(), tokenProjectCtxKey{}, projectID)))
}

// scopeAllows reports whether a key's scope covers the access a route needs.
// Read is implied by write; "ingest" covers neither, and an unknown scope
// covers nothing.
func scopeAllows(have, want string) bool {
	if have == repository.APIKeyScopeFull {
		return true
	}
	switch want {
	case repository.APIKeyScopeFlagsRead:
		return have == repository.APIKeyScopeFlagsRead || have == repository.APIKeyScopeFlagsWrite
	case repository.APIKeyScopeFlagsWrite:
		return have == repository.APIKeyScopeFlagsWrite
	case repository.APIKeyScopeAnalyticsRead:
		// Deliberately not implied by flags:write — a token that can toggle a
		// flag has no business reading the whole event stream.
		return have == repository.APIKeyScopeAnalyticsRead
	}
	return false
}
