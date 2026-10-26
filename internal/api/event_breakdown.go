package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// parseCountFilter reads environment, exclude_bots and the property/value pair
// shared by event-counts and event-breakdown. exclude_bots must be true or false.
func parseCountFilter(q url.Values) (repository.EventCountFilter, error) {
	f := repository.EventCountFilter{Environment: q.Get("environment")}
	if v := q.Get("exclude_bots"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return f, errors.New("exclude_bots must be true or false")
		}
		f.ExcludeBots = b
	}
	if key := q.Get("property"); key != "" {
		if !repository.ValidPropertyKey(key) {
			return f, errors.New("property must be letters, digits and underscores, at most 64 characters")
		}
		if !q.Has("value") {
			return f, errors.New("property needs a value parameter")
		}
		f.PropKey, f.PropValue = key, q.Get("value")
	} else if q.Has("value") {
		return f, errors.New("value needs a property parameter")
	}
	return f, nil
}

// handleEventBreakdown counts one event by device_type, referrer_domain or
// utm_source: events and distinct sessions per value. It answers the question
// behind a funnel drop-off ("who signs up, and from where") with one read-only
// call.
func (s *Server) handleEventBreakdown(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	q := r.URL.Query()
	event := q.Get("event")
	if event == "" {
		jsonError(w, "event is required", http.StatusBadRequest)
		return
	}
	by := q.Get("by")
	if _, ok := repository.BreakdownDimensions[by]; !ok {
		jsonError(w, fmt.Sprintf("by must be one of device_type, referrer_domain, utm_source, got %q", by), http.StatusBadRequest)
		return
	}
	rng, err := timerange.ParseStrict(q)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	filter, err := parseCountFilter(q)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "events.breakdown",
		attribute.String("project.id", projectID),
		attribute.String("event.name", event),
		attribute.String("breakdown.by", by),
	)
	defer span.End()

	rows, err := s.events.EventBreakdown(ctx, projectID, event, by, rng.From, rng.To, 100, filter)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleEventBreakdown")
		return
	}
	if rows == nil {
		rows = []repository.BreakdownRow{}
	}
	var total int64
	for _, row := range rows {
		total += row.Count
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":   projectID,
		"event":        event,
		"by":           by,
		"from":         rng.From.Format(time.RFC3339),
		"to":           rng.To.Format(time.RFC3339),
		"environment":  filter.Environment,
		"exclude_bots": filter.ExcludeBots,
		"total_events": total,
		"values":       rows,
	})
}
