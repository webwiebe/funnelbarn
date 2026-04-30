package api

import (
	"net/http"
	"time"
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
	switch r.URL.Query().Get("range") {
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

	ctx := r.Context()

	totalEvents, err := s.events.CountEvents(ctx, projectID, from, to)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.countEvents")
		return
	}

	uniqueSessions, err := s.events.UniqueSessionCount(ctx, projectID, from, to)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.uniqueSessionCount")
		return
	}

	topPages, err := s.events.TopPages(ctx, projectID, from, to, 10)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.topPages")
		return
	}

	topReferrers, err := s.events.TopReferrers(ctx, projectID, from, to, 10)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.topReferrers")
		return
	}

	timeSeries, err := s.events.DailyEventCounts(ctx, projectID, from, to)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.dailyEventCounts")
		return
	}

	sessionTimeSeries, err := s.events.DailyUniqueSessions(ctx, projectID, from, to)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.dailyUniqueSessions")
		return
	}

	topBrowsers, err := s.events.TopBrowsers(ctx, projectID, from, to, 5)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.topBrowsers")
		return
	}

	deviceTypes, err := s.events.TopDeviceTypes(ctx, projectID, from, to)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.topDeviceTypes")
		return
	}

	topEventNames, err := s.events.TopEventNames(ctx, projectID, from, to, 10)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.topEventNames")
		return
	}

	topUTMSources, err := s.events.TopUTMSources(ctx, projectID, from, to, 5)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.topUTMSources")
		return
	}

	bounceRate, err := s.events.BounceRate(ctx, projectID, from, to)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.bounceRate")
		return
	}

	avgEventsPerSession, err := s.events.AvgEventsPerSession(ctx, projectID, from, to)
	if err != nil {
		mapServiceError(w, err, "handleDashboard.avgEventsPerSession")
		return
	}

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
