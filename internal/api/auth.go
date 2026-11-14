package api

import (
	"log/slog"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// dummyBcryptHash is a valid bcrypt hash compared against on the user-not-found
// login path so that a missing username costs the same as a wrong password,
// preventing username enumeration via response timing.
var dummyBcryptHash = func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("funnelbarn-timing-equalizer"), bcrypt.DefaultCost)
	return h
}()

// handleLogin authenticates a user and sets a session cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Try env-var user auth.
	if s.userAuth.Enabled() {
		if !s.userAuth.Valid(body.Username, body.Password) {
			slog.WarnContext(r.Context(), "login failed", "username", body.Username, "reason", "invalid_credentials", "request_id", RequestIDFromContext(r.Context()))
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
	} else {
		// Fall back to DB user.
		user, err := s.projects.UserByUsername(r.Context(), body.Username)
		if err != nil {
			// Run a bcrypt comparison against a dummy hash so a missing username
			// takes the same time as a wrong password — otherwise the fast
			// no-bcrypt path here leaks which usernames exist.
			_ = bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(body.Password))
			slog.WarnContext(r.Context(), "login failed", "username", body.Username, "reason", "user_not_found", "request_id", RequestIDFromContext(r.Context()))
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)) != nil {
			slog.WarnContext(r.Context(), "login failed", "username", body.Username, "reason", "wrong_password", "request_id", RequestIDFromContext(r.Context()))
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
	}

	secure := s.isSecureRequest(r)

	if s.sessionManager == nil || s.webSessions == nil {
		slog.Error("login: session store not configured", "handled", false)
		jsonError(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	// Local sessions get a row too (token columns NULL): one middleware, one
	// revocation story — deleting the row logs the user out everywhere.
	if _, err := s.issueWebSession(r.Context(), w, secure, domain.WebSession{
		Username:   body.Username,
		AuthMethod: "local",
	}); err != nil {
		slog.Error("failed to create session", "error", err, "handled", false)
		jsonError(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	slog.InfoContext(r.Context(), "user login", "username", body.Username, "request_id", RequestIDFromContext(r.Context()))

	writeJSON(w, http.StatusOK, map[string]any{
		"username": body.Username,
	})
}

// clearSession destroys the presented session server-side and clears the
// session, CSRF, and auth-method cookies. For OIDC sessions it also revokes
// the refresh token at the issuer (best-effort) and returns the RP-initiated
// logout URL ({issuer}/oauth2/end-session with id_token_hint) so the caller
// can end the IdP session too. Shared by handleLogout (JSON API) and
// handleOIDCLoggedOut (the IAMBarn RP-initiated logout landing endpoint).
func (s *Server) clearSession(w http.ResponseWriter, r *http.Request) (logoutURL string) {
	ctx, span := tracing.StartSpan(r.Context(), "oidc.logout")
	defer span.End()
	r = r.WithContext(ctx)

	if cookie, err := r.Cookie("funnelbarn_session"); err == nil && s.webSessions != nil {
		idHash := auth.HashSessionToken(cookie.Value)
		if ws, err := s.webSessions.GetWebSession(r.Context(), idHash); err == nil {
			if ws.AuthMethod == "oidc" && s.oidc != nil {
				// Best-effort: kill the token family server-side so a leaked
				// refresh token dies with the session. Logout must succeed
				// even when the IdP is unreachable.
				if ws.RefreshToken != "" {
					if err := s.oidc.RevokeRefreshToken(r.Context(), ws.RefreshToken); err != nil {
						tracing.RecordError(span, err)
						slog.WarnContext(r.Context(), "logout: revoke refresh token", "error", err)
					}
				}
				if ws.IDToken != "" {
					if u, err := s.oidc.EndSessionURL(ws.IDToken); err == nil {
						logoutURL = u
					} else {
						tracing.RecordError(span, err)
						slog.WarnContext(r.Context(), "logout: build end-session url", "error", err)
					}
				}
			}
			// Deleting the row IS the revocation — the opaque handle is
			// worthless the moment the row is gone.
			if err := s.webSessions.DeleteWebSession(r.Context(), idHash); err != nil {
				tracing.RecordError(span, err)
				slog.ErrorContext(r.Context(), "logout: delete session", "err", err, "handled", false)
			}
		}
	}

	secure := s.isSecureRequest(r)
	http.SetCookie(w, auth.ClearSessionCookie(secure))
	http.SetCookie(w, auth.ClearCSRFCookie(secure))
	// Clear the auth-method hint set by the OIDC callback.
	http.SetCookie(w, &http.Cookie{
		Name:     "funnelbarn_auth_method",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	return logoutURL
}

// handleLogout destroys the session. For OIDC sessions the response carries a
// logout_url the SPA must navigate to, completing the server-driven logout at
// the IdP (RP-initiated logout with id_token_hint).
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	logoutURL := s.clearSession(w, r)
	resp := map[string]string{"status": "logged out"}
	if logoutURL != "" {
		resp["logout_url"] = logoutURL
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleMe returns the current user plus metadata useful for first-run detection.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	// requireSession already validated the session row and stashed the
	// username in the context.
	username := sessionUser(r.Context())
	if username == "" {
		jsonError(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	hasProjects, err := s.projects.HasProjects(r.Context())
	if err != nil {
		slog.Error("failed to check project existence", "error", err, "handled", false)
		// Non-fatal: return false so the UI can show first-run guidance.
		hasProjects = false
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"username":     username,
		"has_projects": hasProjects,
	})
}
