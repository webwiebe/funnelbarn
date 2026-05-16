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

// IAMBarnUserRepo is the storage interface needed by the IAMBarn callback handler.
type IAMBarnUserRepo interface {
	FindUserByIAMBarnSub(ctx context.Context, sub string) (repository.User, error)
	CreateIAMBarnUser(ctx context.Context, sub, username string) (repository.User, error)
}

const (
	iambarnStateCookie = "iambarn_state"
	iambarnStateTTL    = 10 * time.Minute
)

// handleIAMBarnLogin starts the authorization code + PKCE flow.
// GET /api/v1/auth/iambarn/login
func (s *Server) handleIAMBarnLogin(w http.ResponseWriter, r *http.Request) {
	if !s.iambarnFlagEnabled(r.Context()) {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	verifier, err := iambarn.GenerateVerifier()
	if err != nil {
		slog.Error("iambarn: generate pkce verifier", "error", err, "handled", false)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	state, err := generateOpaqueToken()
	if err != nil {
		slog.Error("iambarn: generate state", "error", err, "handled", false)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     iambarnStateCookie,
		Value:    state + "|" + verifier,
		Path:     "/",
		MaxAge:   int(iambarnStateTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})

	http.Redirect(w, r,
		s.iambarnProvider.AuthorizationURL(state, iambarn.Challenge(verifier)),
		http.StatusFound,
	)
}

// handleIAMBarnCallback completes the PKCE flow and issues a FunnelBarn session.
// GET /api/v1/auth/iambarn/callback
func (s *Server) handleIAMBarnCallback(w http.ResponseWriter, r *http.Request) {
	if !s.iambarnFlagEnabled(r.Context()) {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	// Validate state to prevent CSRF.
	stateCookie, err := r.Cookie(iambarnStateCookie)
	if err != nil {
		slog.WarnContext(r.Context(), "iambarn: missing state cookie")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}
	state, verifier, ok := parseStateCookie(stateCookie.Value)
	if !ok {
		slog.WarnContext(r.Context(), "iambarn: malformed state cookie")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}
	if r.URL.Query().Get("state") != state {
		slog.WarnContext(r.Context(), "iambarn: state mismatch")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	// Clear state cookie.
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     iambarnStateCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		slog.WarnContext(r.Context(), "iambarn: authorization error", "error", errParam,
			"description", r.URL.Query().Get("error_description"))
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		slog.WarnContext(r.Context(), "iambarn: missing code in callback")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	// Exchange code for tokens and validate the ID token.
	claims, err := s.iambarnProvider.ExchangeAndValidate(r.Context(), code, verifier)
	if err != nil {
		slog.WarnContext(r.Context(), "iambarn: token exchange/validation failed", "error", err)
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	// Upsert the user record, keyed by the stable IAMBarn sub.
	if s.iambarnUsers != nil {
		if _, upsertErr := s.iambarnUsers.CreateIAMBarnUser(r.Context(), claims.Sub, claims.DisplayName()); upsertErr != nil {
			slog.WarnContext(r.Context(), "iambarn: upsert user", "sub", claims.Sub, "error", upsertErr)
			// Non-fatal: session is issued regardless; user record is best-effort.
		}
	}

	// Issue the standard FunnelBarn session using the IAMBarn display name as username.
	token, expires, err := s.sessionManager.Create(claims.DisplayName())
	if err != nil {
		slog.Error("iambarn: create session", "error", err, "handled", false)
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	http.SetCookie(w, auth.SessionCookie(token, expires, secure))
	http.SetCookie(w, auth.CSRFCookie(token, expires, secure))

	slog.InfoContext(r.Context(), "iambarn login", "sub", claims.Sub, "display", claims.DisplayName())
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
