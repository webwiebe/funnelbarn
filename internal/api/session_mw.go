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
		// authConfigured must reflect EVERY way a user can obtain a session:
		// the env-var admin, the confidential OIDC client, AND local/DB users.
		// Missing any of these would leave every route below served
		// unauthenticated on a deployment that authenticates only that way.
		authConfigured := s.userAuth.Enabled() || s.oidc != nil || s.localUsersExist
		if s.sessionManager == nil || s.webSessions == nil || !authConfigured {
			if !s.apiLimiter.allow(s.clientIP(r)) {
				jsonError(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next(w, r)
			return
		}

		cookie, err := r.Cookie("funnelbarn_session")
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		idHash := auth.HashSessionToken(cookie.Value)
		ws, err := s.webSessions.GetWebSession(r.Context(), idHash)
		if err != nil {
			// Unknown handle: revoked, pruned, or forged. Same response either
			// way — nothing to enumerate.
			jsonError(w, "session expired", http.StatusUnauthorized)
			return
		}
		now := time.Now()
		if ws.AbsoluteExpiresAt <= now.Unix() {
			_ = s.webSessions.DeleteWebSession(r.Context(), idHash)
			jsonError(w, "session expired", http.StatusUnauthorized)
			return
		}

		// OIDC sessions are refresh-gated: session validity tracks the ~15m
		// access token, so central revocation bites at the next refresh.
		if ws.AuthMethod == "oidc" && s.oidc != nil && accessTokenExpired(ws, now) {
			ws, err = s.refreshSession(r.Context(), idHash)
			if err != nil {
				jsonError(w, "session expired", http.StatusUnauthorized)
				return
			}
		}

		if isMutating(r.Method) {
			expected := s.sessionManager.CSRFToken(cookie.Value)
			got := r.Header.Get("X-FunnelBarn-CSRF")
			// Constant-time compare so the CSRF token can't be recovered by timing.
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
				jsonError(w, "csrf token invalid", http.StatusForbidden)
				return
			}
		}

		if !s.apiLimiter.allow(s.clientIP(r)) {
			jsonError(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		slog.Debug("session valid", "username", ws.Username)
		next(w, r.WithContext(withSessionUser(r.Context(), ws.Username)))
	}
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
			// Transient (network, IdP 5xx): serve stale within the grace
			// window, measured from the first failure.
			failingSince := ws.RefreshFailingSince
			if failingSince == 0 {
				failingSince = now.Unix()
				if markErr := s.webSessions.MarkWebSessionRefreshFailing(bgCtx, idHash, failingSince); markErr != nil {
					slog.WarnContext(bgCtx, "session refresh: mark failing", "err", markErr)
				}
				ws.RefreshFailingSince = failingSince
			}
			if now.Unix()-failingSince > int64(s.oidcRefreshGrace.Seconds()) {
				span.SetAttributes(attribute.String("oidc.refresh_outcome", "grace_exceeded"))
				tracing.RecordError(span, err)
				slog.WarnContext(bgCtx, "session refresh: grace exceeded, session cut off",
					"username", ws.Username, "failing_for", now.Unix()-failingSince)
				_ = s.webSessions.DeleteWebSession(bgCtx, idHash)
				return nil, err
			}
			span.SetAttributes(attribute.String("oidc.refresh_outcome", "stale_within_grace"))
			slog.WarnContext(bgCtx, "session refresh failed, serving stale within grace",
				"err", err, "username", ws.Username)
			return ws, nil
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
