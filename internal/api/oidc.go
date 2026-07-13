package api

import (
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/iambarn"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
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

	ctx, span := tracing.StartSpan(r.Context(), "oidc.login",
		attribute.String("auth.provider", "iambarn"),
	)
	defer span.End()

	verifier, err := iambarn.GenerateVerifier()
	if err != nil {
		tracing.RecordError(span, err)
		slog.ErrorContext(ctx, "oidc: generate pkce verifier", "error", err, "handled", false)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	state, err := generateOpaqueToken()
	if err != nil {
		tracing.RecordError(span, err)
		slog.ErrorContext(ctx, "oidc: generate state", "error", err, "handled", false)
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

// handleOIDCCallback completes the PKCE flow and issues a FunnelBarn session.
// GET /api/v1/auth/oidc/callback
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if !s.iambarnFlagEnabled(r.Context(), map[string]any{"user_agent": r.Header.Get("User-Agent")}) {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "oidc.callback",
		attribute.String("auth.provider", "iambarn"),
	)
	defer span.End()

	// Validate state to prevent CSRF.
	stateCookie, err := r.Cookie(oidcStateCookie)
	if err != nil {
		span.SetAttributes(attribute.String("oidc.failure_reason", "missing_state_cookie"))
		slog.WarnContext(ctx, "oidc: missing state cookie")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}
	state, verifier, ok := parseStateCookie(stateCookie.Value)
	if !ok {
		span.SetAttributes(attribute.String("oidc.failure_reason", "malformed_state_cookie"))
		slog.WarnContext(ctx, "oidc: malformed state cookie")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}
	if r.URL.Query().Get("state") != state {
		span.SetAttributes(attribute.String("oidc.failure_reason", "state_mismatch"))
		slog.WarnContext(ctx, "oidc: state mismatch")
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
		span.SetAttributes(
			attribute.String("oidc.failure_reason", "authorization_error"),
			attribute.String("oidc.error", errParam),
		)
		slog.WarnContext(ctx, "oidc: authorization error", "error", errParam,
			"description", r.URL.Query().Get("error_description"))
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		span.SetAttributes(attribute.String("oidc.failure_reason", "missing_code"))
		slog.WarnContext(ctx, "oidc: missing code in callback")
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}

	// Exchange code for tokens and validate the ID token.
	claims, err := s.iambarnProvider.ExchangeAndValidate(ctx, code, verifier)
	if err != nil {
		span.SetAttributes(attribute.String("oidc.failure_reason", "exchange_failed"))
		tracing.RecordError(span, err)
		slog.WarnContext(ctx, "oidc: token exchange/validation failed", "error", err)
		http.Redirect(w, r, "/login?error=auth_failed", http.StatusFound)
		return
	}
	span.SetAttributes(attribute.String("user.sub", claims.Sub))

	// Upsert the user record, keyed by the stable sub claim.
	if s.iambarnUsers != nil {
		if _, upsertErr := s.iambarnUsers.CreateIAMBarnUser(ctx, claims.Sub, claims.DisplayName()); upsertErr != nil {
			slog.WarnContext(ctx, "oidc: upsert user", "sub", claims.Sub, "error", upsertErr)
			// Non-fatal: session is issued regardless; user record is best-effort.
		}
	}

	token, expires, err := s.sessionManager.Create(claims.DisplayName())
	if err != nil {
		span.SetAttributes(attribute.String("oidc.failure_reason", "session_create_failed"))
		tracing.RecordError(span, err)
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

	slog.InfoContext(ctx, "oidc login", "sub", claims.Sub, "display", claims.DisplayName())
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
