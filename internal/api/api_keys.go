package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// safeKey is an API key as returned to the dashboard: never the hash, and with
// last_used_at so an operator can tell a token something depends on from one
// that was issued and forgotten before revoking it.
type safeKey struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	Name       string `json:"name"`
	Scope      string `json:"scope"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

func toSafeKey(k domain.APIKey) safeKey {
	sk := safeKey{
		ID:        k.ID,
		ProjectID: k.ProjectID,
		Name:      k.Name,
		Scope:     k.Scope,
		CreatedAt: k.CreatedAt.Format(time.RFC3339),
	}
	if k.LastUsedAt != nil {
		sk.LastUsedAt = k.LastUsedAt.Format(time.RFC3339)
	}
	return sk
}

// handleListAPIKeys lists API keys.
// Accepts an optional ?project_id= query param to filter by project.
// When omitted, returns all API keys across all projects.
func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")

	var keys []domain.APIKey
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

	var safe []safeKey
	for _, k := range keys {
		safe = append(safe, toSafeKey(k))
	}
	if safe == nil {
		safe = []safeKey{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": safe})
}

// handleCreateAPIKey creates a new API key for a project.
// project_id may be sent in the request body or as a query param, and is
// required: this used to fall back to the first project on the instance, which
// meant a caller that forgot to name one got a key silently bound to whichever
// project happened to sort first — indistinguishable from a working key until
// it 403s on the project you meant.
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

	if body.ProjectID == "" {
		jsonError(w, "project_id is required — an API key is always scoped to one project", http.StatusBadRequest)
		return
	}

	if body.Scope == "" {
		body.Scope = domain.APIKeyScopeIngest
	}

	// Verify project exists.
	if _, err := s.projects.GetProject(r.Context(), body.ProjectID); err != nil {
		mapServiceError(w, err, "handleCreateAPIKey.getProject")
		return
	}

	key, plaintext, err := s.apikeys.IssueAPIKey(r.Context(), body.Name, body.ProjectID, body.Scope)
	if err != nil {
		var genErr *service.KeyGenerationError
		if errors.As(err, &genErr) {
			slog.Error("failed to generate api key random bytes", "error", genErr.Err, "handled", false)
			jsonError(w, "failed to generate key", http.StatusInternalServerError)
			return
		}
		mapServiceError(w, err, "handleCreateAPIKey")
		return
	}

	// Return the plaintext key once — it won't be shown again.
	slog.InfoContext(r.Context(), "api key created", "key_id", key.ID, "name", body.Name, "scope", body.Scope, "project_id", body.ProjectID, "request_id", RequestIDFromContext(r.Context()))
	writeJSON(w, http.StatusCreated, map[string]any{
		"api_key": toSafeKey(key),
		"key":     plaintext,
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
