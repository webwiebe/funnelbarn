package api

import (
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// handleBackchannelLogout implements the RP side of OIDC Back-Channel Logout
// 1.0. iambarn POSTs a signed logout token (form-encoded, `logout_token=`)
// when a user's IdP session ends — end-session, admin revocation, suspension —
// and we destroy the matching local sessions immediately, instead of waiting
// for the next refresh to fail.
//
// The endpoint is public (the IdP is not a browser: no cookies, no CSRF) but
// rate-limited; the logout token itself is the authentication — signature,
// issuer, audience, freshness, and the mandatory `events` claim are all
// verified against the issuer's JWKS before anything is deleted.
//
// POST /api/v1/oidc/backchannel-logout
func (s *Server) handleBackchannelLogout(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil || s.webSessions == nil {
		jsonError(w, "oidc not configured", http.StatusNotFound)
		return
	}
	ctx, span := tracing.StartSpan(r.Context(), "oidc.backchannel_logout")
	defer span.End()
	r = r.WithContext(ctx)

	// Per spec the response must not be cached.
	w.Header().Set("Cache-Control", "no-store")

	if err := r.ParseForm(); err != nil {
		tracing.RecordError(span, err)
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	raw := r.PostFormValue("logout_token")
	if raw == "" {
		tracing.RecordError(span, errors.New("backchannel-logout: logout_token missing"))
		jsonError(w, "logout_token required", http.StatusBadRequest)
		return
	}
	claims, err := s.oidc.VerifyLogoutToken(r.Context(), raw)
	if err != nil {
		tracing.RecordError(span, err)
		slog.WarnContext(r.Context(), "backchannel-logout: token rejected", "error", err)
		jsonError(w, "invalid logout token", http.StatusBadRequest)
		return
	}
	span.SetAttributes(attribute.String("oidc.sid", claims.SessionID), attribute.String("oidc.sub", claims.Subject))

	// Prefer the precise sid (one IdP session); fall back to sub (all of the
	// user's sessions). Deleting zero rows is still a success per spec — the
	// desired state ("no sessions for this sid/sub") already holds.
	var deleted int64
	if claims.SessionID != "" {
		deleted, err = s.webSessions.DeleteWebSessionsByIdpSid(r.Context(), claims.SessionID)
	} else {
		deleted, err = s.webSessions.DeleteWebSessionsByIdpSub(r.Context(), claims.Subject)
	}
	if err != nil {
		tracing.RecordError(span, err)
		slog.ErrorContext(r.Context(), "backchannel-logout: delete sessions",
			"err", err, "handled", false, "sid", claims.SessionID, "sub", claims.Subject)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	span.SetAttributes(attribute.Int64("oidc.sessions_deleted", deleted))
	slog.InfoContext(r.Context(), "backchannel-logout: sessions revoked",
		"sid", claims.SessionID, "sub", claims.Subject, "deleted", deleted)
	w.WriteHeader(http.StatusOK)
}
