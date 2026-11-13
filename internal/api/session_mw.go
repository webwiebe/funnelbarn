package api

// Session authentication for the dashboard's own routes. Split out of
// server.go so that file stays under its length ratchet; the behaviour is
// unchanged.

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// requireSession wraps a handler to enforce server-side session authentication.
// The cookie is an opaque handle; the session row (username, expiry, iambarn
// tokens) lives in web_sessions. For OIDC sessions with an expired access
// token it runs the refresh_token grant (singleflight per session) before
// serving. It also applies the apiLimiter rate limit and CSRF validation on
// mutating methods.
//
// The no-auth pass-through below is a development convenience only: in
// production, startup refuses to boot with no authentication mechanism
// configured (cmd/funnelbarn), so this branch is unreachable there.
func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.sessionManager == nil || s.webSessions == nil || !s.authConfigured() {
			if !s.apiLimiter.allow(s.clientIP(r)) {
				jsonError(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			// No sign-in exists here, so there is no less-trusted caller either.
			next(w, r.WithContext(withSessionAdmin(r.Context(), true)))
			return
		}

		cookie, err := r.Cookie("funnelbarn_session")
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ws, ok := s.resolveSession(w, r, cookie.Value)
		if !ok {
			return
		}

		if isMutating(r.Method) && !s.csrfValid(r, cookie.Value) {
			jsonError(w, "csrf token invalid", http.StatusForbidden)
			return
		}

		if !s.apiLimiter.allow(s.clientIP(r)) {
			jsonError(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		slog.Debug("session valid", "username", ws.Username)
		ctx := withSessionUser(r.Context(), ws.Username)
		next(w, r.WithContext(withSessionAdmin(ctx, s.sessionIsAdmin(ws))))
	}
}

// authConfigured must reflect EVERY way a user can obtain a session: the
// env-var admin, the confidential OIDC client, AND local/DB users. Missing any
// of these would leave every route served unauthenticated on a deployment that
// authenticates only that way.
func (s *Server) authConfigured() bool {
	return s.userAuth.Enabled() || s.oidc != nil || s.localUsersExist
}

// csrfValid compares the CSRF header with the token derived from the session
// cookie, in constant time so the token can't be recovered by timing.
func (s *Server) csrfValid(r *http.Request, cookieValue string) bool {
	expected := s.sessionManager.CSRFToken(cookieValue)
	got := r.Header.Get("X-FunnelBarn-CSRF")
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// resolveSession loads the session behind the cookie and brings it up to date:
// a reader forwards to the writer when serving needs a write, an expired row
// is pruned, and an OIDC session with an expired access token is refreshed.
// ok is false when a response has already been written.
func (s *Server) resolveSession(w http.ResponseWriter, r *http.Request, cookieValue string) (repository.WebSession, bool) {
	idHash := auth.HashSessionToken(cookieValue)
	ws, err := s.webSessions.GetWebSession(r.Context(), idHash)
	if err != nil {
		// Unknown handle: revoked, pruned, or forged. Same response either
		// way, nothing to enumerate.
		jsonError(w, "session expired", http.StatusUnauthorized)
		return ws, false
	}
	now := time.Now()
	expired := ws.AbsoluteExpiresAt <= now.Unix()
	// Pruning an expired session and storing refreshed tokens both write
	// the row, so a reader lets the writer serve such a request whole. A
	// reader that refreshed itself would burn the single-use refresh token
	// and then fail to store its successor.
	if s.forwarder != nil && (expired || s.needsRefresh(ws, now)) {
		s.forward(w, r)
		return ws, false
	}
	if expired {
		_ = s.webSessions.DeleteWebSession(r.Context(), idHash)
		jsonError(w, "session expired", http.StatusUnauthorized)
		return ws, false
	}

	// OIDC sessions are refresh-gated: session validity tracks the ~15m
	// access token, so central revocation bites at the next refresh.
	if s.needsRefresh(ws, now) {
		ws, err = s.refreshSession(r.Context(), idHash)
		if err != nil {
			jsonError(w, "session expired", http.StatusUnauthorized)
			return ws, false
		}
	}
	return ws, true
}

// needsRefresh reports whether serving ws first runs the refresh_token grant.
func (s *Server) needsRefresh(ws repository.WebSession, now time.Time) bool {
	return ws.AuthMethod == "oidc" && s.oidc != nil && accessTokenExpired(ws, now)
}

// accessTokenExpired reports whether the session's access token is past (or
// within accessTokenSkew of) its expiry. Sessions without a known expiry
// (no expires_in in the token response) are never treated as expired.
func accessTokenExpired(ws repository.WebSession, now time.Time) bool {
	return ws.AccessExpiresAt != 0 && now.Unix() >= ws.AccessExpiresAt-int64(accessTokenSkew.Seconds())
}

// refreshSession runs the refresh_token grant for one session, singleflighted
// per session row so concurrent requests cannot replay the single-use refresh
// token (a replay revokes the whole token family at the IdP).
//
// Outcomes:
//   - success: tokens rotated + claims re-snapshotted in the row
//   - invalid_grant: the session is dead centrally → row deleted, error
//   - new claims fail the sign-in role check (role removed in IAMBarn):
//     row deleted, the new refresh token revoked, error
//   - transient failure (IdP down): serve stale within the grace window
//     measured from the FIRST failure; past the window → row deleted, error
func (s *Server) refreshSession(ctx context.Context, idHash string) (repository.WebSession, error) {
	ctx, span := tracing.StartSpan(ctx, "oidc.refresh_session")
	defer span.End()

	// The refresh must complete even if the triggering request is canceled
	// mid-rotation — losing the rotated refresh token kills the session.
	// Waiting singleflight sharers also must not inherit a canceled context.
	bgCtx := context.WithoutCancel(ctx)
	v, err, _ := s.refreshGroup.Do(idHash, func() (any, error) {
		ws, err := s.webSessions.GetWebSession(bgCtx, idHash)
		if err != nil {
			return nil, err
		}
		span.SetAttributes(attribute.String("oidc.sub", ws.IdpSub))
		now := time.Now()
		if !accessTokenExpired(ws, now) {
			// Another flight already refreshed while we waited.
			span.SetAttributes(attribute.String("oidc.refresh_outcome", "already_refreshed"))
			return ws, nil
		}

		refreshed, err := s.oidc.Refresh(bgCtx, ws.RefreshToken)
		if errors.Is(err, auth.ErrRefreshInvalid) {
			// Revoked / rotated-elsewhere / user suspended: kill the session
			// immediately. invalid_grant never gets grace.
			span.SetAttributes(attribute.String("oidc.refresh_outcome", "invalid_grant"))
			slog.InfoContext(bgCtx, "session refresh: invalid_grant, session revoked",
				"username", ws.Username, "sub", ws.IdpSub)
			_ = s.webSessions.DeleteWebSession(bgCtx, idHash)
			return nil, err
		}
		if err != nil {
			return s.refreshFailed(bgCtx, span, idHash, ws, now, err)
		}

		if refreshed.Claims != nil && !s.oidc.Allowed(*refreshed.Claims) {
			span.SetAttributes(attribute.String("oidc.refresh_outcome", "access_revoked"))
			s.endSessionWithoutAccess(bgCtx, idHash, ws, refreshed.RefreshToken)
			return nil, errSessionAccessRevoked
		}

		claimsJSON := ""
		if refreshed.Claims != nil {
			// Re-snapshot groups/roles so central role changes propagate now,
			// not at the next login.
			claimsJSON = marshalClaims(*refreshed.Claims)
		}
		if err := s.webSessions.UpdateWebSessionTokens(bgCtx, idHash,
			refreshed.IDToken, refreshed.AccessToken, refreshed.RefreshToken,
			unixOrZero(refreshed.ExpiresAt), claimsJSON, now.Unix()); err != nil {
			// The old refresh token is already burned; failing to store the new
			// one makes the session unrenewable. Fail the request rather than
			// pretend the session is healthy.
			span.SetAttributes(attribute.String("oidc.refresh_outcome", "persist_error"))
			tracing.RecordError(span, err)
			slog.ErrorContext(bgCtx, "session refresh: persist rotated tokens",
				"err", err, "handled", false)
			return nil, err
		}
		span.SetAttributes(attribute.String("oidc.refresh_outcome", "rotated"))
		slog.InfoContext(bgCtx, "session refresh: rotated",
			"username", ws.Username, "claims_source", refreshed.ClaimsSource)
		ws.IDToken = refreshed.IDToken
		ws.AccessToken = refreshed.AccessToken
		ws.RefreshToken = refreshed.RefreshToken
		ws.AccessExpiresAt = unixOrZero(refreshed.ExpiresAt)
		if claimsJSON != "" {
			ws.ClaimsJSON = claimsJSON
		}
		ws.LastRefreshAt = now.Unix()
		ws.RefreshFailingSince = 0
		return ws, nil
	})
	if err != nil {
		return repository.WebSession{}, err
	}
	return v.(repository.WebSession), nil
}

// errSessionAccessRevoked ends a session whose refreshed claims no longer pass
// the sign-in role check.
var errSessionAccessRevoked = errors.New("session refresh: sign-in role no longer held")

// endSessionWithoutAccess deletes a session whose refreshed claims fail the
// sign-in role check, and revokes the refresh token the IdP just issued so it
// cannot outlive the session. Both steps are best-effort; the caller answers
// 401 either way.
func (s *Server) endSessionWithoutAccess(ctx context.Context, idHash string, ws repository.WebSession, refreshToken string) {
	slog.InfoContext(ctx, "session refresh: sign-in role no longer held, session ended",
		"username", ws.Username, "sub", ws.IdpSub)
	if err := s.webSessions.DeleteWebSession(ctx, idHash); err != nil {
		slog.WarnContext(ctx, "session refresh: delete session without access", "err", err)
	}
	if refreshToken == "" {
		return
	}
	if err := s.oidc.RevokeRefreshToken(ctx, refreshToken); err != nil {
		slog.WarnContext(ctx, "session refresh: revoke refresh token", "err", err)
	}
}

// refreshFailed handles a transient refresh failure (network, IdP 5xx): it
// serves the stale session within the grace window, measured from the first
// failure, and deletes the row once the window has passed.
func (s *Server) refreshFailed(ctx context.Context, span trace.Span, idHash string, ws repository.WebSession, now time.Time, err error) (any, error) {
	failingSince := ws.RefreshFailingSince
	if failingSince == 0 {
		failingSince = now.Unix()
		if markErr := s.webSessions.MarkWebSessionRefreshFailing(ctx, idHash, failingSince); markErr != nil {
			slog.WarnContext(ctx, "session refresh: mark failing", "err", markErr)
		}
		ws.RefreshFailingSince = failingSince
	}
	if now.Unix()-failingSince > int64(s.oidcRefreshGrace.Seconds()) {
		span.SetAttributes(attribute.String("oidc.refresh_outcome", "grace_exceeded"))
		tracing.RecordError(span, err)
		slog.WarnContext(ctx, "session refresh: grace exceeded, session cut off",
			"username", ws.Username, "failing_for", now.Unix()-failingSince)
		_ = s.webSessions.DeleteWebSession(ctx, idHash)
		return nil, err
	}
	span.SetAttributes(attribute.String("oidc.refresh_outcome", "stale_within_grace"))
	slog.WarnContext(ctx, "session refresh failed, serving stale within grace",
		"err", err, "username", ws.Username)
	return ws, nil
}
