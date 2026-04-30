package api

import (
	"net/http"

	"github.com/wiebe-xyz/funnelbarn/internal/apierr"
	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
)

// handleDashboard returns aggregate dashboard stats for a project.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		apierr.WriteHTTP(w, apierr.BadRequest("project id required"))
		return
	}

	tr := timerange.Parse(r.URL.Query())
	ctx := r.Context()

	totalEvents, err := s.events.CountEvents(ctx, projectID, tr.From, tr.To)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	uniqueSessions, err := s.events.UniqueSessionCount(ctx, projectID, tr.From, tr.To)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	topPages, err := s.events.TopPages(ctx, projectID, tr.From, tr.To, 10)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	topReferrers, err := s.events.TopReferrers(ctx, projectID, tr.From, tr.To, 10)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	timeSeries, err := s.events.DailyEventCounts(ctx, projectID, tr.From, tr.To)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	sessionTimeSeries, err := s.events.DailyUniqueSessions(ctx, projectID, tr.From, tr.To)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	topBrowsers, err := s.events.TopBrowsers(ctx, projectID, tr.From, tr.To, 5)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	deviceTypes, err := s.events.TopDeviceTypes(ctx, projectID, tr.From, tr.To)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	topEventNames, err := s.events.TopEventNames(ctx, projectID, tr.From, tr.To, 10)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	topUTMSources, err := s.events.TopUTMSources(ctx, projectID, tr.From, tr.To, 5)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	bounceRate, err := s.events.BounceRate(ctx, projectID, tr.From, tr.To)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	avgEventsPerSession, err := s.events.AvgEventsPerSession(ctx, projectID, tr.From, tr.To)
	if err != nil {
		apierr.WriteHTTP(w, apierr.Internal())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":             projectID,
		"from":                   tr.From.Format("2006-01-02T15:04:05Z07:00"),
		"to":                     tr.To.Format("2006-01-02T15:04:05Z07:00"),
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
