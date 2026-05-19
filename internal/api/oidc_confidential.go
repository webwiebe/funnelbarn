package api

import (
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

const (
	oidcConfStateCookie = "funnelbarn_oidc_state"
	oidcConfNonceCookie = "funnelbarn_oidc_nonce"
	oidcConfCookieTTL   = 10 * time.Minute
)

// handleOIDCConfidentialLogin starts the OIDC authorization-code flow by
// redirecting the browser to the issuer's authorize endpoint. State + nonce
// are stored in short-lived HttpOnly cookies and checked on the callback.
//
// This is the bugbarn-style confidential-client flow gated by
// FUNNELBARN_OIDC_* env vars. It is independent of the existing IAMBarn PKCE
// flow on /api/v1/auth/oidc/* (which is feature-flag gated).
func (s *Server) handleOIDCConfidentialLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		jsonError(w, "oidc not configured", http.StatusNotFound)
		return
	}
	state := oidcConfRandomToken()
	nonce := oidcConfRandomToken()
	authURL, err := s.oidc.AuthorizeURL(state, nonce)
	if err != nil {
		slog.WarnContext(r.Context(), "oidc: build authorize url", "error", err)
		jsonError(w, "oidc unavailable", http.StatusServiceUnavailable)
		return
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, oidcConfShortLivedCookie(oidcConfStateCookie, state, secure))
	http.SetCookie(w, oidcConfShortLivedCookie(oidcConfNonceCookie, nonce, secure))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCConfidentialCallback handles the redirect back from the issuer. On
// success it issues a local session cookie that authenticates the browser for
// the SPA.
func (s *Server) handleOIDCConfidentialCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		jsonError(w, "oidc not configured", http.StatusNotFound)
		return
	}
	stateCookie, err := r.Cookie(oidcConfStateCookie)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		slog.WarnContext(r.Context(), "oidc: state mismatch")
		jsonError(w, "oidc state mismatch", http.StatusBadRequest)
		return
	}
	nonceCookie, err := r.Cookie(oidcConfNonceCookie)
	if err != nil || nonceCookie.Value == "" {
		jsonError(w, "oidc nonce missing", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		jsonError(w, "oidc code missing", http.StatusBadRequest)
		return
	}
	claims, err := s.oidc.Exchange(r.Context(), code, nonceCookie.Value)
	if err != nil {
		slog.WarnContext(r.Context(), "oidc: exchange failed", "error", err)
		jsonError(w, "oidc exchange failed", http.StatusUnauthorized)
		return
	}
	if !s.oidc.Allowed(claims) {
		slog.WarnContext(r.Context(), "oidc: access denied",
			"sub", claims.Subject, "groups", claims.Groups, "roles", claims.Roles)
		jsonError(w, "access denied: user is not a member of the required group", http.StatusForbidden)
		return
	}
	if s.sessionManager == nil {
		jsonError(w, "session unavailable", http.StatusServiceUnavailable)
		return
	}
	username := claims.PreferredName()
	if username == "" {
		username = "oidc-user"
	}
	token, expires, err := s.sessionManager.Create(username)
	if err != nil {
		jsonError(w, "session unavailable", http.StatusServiceUnavailable)
		return
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, auth.SessionCookie(token, expires, secure))
	http.SetCookie(w, auth.CSRFCookie(token, expires, secure))
	// Non-HttpOnly hint so the SPA can show OIDC-specific UI (e.g. the
	// IAMBarn profile link) only for sessions that actually came from
	// iambarn. Same expiry as the session.
	http.SetCookie(w, &http.Cookie{
		Name:     "funnelbarn_auth_method",
		Value:    "oidc",
		Path:     "/",
		Expires:  expires,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	// Clear the short-lived state/nonce cookies.
	http.SetCookie(w, oidcConfShortLivedCookie(oidcConfStateCookie, "", secure))
	http.SetCookie(w, oidcConfShortLivedCookie(oidcConfNonceCookie, "", secure))
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func oidcConfShortLivedCookie(name, value string, secure bool) *http.Cookie {
	maxAge := int(oidcConfCookieTTL.Seconds())
	if value == "" {
		maxAge = -1
	}
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}
}

func oidcConfRandomToken() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
