package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
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

// handleMe returns the current user.
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
	writeJSON(w, http.StatusOK, map[string]string{"username": username})
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
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
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

// handleListAPIKeys lists API keys. Requires project_id query param.
func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		jsonError(w, "project_id required", http.StatusBadRequest)
		return
	}
	keys, err := s.store.ListAPIKeys(r.Context(), projectID)
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
			CreatedAt: k.CreatedAt.String(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": safe})
}

// handleCreateAPIKey creates a new API key for a project.
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
	if body.ProjectID == "" {
		jsonError(w, "project_id is required", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if body.Scope == "" {
		body.Scope = storage.APIKeyScopeFull
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
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         key.ID,
		"name":       key.Name,
		"scope":      key.Scope,
		"key":        plaintext,
		"created_at": key.CreatedAt,
	})
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
