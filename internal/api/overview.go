package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// parseOverviewRange parses the shared time-range + environment query params used
// by the cross-project analytics endpoints. Supports ?range=24h|7d|30d or
// explicit ?from=&to= (RFC3339); explicit values override the shorthand.
func parseOverviewRange(r *http.Request) (from, to time.Time, env string) {
	to = time.Now().UTC()
	from = to.AddDate(0, 0, -30)
	switch r.URL.Query().Get("range") {
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.AddDate(0, 0, -7)
	case "30d":
		from = to.AddDate(0, 0, -30)
	}
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
	return from, to, r.URL.Query().Get("environment")
}

// handleOverview returns instance-wide analytics: totals, per-project rollups,
// the visitors-per-site series, and top pages/referrers/countries.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	from, to, env := parseOverviewRange(r)

	ctx, span := tracing.StartSpan(r.Context(), "overview.aggregate",
		attribute.String("environment", env),
	)
	defer span.End()

	// recordOnErr surfaces an error onto the span and falls through to the
	// standard service-error mapper, so a 5xx here isn't mislabeled "ok" in
	// trace search.
	recordOnErr := func(err error, op string) bool {
		if err == nil {
			return false
		}
		tracing.RecordError(span, err)
		mapServiceError(w, err, op)
		return true
	}

	totalEvents, totalSessions, err := s.overview.OverviewTotals(ctx, from, to, env)
	if recordOnErr(err, "handleOverview.totals") {
		return
	}
	projects, err := s.overview.ProjectRollups(ctx, from, to, env)
	if recordOnErr(err, "handleOverview.projectRollups") {
		return
	}
	visitors, err := s.overview.OverviewVisitorsByProjectDaily(ctx, from, to, env)
	if recordOnErr(err, "handleOverview.visitors") {
		return
	}
	topPages, err := s.overview.OverviewTopPages(ctx, from, to, 10, env)
	if recordOnErr(err, "handleOverview.topPages") {
		return
	}
	topReferrers, err := s.overview.OverviewTopReferrers(ctx, from, to, 10, env)
	if recordOnErr(err, "handleOverview.topReferrers") {
		return
	}
	topCountries, err := s.overview.OverviewTopCountries(ctx, from, to, 10, env)
	if recordOnErr(err, "handleOverview.topCountries") {
		return
	}

	span.SetAttributes(
		attribute.Int64("overview.total_events", totalEvents),
		attribute.Int64("overview.total_sessions", totalSessions),
		attribute.Int("overview.projects.count", len(projects)),
	)

	// Optional dimension breakdown for the "per type" view.
	var dimension []repository.DimensionStat
	if dim := r.URL.Query().Get("dimension"); dim != "" {
		span.SetAttributes(attribute.String("overview.dimension", dim))
		dimension, err = s.overview.OverviewDimensionBreakdown(ctx, dim, from, to, 10, env)
		if err != nil {
			tracing.RecordError(span, err)
			jsonError(w, "unsupported dimension", http.StatusBadRequest)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"from":                from.Format(time.RFC3339),
		"to":                  to.Format(time.RFC3339),
		"total_events":        totalEvents,
		"unique_sessions":     totalSessions,
		"projects":            projects,
		"visitors_by_project": visitors,
		"top_pages":           topPages,
		"top_referrers":       topReferrers,
		"top_countries":       topCountries,
		"dimension_breakdown": dimension,
	})
}

// handleOverviewEvents returns a keyset-paginated event feed across all projects.
func (s *Server) handleOverviewEvents(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	f := repository.EventFilter{
		ProjectID:   r.URL.Query().Get("project_id"),
		Name:        r.URL.Query().Get("name"),
		Environment: r.URL.Query().Get("environment"),
		CursorID:    r.URL.Query().Get("cursor_id"),
	}
	if v := r.URL.Query().Get("cursor_time"); v != "" {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			f.CursorOccurredAt = t
		}
	}

	ctx, span := tracing.StartSpan(r.Context(), "overview.events",
		attribute.String("project.id", f.ProjectID),
		attribute.String("event.name", f.Name),
		attribute.String("environment", f.Environment),
		attribute.Int("limit", limit),
	)
	defer span.End()

	events, err := s.overview.ListAllEvents(ctx, f, limit)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleOverviewEvents")
		return
	}
	if events == nil {
		events = []repository.Event{}
	}
	span.SetAttributes(attribute.Int("events.count", len(events)))

	// Cursor for the next page: the last row's (occurred_at, id). Present only
	// when the page was full (more rows may follow).
	var next map[string]string
	if len(events) == limit {
		last := events[len(events)-1]
		next = map[string]string{
			"cursor_time": last.OccurredAt.UTC().Format(time.RFC3339Nano),
			"cursor_id":   last.ID,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":      events,
		"next_cursor": next,
	})
}

// ---- Canonical event catalog ----

func (s *Server) handleListCanonicalEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.overview.ListCanonicalEvents(r.Context())
	if err != nil {
		mapServiceError(w, err, "handleListCanonicalEvents")
		return
	}
	if events == nil {
		events = []repository.CanonicalEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"canonical_events": events})
}

func (s *Server) handleCreateCanonicalEvent(w http.ResponseWriter, r *http.Request) {
	var body repository.CanonicalEvent
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	created, err := s.overview.CreateCanonicalEvent(r.Context(), body)
	if err != nil {
		mapServiceError(w, err, "handleCreateCanonicalEvent")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateCanonicalEvent(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		jsonError(w, "key required", http.StatusBadRequest)
		return
	}
	var body repository.CanonicalEvent
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	body.Key = key
	updated, err := s.overview.UpdateCanonicalEvent(r.Context(), body)
	if err != nil {
		mapServiceError(w, err, "handleUpdateCanonicalEvent")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteCanonicalEvent(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		jsonError(w, "key required", http.StatusBadRequest)
		return
	}
	if err := s.overview.DeleteCanonicalEvent(r.Context(), key); err != nil {
		mapServiceError(w, err, "handleDeleteCanonicalEvent")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Per-project mappings ----

func (s *Server) handleListMappings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "overview.mappings.list",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	mappings, err := s.overview.ListMappings(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleListMappings")
		return
	}
	if mappings == nil {
		mappings = []repository.EventNameMapping{}
	}
	span.SetAttributes(attribute.Int("mappings.count", len(mappings)))
	writeJSON(w, http.StatusOK, map[string]any{"mappings": mappings})
}

func (s *Server) handleSetMappings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	var body struct {
		Mappings []repository.EventNameMapping `json:"mappings"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "overview.mappings.set",
		attribute.String("project.id", projectID),
		attribute.Int("mappings.count", len(body.Mappings)),
	)
	defer span.End()

	if err := s.overview.SetMappings(ctx, projectID, body.Mappings); err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleSetMappings")
		return
	}
	// Return the fresh mapping set.
	mappings, err := s.overview.ListMappings(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleSetMappings.list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mappings": mappings})
}

func (s *Server) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	raw := r.PathValue("raw")
	if projectID == "" || raw == "" {
		jsonError(w, "project id and raw name required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "overview.mappings.delete",
		attribute.String("project.id", projectID),
		attribute.String("mapping.raw", raw),
	)
	defer span.End()

	if err := s.overview.DeleteMapping(ctx, projectID, raw); err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleDeleteMapping")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMappingSuggestions(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	ctx, span := tracing.StartSpan(r.Context(), "overview.mappings.suggestions",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	suggestions, err := s.overview.MappingSuggestions(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleMappingSuggestions")
		return
	}
	if suggestions == nil {
		suggestions = []repository.MappingSuggestion{}
	}
	span.SetAttributes(attribute.Int("suggestions.count", len(suggestions)))
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}

// ---- Canonical funnels ----

func (s *Server) handleListCanonicalFunnels(w http.ResponseWriter, r *http.Request) {
	funnels, err := s.overview.ListCanonicalFunnels(r.Context())
	if err != nil {
		mapServiceError(w, err, "handleListCanonicalFunnels")
		return
	}
	if funnels == nil {
		funnels = []repository.CanonicalFunnel{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"funnels": funnels})
}

// canonicalFunnelBody is the request shape for creating/updating a canonical funnel.
type canonicalFunnelBody struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description"`
	Scope       string                           `json:"scope"`
	ProjectIDs  []string                         `json:"project_ids"`
	Segment     string                           `json:"segment"`
	Steps       []repository.CanonicalFunnelStep `json:"steps"`
}

func (b canonicalFunnelBody) toFunnel(id string) repository.CanonicalFunnel {
	scope := b.Scope
	if scope == "" {
		scope = "session"
	}
	return repository.CanonicalFunnel{
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

// splitCSV splits a comma-separated list, trimming spaces and dropping empties.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
