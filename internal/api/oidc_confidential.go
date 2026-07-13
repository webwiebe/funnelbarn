package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// IAMBarnUserRepo is the storage interface needed by the OIDC callback handler
// to upsert the local user record keyed on the stable IdP subject.
type IAMBarnUserRepo interface {
	FindUserByIAMBarnSub(ctx context.Context, sub string) (repository.User, error)
	CreateIAMBarnUser(ctx context.Context, sub, username string) (repository.User, error)
}

const (
	oidcConfStateCookie = "funnelbarn_oidc_state"
	oidcConfNonceCookie = "funnelbarn_oidc_nonce"
	oidcConfCookieTTL   = 10 * time.Minute
)

// handleOIDCConfidentialLogin starts the OIDC authorization-code + PKCE flow
// by redirecting the browser to the issuer's authorize endpoint. State (with
// the PKCE verifier, "state|verifier") and nonce are stored in short-lived
// HttpOnly cookies and checked on the callback.
func (s *Server) handleOIDCConfidentialLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		jsonError(w, "oidc not configured", http.StatusNotFound)
		return
	}
	ctx, span := tracing.StartSpan(r.Context(), "oidc_confidential.login",
		attribute.String("auth.provider", "oidc_confidential"),
	)
	defer span.End()

	state := oidcConfRandomToken()
	nonce := oidcConfRandomToken()
	verifier := oauth2.GenerateVerifier()
	authURL, err := s.oidc.AuthorizeURL(state, nonce, verifier)
	if err != nil {
		tracing.RecordError(span, err)
		slog.WarnContext(ctx, "oidc: build authorize url", "error", err)
		jsonError(w, "oidc unavailable", http.StatusServiceUnavailable)
		return
	}
	secure := s.isSecureRequest(r)
	http.SetCookie(w, oidcConfShortLivedCookie(oidcConfStateCookie, state+"|"+verifier, secure))
	http.SetCookie(w, oidcConfShortLivedCookie(oidcConfNonceCookie, nonce, secure))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCConfidentialCallback handles the redirect back from the issuer. On
// success it persists the iambarn token set in a server-side session row and
// hands the browser an opaque session handle.
func (s *Server) handleOIDCConfidentialCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		jsonError(w, "oidc not configured", http.StatusNotFound)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "oidc_confidential.callback",
		attribute.String("auth.provider", "oidc_confidential"),
	)
	defer span.End()

	stateCookie, err := r.Cookie(oidcConfStateCookie)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		span.SetAttributes(attribute.String("oidc.failure_reason", "state_mismatch"))
		slog.WarnContext(ctx, "oidc: state mismatch")
		jsonError(w, "oidc state mismatch", http.StatusBadRequest)
		return
	}
	nonceCookie, err := r.Cookie(oidcConfNonceCookie)
	if err != nil || nonceCookie.Value == "" {
		span.SetAttributes(attribute.String("oidc.failure_reason", "nonce_missing"))
		jsonError(w, "oidc nonce missing", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		span.SetAttributes(attribute.String("oidc.failure_reason", "code_missing"))
		jsonError(w, "oidc code missing", http.StatusBadRequest)
		return
	}
	claims, err := s.oidc.Exchange(ctx, code, nonceCookie.Value)
	if err != nil {
		span.SetAttributes(attribute.String("oidc.failure_reason", "exchange_failed"))
		tracing.RecordError(span, err)
		slog.WarnContext(ctx, "oidc: exchange failed", "error", err)
		jsonError(w, "oidc exchange failed", http.StatusUnauthorized)
		return
	}
	span.SetAttributes(attribute.String("user.sub", claims.Subject))
	if !s.oidc.Allowed(claims) {
		span.SetAttributes(attribute.String("oidc.failure_reason", "access_denied"))
		slog.WarnContext(ctx, "oidc: access denied",
			"sub", claims.Subject, "groups", claims.Groups, "roles", claims.Roles)
		jsonError(w, "access denied: user is not a member of the required group", http.StatusForbidden)
		return
	}
	if s.sessionManager == nil {
		// Misconfiguration: OIDC enabled but no session manager wired.
		span.SetAttributes(attribute.String("oidc.failure_reason", "session_manager_unconfigured"))
		slog.ErrorContext(ctx, "oidc: session manager not configured",
			"handled", false, "sub", claims.Subject)
		jsonError(w, "session unavailable", http.StatusServiceUnavailable)
		return
	}
	username := claims.PreferredName()
	if username == "" {
		username = "oidc-user"
	}

	// Upsert the user record, keyed by the stable sub claim. Non-fatal: the
	// session is issued regardless; the user record is best-effort.
	if s.iambarnUsers != nil {
		if _, upsertErr := s.iambarnUsers.CreateIAMBarnUser(r.Context(), claims.Subject, username); upsertErr != nil {
			slog.WarnContext(r.Context(), "oidc: upsert user", "sub", claims.Subject, "error", upsertErr)
		}
	}

	secure := s.isSecureRequest(r)
	expires, err := s.issueWebSession(r.Context(), w, secure, repository.WebSession{
		Username:        username,
		AuthMethod:      "oidc",
		IdpSub:          claims.Subject,
		IdpSid:          claims.SessionID,
		IDToken:         result.IDToken,
		AccessToken:     result.AccessToken,
		RefreshToken:    result.RefreshToken,
		AccessExpiresAt: unixOrZero(result.ExpiresAt),
		ClaimsJSON:      marshalClaims(claims),
	})
	if err != nil {
		span.SetAttributes(attribute.String("oidc.failure_reason", "session_create_failed"))
		tracing.RecordError(span, err)
		slog.ErrorContext(ctx, "oidc: failed to create session",
			"err", err, "handled", false, "sub", claims.Subject, "username", username)
		jsonError(w, "session unavailable", http.StatusServiceUnavailable)
		return
	}
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

	slog.InfoContext(r.Context(), "oidc login", "sub", claims.Subject, "sid", claims.SessionID, "username", username)
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

// issueWebSession mints an opaque session handle, persists the session row,
// and sets the session + CSRF cookies. Returns the session's absolute expiry.
func (s *Server) issueWebSession(ctx context.Context, w http.ResponseWriter, secure bool, ws repository.WebSession) (time.Time, error) {
	token, expires, err := s.sessionManager.Create()
	if err != nil {
		return time.Time{}, err
	}
	ws.IDHash = auth.HashSessionToken(token)
	ws.CreatedAt = time.Now().UTC().Unix()
	ws.AbsoluteExpiresAt = expires.Unix()
	if err := s.webSessions.CreateWebSession(ctx, ws); err != nil {
		return time.Time{}, err
	}
	http.SetCookie(w, auth.SessionCookie(token, expires, secure))
	http.SetCookie(w, auth.CSRFCookie(s.sessionManager.CSRFToken(token), expires, secure))
	return expires, nil
}

// marshalClaims snapshots the ID-token claims for storage in claims_json.
func marshalClaims(claims auth.OIDCClaims) string {
	b, err := json.Marshal(claims)
	if err != nil {
		return ""
	}
	return string(b)
}

// unixOrZero converts a token expiry to unix seconds, mapping the zero time
// (token response without expires_in) to 0 = "no expiry known".
func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// parseStateCookie splits the "state|verifier" value of the OIDC state cookie.
func parseStateCookie(value string) (state, verifier string, ok bool) {
	idx := strings.Index(value, "|")
	if idx <= 0 || idx == len(value)-1 {
		return "", "", false
	}
	return value[:idx], value[idx+1:], true
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
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure here would mean OIDC state/nonce tokens lose
		// entropy. Log loudly so it surfaces in BugBarn — the caller still
		// gets a token, but security relies on this not happening silently.
		slog.Error("oidc: crypto/rand failed generating state/nonce",
			"err", err, "handled", false)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
