package api

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// handleDashboard returns aggregate dashboard stats for a project.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	from, to, hourly := dashboardWindow(r)
	rangeParam := r.URL.Query().Get("range")

	env := r.URL.Query().Get("environment")

	ctx, span := tracing.StartSpan(r.Context(), "dashboard.aggregate",
		attribute.String("project.id", projectID),
		attribute.String("range", rangeParam),
		attribute.String("environment", env),
	)
	defer span.End()

	stats, op, err := s.collectDashboard(ctx, projectID, from, to, env, hourly)
	if err != nil {
		// Without span.RecordError, trace search in SpanBarn lists these spans
		// as "ok" even though the request 5xx'd.
		tracing.RecordError(span, err)
		mapServiceError(w, err, op)
		return
	}

	span.SetAttributes(
		attribute.Int64("dashboard.total_events", stats["total_events"].(int64)),
		attribute.Int64("dashboard.unique_sessions", stats["unique_sessions"].(int64)),
	)

	stats["project_id"] = projectID
	stats["from"] = from.Format(time.RFC3339)
	stats["to"] = to.Format(time.RFC3339)
	writeJSON(w, http.StatusOK, stats)
}

// dashboardWindow reads the dashboard window: the last 30 days unless ?range=
// (24h, 7d, 30d) or explicit RFC 3339 ?from= and ?to= say otherwise. hourly is
// true for the 24h and 7d ranges, which chart per hour instead of per day.
func dashboardWindow(r *http.Request) (from, to time.Time, hourly bool) {
	to = time.Now().UTC()
	from = to.AddDate(0, 0, -30)
	rangeParam := r.URL.Query().Get("range")
	switch rangeParam {
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.AddDate(0, 0, -7)
	}
	from, to = overrideWindow(r, from, to)
	return from, to, rangeParam == "24h" || rangeParam == "7d"
}

// dashboardStep is one query of the dashboard. op names it in the error
// mapper and the logs, so a failure stays attributable to the query.
type dashboardStep struct {
	key, op string
	run     func() (any, error)
}

// collectDashboard runs the dashboard queries in order and returns the
// response body keyed by field. On the first failure it returns the failing
// query's op for the error mapper.
func (s *Server) collectDashboard(ctx context.Context, projectID string, from, to time.Time, env string, hourly bool) (map[string]any, string, error) {
	ev := s.events
	steps := []dashboardStep{
		{"total_events", "countEvents", func() (any, error) { return ev.CountEvents(ctx, projectID, from, to, env) }},
		{"unique_sessions", "uniqueSessionCount", func() (any, error) { return ev.UniqueSessionCount(ctx, projectID, from, to, env) }},
		{"top_pages", "topPages", func() (any, error) { return ev.TopPages(ctx, projectID, from, to, 10, env) }},
		{"top_referrers", "topReferrers", func() (any, error) { return ev.TopReferrers(ctx, projectID, from, to, 10, env) }},
		{"events_time_series", "eventCounts", func() (any, error) {
			if hourly {
				return ev.HourlyEventCounts(ctx, projectID, from, to, env)
			}
			return ev.DailyEventCounts(ctx, projectID, from, to, env)
		}},
		{"sessions_time_series", "dailyUniqueSessions", func() (any, error) { return ev.DailyUniqueSessions(ctx, projectID, from, to, env) }},
		{"top_browsers", "topBrowsers", func() (any, error) { return ev.TopBrowsers(ctx, projectID, from, to, 5, env) }},
		{"device_types", "topDeviceTypes", func() (any, error) { return ev.TopDeviceTypes(ctx, projectID, from, to, env) }},
		{"top_event_names", "topEventNames", func() (any, error) { return ev.TopEventNames(ctx, projectID, from, to, 10, env) }},
		{"top_utm_sources", "topUTMSources", func() (any, error) { return ev.TopUTMSources(ctx, projectID, from, to, 5, env) }},
		{"bounce_rate", "bounceRate", func() (any, error) { return ev.BounceRate(ctx, projectID, from, to, env) }},
		{"avg_events_per_session", "avgEventsPerSession", func() (any, error) { return ev.AvgEventsPerSession(ctx, projectID, from, to, env) }},
	}
	out := make(map[string]any, len(steps)+3)
	for _, st := range steps {
		v, err := st.run()
		if err != nil {
			return nil, "handleDashboard." + st.op, err
		}
		out[st.key] = v
	}
	return out, "", nil
}
