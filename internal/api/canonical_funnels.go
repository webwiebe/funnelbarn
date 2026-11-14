package api

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// ---- Canonical funnels ----

func (s *Server) handleListCanonicalFunnels(w http.ResponseWriter, r *http.Request) {
	funnels, err := s.overview.ListCanonicalFunnels(r.Context())
	if err != nil {
		mapServiceError(w, err, "handleListCanonicalFunnels")
		return
	}
	if funnels == nil {
		funnels = []domain.CanonicalFunnel{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"funnels": funnels})
}

// canonicalFunnelBody is the request shape for creating/updating a canonical funnel.
type canonicalFunnelBody struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Scope       string                       `json:"scope"`
	ProjectIDs  []string                     `json:"project_ids"`
	Segment     string                       `json:"segment"`
	Steps       []domain.CanonicalFunnelStep `json:"steps"`
}

func (b canonicalFunnelBody) toFunnel(id string) domain.CanonicalFunnel {
	scope := b.Scope
	if scope == "" {
		scope = "session"
	}
	return domain.CanonicalFunnel{
		ID:          id,
		Name:        b.Name,
		Description: b.Description,
		Scope:       scope,
		ProjectIDs:  b.ProjectIDs,
		Segment:     b.Segment,
		Steps:       b.Steps,
	}
}

func (s *Server) handleCreateCanonicalFunnel(w http.ResponseWriter, r *http.Request) {
	var body canonicalFunnelBody
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	created, err := s.overview.CreateCanonicalFunnel(r.Context(), body.toFunnel(""))
	if err != nil {
		mapServiceError(w, err, "handleCreateCanonicalFunnel")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateCanonicalFunnel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "funnel id required", http.StatusBadRequest)
		return
	}
	var body canonicalFunnelBody
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	updated, err := s.overview.UpdateCanonicalFunnel(r.Context(), body.toFunnel(id))
	if err != nil {
		mapServiceError(w, err, "handleUpdateCanonicalFunnel")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteCanonicalFunnel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "funnel id required", http.StatusBadRequest)
		return
	}
	if err := s.overview.DeleteCanonicalFunnel(r.Context(), id); err != nil {
		mapServiceError(w, err, "handleDeleteCanonicalFunnel")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCanonicalFunnelAnalysis runs a saved canonical funnel across projects.
// project_ids and segment default to the funnel's stored values; ?from&to&
// segment&project_ids override per request.
func (s *Server) handleCanonicalFunnelAnalysis(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "funnel id required", http.StatusBadRequest)
		return
	}
	funnel, err := s.overview.GetCanonicalFunnel(r.Context(), id)
	if err != nil {
		mapServiceError(w, err, "handleCanonicalFunnelAnalysis.get")
		return
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	// project_ids override (comma-separated); else stored default (empty = all).
	projectIDs := funnel.ProjectIDs
	if v := r.URL.Query().Get("project_ids"); v != "" {
		projectIDs = splitCSV(v)
	}

	// Segment override; else stored default.
	segmentName := funnel.Segment
	if _, ok := r.URL.Query()["segment"]; ok {
		segmentName = r.URL.Query().Get("segment")
	}
	seg := parseSegment(segmentName)

	ctx, span := tracing.StartSpan(r.Context(), "overview.canonical_funnels.analyze",
		attribute.String("funnel.id", id),
		attribute.Int("funnel.project_ids.count", len(projectIDs)),
		attribute.String("segment", segmentName),
	)
	defer span.End()

	result, err := s.overview.AnalyzeCanonicalFunnel(ctx, funnel, projectIDs, from, to, seg)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleCanonicalFunnelAnalysis")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"funnel": funnel,
		"result": result,
		"from":   from.Format(time.RFC3339),
		"to":     to.Format(time.RFC3339),
	})
}
