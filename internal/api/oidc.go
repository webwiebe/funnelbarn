package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/iambarn"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// IAMBarnUserRepo is the storage interface needed by the OIDC callback handler.
type IAMBarnUserRepo interface {
	FindUserByIAMBarnSub(ctx context.Context, sub string) (repository.User, error)
	CreateIAMBarnUser(ctx context.Context, sub, username string) (repository.User, error)
}

const (
	oidcStateCookie = "oidc_state"
	oidcStateTTL    = 10 * time.Minute
)

// handleOIDCLogin starts the authorization code + PKCE flow.
// GET /api/v1/auth/oidc/login
func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if !s.iambarnFlagEnabled(r.Context(), map[string]any{"user_agent": r.Header.Get("User-Agent")}) {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	verifier, err := iambarn.GenerateVerifier()
	if err != nil {
		slog.Error("oidc: generate pkce verifier", "error", err, "handled", false)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	state, err := generateOpaqueToken()
	if err != nil {
		slog.Error("oidc: generate state", "error", err, "handled", false)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    state + "|" + verifier,
		Path:     "/",
		MaxAge:   int(oidcStateTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})

	http.Redirect(w, r,
		s.iambarnProvider.AuthorizationURL(state, iambarn.Challenge(verifier)),
		http.StatusFound,
	)
}

// handleOIDCLoggedOut is the landing endpoint for IAMBarn's RP-initiated logout
// (/oauth2/end-session). IAMBarn ends its own session, then redirects the
// browser here; we clear the local FunnelBarn session so both are gone, then
// send the user to the login page. Registered as the client's
// post_logout_redirect_uri.
// GET /api/v1/auth/oidc/logged-out
func (s *Server) handleOIDCLoggedOut(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w, r)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// handleOIDCCallback completes the PKCE flow and issues a FunnelBarn session.
// GET /api/v1/auth/oidc/callback
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if !s.iambarnFlagEnabled(r.Context(), map[string]any{"user_agent": r.Header.Get("User-Agent")}) {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	// Validate state to prevent CSRF.
	stateCookie, err := r.Cookie(oidcStateCookie)
	if err != nil {
		slog.WarnContext(r.Context(), "oidc: missing state cookie")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}
	state, verifier, ok := parseStateCookie(stateCookie.Value)
	if !ok {
		slog.WarnContext(r.Context(), "oidc: malformed state cookie")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}
	if r.URL.Query().Get("state") != state {
		slog.WarnContext(r.Context(), "oidc: state mismatch")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	// Clear state cookie.
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		slog.WarnContext(r.Context(), "oidc: authorization error", "error", errParam,
			"description", r.URL.Query().Get("error_description"))
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		slog.WarnContext(r.Context(), "oidc: missing code in callback")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	// Exchange code for tokens and validate the ID token.
	claims, err := s.iambarnProvider.ExchangeAndValidate(r.Context(), code, verifier)
	if err != nil {
		slog.WarnContext(r.Context(), "oidc: token exchange/validation failed", "error", err)
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	// Upsert the user record, keyed by the stable sub claim.
	if s.iambarnUsers != nil {
		if _, upsertErr := s.iambarnUsers.CreateIAMBarnUser(r.Context(), claims.Sub, claims.DisplayName()); upsertErr != nil {
			slog.WarnContext(r.Context(), "oidc: upsert user", "sub", claims.Sub, "error", upsertErr)
			// Non-fatal: session is issued regardless; user record is best-effort.
		}
	}

	token, expires, err := s.sessionManager.Create(claims.DisplayName())
	if err != nil {
		slog.Error("oidc: create session", "error", err, "handled", false)
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

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

	slog.InfoContext(r.Context(), "oidc login", "sub", claims.Sub, "display", claims.DisplayName())
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func parseStateCookie(value string) (state, verifier string, ok bool) {
	idx := strings.Index(value, "|")
	if idx <= 0 || idx == len(value)-1 {
		return "", "", false
	}
	return value[:idx], value[idx+1:], true
}

func generateOpaqueToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
