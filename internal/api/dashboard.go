package api

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// handleDashboard returns aggregate dashboard stats for a project.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	// Parse time range. Supports ?range=24h|7d|30d or explicit ?from=&to= (RFC3339).
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	rangeParam := r.URL.Query().Get("range")
	switch rangeParam {
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.AddDate(0, 0, -7)
	case "30d":
		from = to.AddDate(0, 0, -30)
	}
	// Explicit from/to override the range shorthand.
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

	env := r.URL.Query().Get("environment")

	ctx, span := tracing.StartSpan(r.Context(), "dashboard.aggregate",
		attribute.String("project.id", projectID),
		attribute.String("range", rangeParam),
		attribute.String("environment", env),
	)
	defer span.End()

	// recordOnErr surfaces an error onto the span and falls through to the
	// standard service-error mapper. Without span.RecordError, trace search
	// in SpanBarn lists these spans as "ok" even though the request 5xx'd.
	recordOnErr := func(err error, op string) bool {
		if err == nil {
			return false
		}
		tracing.RecordError(span, err)
		mapServiceError(w, err, op)
		return true
	}

	totalEvents, err := s.events.CountEvents(ctx, projectID, from, to, env)
	if recordOnErr(err, "handleDashboard.countEvents") {
		return
	}

	uniqueSessions, err := s.events.UniqueSessionCount(ctx, projectID, from, to, env)
	if recordOnErr(err, "handleDashboard.uniqueSessionCount") {
		return
	}

	topPages, err := s.events.TopPages(ctx, projectID, from, to, 10, env)
	if recordOnErr(err, "handleDashboard.topPages") {
		return
	}

	topReferrers, err := s.events.TopReferrers(ctx, projectID, from, to, 10, env)
	if recordOnErr(err, "handleDashboard.topReferrers") {
		return
	}

	var timeSeries []repository.TimeSeriesPoint
	if rangeParam == "24h" || rangeParam == "7d" {
		timeSeries, err = s.events.HourlyEventCounts(ctx, projectID, from, to, env)
	} else {
		timeSeries, err = s.events.DailyEventCounts(ctx, projectID, from, to, env)
	}
	if recordOnErr(err, "handleDashboard.eventCounts") {
		return
	}

	sessionTimeSeries, err := s.events.DailyUniqueSessions(ctx, projectID, from, to, env)
	if recordOnErr(err, "handleDashboard.dailyUniqueSessions") {
		return
	}

	topBrowsers, err := s.events.TopBrowsers(ctx, projectID, from, to, 5, env)
	if recordOnErr(err, "handleDashboard.topBrowsers") {
		return
	}

	deviceTypes, err := s.events.TopDeviceTypes(ctx, projectID, from, to, env)
	if recordOnErr(err, "handleDashboard.topDeviceTypes") {
		return
	}

	topEventNames, err := s.events.TopEventNames(ctx, projectID, from, to, 10, env)
	if recordOnErr(err, "handleDashboard.topEventNames") {
		return
	}

	topUTMSources, err := s.events.TopUTMSources(ctx, projectID, from, to, 5, env)
	if recordOnErr(err, "handleDashboard.topUTMSources") {
		return
	}

	bounceRate, err := s.events.BounceRate(ctx, projectID, from, to, env)
	if recordOnErr(err, "handleDashboard.bounceRate") {
		return
	}

	avgEventsPerSession, err := s.events.AvgEventsPerSession(ctx, projectID, from, to, env)
	if recordOnErr(err, "handleDashboard.avgEventsPerSession") {
		return
	}

	span.SetAttributes(
		attribute.Int64("dashboard.total_events", totalEvents),
		attribute.Int64("dashboard.unique_sessions", uniqueSessions),
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":             projectID,
		"from":                   from.Format(time.RFC3339),
		"to":                     to.Format(time.RFC3339),
		"total_events":           totalEvents,
		"unique_sessions":        uniqueSessions,
		"bounce_rate":            bounceRate,
		"avg_events_per_session": avgEventsPerSession,
		"top_pages":              topPages,
		"top_referrers":          topReferrers,
		"top_browsers":           topBrowsers,
		"device_types":           deviceTypes,
		"top_event_names":        topEventNames,
		"top_utm_sources":        topUTMSources,
		"events_time_series":     timeSeries,
		"sessions_time_series":   sessionTimeSeries,
	})
}
