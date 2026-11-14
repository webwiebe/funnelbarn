package api

// Admin-only session routes. Every signed-in session can read and write every
// project; issuing and deleting API keys also needs the admin check below
// (#439).

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

const sessionAdminKey contextKey = "session_admin"

// withSessionAdmin records whether the validated session may manage
// credentials. Set by requireSession.
func withSessionAdmin(ctx context.Context, admin bool) context.Context {
	return context.WithValue(ctx, sessionAdminKey, admin)
}

// sessionAdmin reports whether the request's session may manage credentials.
// A request that never passed requireSession is not an admin.
func sessionAdmin(ctx context.Context) bool {
	admin, _ := ctx.Value(sessionAdminKey).(bool)
	return admin
}

// sessionIsAdmin decides admin status for a validated session. Local sessions
// (the env-var admin and DB users) are set up by whoever runs the server, so
// they are admins. An OIDC session is an admin while its stored claims pass
// the sign-in role check: owner, organization_admin or operator in the
// required organization. claims_json is the sign-in snapshot, replaced when a
// refresh returns new claims (#481 makes that happen with IAMBarn, whose
// refresh response carries no id_token). Groups do not count, for the reason
// given on OIDCClient.Allowed.
func (s *Server) sessionIsAdmin(ws domain.WebSession) bool {
	if ws.AuthMethod != "oidc" {
		return true
	}
	if s.oidc == nil || ws.ClaimsJSON == "" {
		return false
	}
	var claims auth.OIDCClaims
	if err := json.Unmarshal([]byte(ws.ClaimsJSON), &claims); err != nil {
		return false
	}
	return s.oidc.Allowed(claims)
}

// requireAdminSession is requireSession plus the admin check. A valid session
// without admin status gets 403.
func (s *Server) requireAdminSession(next http.HandlerFunc) http.HandlerFunc {
	return s.requireSession(func(w http.ResponseWriter, r *http.Request) {
		if !sessionAdmin(r.Context()) {
			slog.WarnContext(r.Context(), "admin route: session lacks an admin role",
				"path", r.URL.Path,
				"username", sessionUser(r.Context()),
				"request_id", RequestIDFromContext(r.Context()),
			)
			jsonError(w, "forbidden: managing API keys needs the owner, organization_admin or operator role", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}
