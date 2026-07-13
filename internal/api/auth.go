package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
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
	if _, err := s.issueWebSession(r.Context(), w, secure, repository.WebSession{
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

// handleListProjects lists all projects.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.projects.ListProjects(r.Context())
	if err != nil {
		mapServiceError(w, err, "handleListProjects")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

// handleCreateProject creates a new project.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Slug   string `json:"slug"`
		Domain string `json:"domain"` // UI sends domain; use as slug if slug is empty
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// Derive slug from domain or name if not provided; service validates emptiness.
	if body.Slug == "" && body.Domain != "" {
		body.Slug = toSlug(body.Domain)
	}
	if body.Slug == "" && body.Name != "" {
		body.Slug = toSlug(body.Name)
	}

	project, err := s.projects.CreateProject(r.Context(), body.Name, body.Slug)
	if err != nil {
		mapServiceError(w, err, "handleCreateProject")
		return
	}
	slog.InfoContext(r.Context(), "project created", "project_id", project.ID, "name", body.Name, "request_id", RequestIDFromContext(r.Context()))
	writeJSON(w, http.StatusCreated, project)
}

// handleListAPIKeys lists API keys.
// Accepts an optional ?project_id= query param to filter by project.
// When omitted, returns all API keys across all projects.
func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")

	var keys []repository.APIKey
	var err error
	if projectID != "" {
		keys, err = s.apikeys.ListAPIKeys(r.Context(), projectID)
	} else {
		keys, err = s.apikeys.ListAllAPIKeys(r.Context())
	}
	if err != nil {
		mapServiceError(w, err, "handleListAPIKeys")
		return
	}

	// Mask key hashes in the response.
	type safeKey struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Scope     string `json:"scope"`
		CreatedAt string `json:"created_at"`
	}
	var safe []safeKey
	for _, k := range keys {
		safe = append(safe, safeKey{
			ID:        k.ID,
			Name:      k.Name,
			Scope:     k.Scope,
			CreatedAt: k.CreatedAt.Format(time.RFC3339),
		})
	}
	if safe == nil {
		safe = []safeKey{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": safe})
}

// handleCreateAPIKey creates a new API key for a project.
// project_id may be sent in the request body or as a query param.
// When omitted, the first project is used (single-tenant convenience).
func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectID string `json:"project_id"`
		Name      string `json:"name"`
		Scope     string `json:"scope"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Allow project_id from query param as fallback.
	if body.ProjectID == "" {
		body.ProjectID = r.URL.Query().Get("project_id")
	}

	// If still empty, pick the first available project.
	if body.ProjectID == "" {
		projects, err := s.projects.ListProjects(r.Context())
		if err != nil || len(projects) == 0 {
			jsonError(w, "no projects found — create a project first", http.StatusBadRequest)
			return
		}
		body.ProjectID = projects[0].ID
	}

	if body.Scope == "" {
		body.Scope = repository.APIKeyScopeIngest
	}

	// Verify project exists.
	if _, err := s.projects.GetProject(r.Context(), body.ProjectID); err != nil {
		mapServiceError(w, err, "handleCreateAPIKey.getProject")
		return
	}

	// Generate random key.
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		slog.Error("failed to generate api key random bytes", "error", err, "handled", false)
		jsonError(w, "failed to generate key", http.StatusInternalServerError)
		return
	}
	plaintext := hex.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(plaintext))
	keySHA256 := hex.EncodeToString(sum[:])

	key, err := s.apikeys.CreateAPIKey(r.Context(), body.Name, body.ProjectID, keySHA256, body.Scope)
	if err != nil {
		mapServiceError(w, err, "handleCreateAPIKey")
		return
	}

	// Return the plaintext key once — it won't be shown again.
	type safeKey struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Scope     string `json:"scope"`
		CreatedAt string `json:"created_at"`
	}
	slog.InfoContext(r.Context(), "api key created", "key_id", key.ID, "name", body.Name, "scope", body.Scope, "project_id", body.ProjectID, "request_id", RequestIDFromContext(r.Context()))
	writeJSON(w, http.StatusCreated, map[string]any{
		"api_key": safeKey{
			ID:        key.ID,
			Name:      key.Name,
			Scope:     key.Scope,
			CreatedAt: key.CreatedAt.Format(time.RFC3339),
		},
		"key": plaintext,
	})
}

// handleDeleteAPIKey deletes an API key by its ID.
func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	keyID := r.PathValue("kid")
	if keyID == "" {
		jsonError(w, "key id required", http.StatusBadRequest)
		return
	}
	if err := s.apikeys.DeleteAPIKey(r.Context(), keyID); err != nil {
		mapServiceError(w, err, "handleDeleteAPIKey")
		return
	}
	slog.InfoContext(r.Context(), "api key deleted", "key_id", keyID, "request_id", RequestIDFromContext(r.Context()))
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteProject deletes a project and all its data.
func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	// Purge R2 recording blobs first: once the SQLite rows are gone we can no
	// longer locate the chunk keys. A purge failure is logged but must not block
	// tenant deletion — a stray blob orphan is preferable to an undeletable
	// project (and the retention sweep will not reach orphaned blobs either).
	if s.recordings != nil {
		if err := s.recordings.PurgeProjectRecordings(r.Context(), projectID); err != nil {
			slog.ErrorContext(r.Context(), "delete project: purge recordings failed",
				"err", err, "handled", true, "project_id", projectID)
		}
	}
	if err := s.projects.DeleteProject(r.Context(), projectID); err != nil {
		mapServiceError(w, err, "handleDeleteProject")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateProject updates a project's name and domain.
func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	var body struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	project, err := s.projects.UpdateProject(r.Context(), projectID, body.Name, body.Domain)
	if err != nil {
		mapServiceError(w, err, "handleUpdateProject")
		return
	}
	writeJSON(w, http.StatusOK, project)
}

// handleApproveProject sets a project's status to 'active'.
//
// POST /api/v1/projects/{id}/approve
func (s *Server) handleApproveProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	project, err := s.projects.ApproveProject(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleApproveProject")
		return
	}
	writeJSON(w, http.StatusOK, project)
}

// toSlug converts a display name to a URL-safe slug.
func toSlug(name string) string {
	var sb strings.Builder
	prev := '-'
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
			prev = r
		} else if prev != '-' {
			sb.WriteRune('-')
			prev = '-'
		}
	}
	return strings.Trim(sb.String(), "-")
}
