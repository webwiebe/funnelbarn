package api

import (
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// handleListProjects lists all projects for a dashboard session, or just the
// one project an API token is scoped to.
//
// The path carries no {id}, so the token middleware cannot check the route
// against the key's project for us — narrowing here is what keeps one
// customer's analytics token from enumerating every project on the instance.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.projects.ListProjects(r.Context())
	if err != nil {
		mapServiceError(w, err, "handleListProjects")
		return
	}
	if tokenProject := tokenProjectFromContext(r.Context()); tokenProject != "" {
		mine := make([]domain.Project, 0, 1)
		for _, p := range projects {
			if p.ID == tokenProject {
				mine = append(mine, p)
			}
		}
		projects = mine
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
	// The domain is a display label for the site the project measures. It
	// drives no routing: ingest resolves the project by API key and
	// x-funnelbarn-project, which is what lets any f.<domain> alias work.
	if domain := strings.TrimSpace(body.Domain); domain != "" {
		project, err = s.projects.UpdateProject(r.Context(), project.ID, project.Name, domain)
		if err != nil {
			mapServiceError(w, err, "handleCreateProject")
			return
		}
	}
	slog.InfoContext(r.Context(), "project created", "project_id", project.ID, "name", body.Name, "request_id", RequestIDFromContext(r.Context()))
	writeJSON(w, http.StatusCreated, project)
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
