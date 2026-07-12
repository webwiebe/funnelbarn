package api

import (
	"net/http"
)

// handleOIDCLoggedOut is the landing endpoint for IAMBarn's RP-initiated logout
// (/oauth2/end-session). IAMBarn ends its own session, then redirects the
// browser here; we destroy the local FunnelBarn session so both are gone, then
// send the user to the login page. Registered as the client's
// post_logout_redirect_uri — the path is unchanged so the IdP allowlist stays
// valid.
// GET /api/v1/auth/oidc/logged-out
func (s *Server) handleOIDCLoggedOut(w http.ResponseWriter, r *http.Request) {
	// The IdP session is already ended (this redirect IS the end-session
	// round-trip), so the returned end-session URL is intentionally unused.
	s.clearSession(w, r)
	http.Redirect(w, r, "/login", http.StatusFound)
}
