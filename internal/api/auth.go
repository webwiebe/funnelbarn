package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/storage"
)

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
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
	} else {
		// Fall back to DB user.
		user, err := s.store.UserByUsername(r.Context(), body.Username)
		if err != nil {
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)) != nil {
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
	}

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	token, expires, err := s.sessionManager.Create(body.Username)
	if err != nil {
		jsonError(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, auth.SessionCookie(token, expires, secure))
	http.SetCookie(w, auth.CSRFCookie(token, expires, secure))

	writeJSON(w, http.StatusOK, map[string]any{
		"username": body.Username,
	})
}

// handleLogout clears the session cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, auth.ClearSessionCookie(secure))
	http.SetCookie(w, auth.ClearCSRFCookie(secure))
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// handleMe returns the current user plus metadata useful for first-run detection.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("funnelbarn_session")
	if err != nil {
		jsonError(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	username, ok := s.sessionManager.Valid(cookie.Value)
	if !ok {
		jsonError(w, "session invalid", http.StatusUnauthorized)
		return
	}

	hasProjects, _ := s.store.HasProjects(r.Context())

	writeJSON(w, http.StatusOK, map[string]any{
		"username":     username,
		"has_projects": hasProjects,
	})
}

// handleListProjects lists all projects.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		jsonError(w, "failed to list projects", http.StatusInternalServerError)
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
	if body.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if body.Slug == "" && body.Domain != "" {
		body.Slug = toSlug(body.Domain)
	}
	if body.Slug == "" {
		body.Slug = toSlug(body.Name)
	}

	project, err := s.store.CreateProject(r.Context(), body.Name, body.Slug)
	if err != nil {
		jsonError(w, "failed to create project", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

// handleListAPIKeys lists API keys.
// Accepts an optional ?project_id= query param to filter by project.
// When omitted, returns all API keys across all projects.
func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")

	var keys []storage.APIKey
	var err error
	if projectID != "" {
		keys, err = s.store.ListAPIKeys(r.Context(), projectID)
	} else {
		keys, err = s.store.ListAllAPIKeys(r.Context())
	}
	if err != nil {
		jsonError(w, "failed to list api keys", http.StatusInternalServerError)
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
		projects, err := s.store.ListProjects(r.Context())
		if err != nil || len(projects) == 0 {
			jsonError(w, "no projects found — create a project first", http.StatusBadRequest)
			return
		}
		body.ProjectID = projects[0].ID
	}

	if body.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if body.Scope == "" {
		body.Scope = storage.APIKeyScopeIngest
	}
	if body.Scope != storage.APIKeyScopeFull && body.Scope != storage.APIKeyScopeIngest {
		jsonError(w, "scope must be 'full' or 'ingest'", http.StatusBadRequest)
		return
	}

	// Verify project exists.
	if _, err := s.store.ProjectByID(r.Context(), body.ProjectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, "project not found", http.StatusNotFound)
			return
		}
		jsonError(w, "failed to look up project", http.StatusInternalServerError)
		return
	}

	// Generate random key.
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		jsonError(w, "failed to generate key", http.StatusInternalServerError)
		return
	}
	plaintext := hex.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(plaintext))
	keySHA256 := hex.EncodeToString(sum[:])

	key, err := s.store.CreateAPIKey(r.Context(), body.Name, body.ProjectID, keySHA256, body.Scope)
	if err != nil {
		jsonError(w, "failed to create api key", http.StatusInternalServerError)
		return
	}

	// Return the plaintext key once — it won't be shown again.
	// Response shape: { api_key: {...}, key: "<plaintext>" }
	type safeKey struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Scope     string `json:"scope"`
		CreatedAt string `json:"created_at"`
	}
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
	if err := s.store.DeleteAPIKey(r.Context(), keyID); err != nil {
		jsonError(w, "failed to delete api key", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteProject deletes a project and all its data.
func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteProject(r.Context(), projectID); err != nil {
		jsonError(w, "failed to delete project", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateProject updates a project's name.
func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	project, err := s.store.UpdateProject(r.Context(), projectID, body.Name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, "project not found", http.StatusNotFound)
			return
		}
		jsonError(w, "failed to update project", http.StatusInternalServerError)
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
