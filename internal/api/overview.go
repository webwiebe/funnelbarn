package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// parseOverviewRange parses the shared time-range + environment query params used
// by the cross-project analytics endpoints. Supports ?range=24h|7d|30d or
// explicit ?from=&to= (RFC3339); explicit values override the shorthand.
func parseOverviewRange(r *http.Request) (from, to time.Time, env string) {
	rng := timerange.Parse(r.URL.Query())
	from, to = rng.From, rng.To
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
	visitors, err := s.overview.OverviewVisitorsByProject(ctx, from, to, env, to.Sub(from) <= 48*time.Hour)
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
