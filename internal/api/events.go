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

// addPaginationHeaders sets a Link header for the next page when more results
// may be available (i.e. the returned count equals the page limit).
func addPaginationHeaders(w http.ResponseWriter, r *http.Request, limit, offset, count int) {
	if count < limit {
		return // no next page
	}
	nextOffset := offset + limit
	q := r.URL.Query()
	q.Set("offset", strconv.Itoa(nextOffset))
	q.Set("limit", strconv.Itoa(limit))
	next := r.URL.Path + "?" + q.Encode()
	w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next))
}

// handleEventNames returns distinct event names for a project (autocomplete).
func (s *Server) handleEventNames(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	names, err := s.events.DistinctEventNames(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleEventNames")
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"event_names": names})
}

// handleEventCounts returns per-event-name counts over a date range — the
// smallest thing a scheduled readout needs to answer "how many signups last
// week". The dashboard payload carries the same numbers, but only its top ten
// and wrapped in a dozen other aggregates; this is the one query, on its own,
// reachable with a read-only analytics:read token.
func (s *Server) handleEventCounts(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	rng, err := timerange.ParseStrict(r.URL.Query())
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Unlike the dashboard's fixed top-10, a readout wants the whole catalog by
	// default; the ceiling only exists to bound the response.
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 500 {
			jsonError(w, "limit must be an integer between 1 and 500", http.StatusBadRequest)
			return
		}
		limit = n
	}

	filter, err := parseCountFilter(r.URL.Query())
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	env := filter.Environment

	ctx, span := tracing.StartSpan(r.Context(), "events.counts",
		attribute.String("project.id", projectID),
		attribute.Int("limit", limit),
		attribute.String("environment", env),
	)
	defer span.End()

	counts, err := s.events.TopEventNamesFiltered(ctx, projectID, rng.From, rng.To, limit, filter)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleEventCounts")
		return
	}
	if counts == nil {
		counts = []repository.EventNameStat{}
	}

	var total int64
	for _, c := range counts {
		total += c.Count
	}
	span.SetAttributes(attribute.Int("event.result_count", len(counts)))

	body := map[string]any{
		"project_id":   projectID,
		"from":         rng.From.Format(time.RFC3339),
		"to":           rng.To.Format(time.RFC3339),
		"environment":  env,
		"limit":        limit,
		"events":       counts,
		"total_events": total,
	}
	if filter.ExcludeBots {
		body["exclude_bots"] = true
	}
	writeJSON(w, http.StatusOK, body)
}

// handleEventProperties returns distinct JSON property keys for a given event name.
func (s *Server) handleEventProperties(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	eventName := r.URL.Query().Get("event_name")
	if eventName == "" {
		jsonError(w, "event_name query parameter required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "events.properties",
		attribute.String("project.id", projectID),
		attribute.String("event.name", eventName),
	)
	defer span.End()

	props, err := s.events.DistinctEventProperties(ctx, projectID, eventName)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleEventProperties")
		return
	}
	if props == nil {
		props = []string{}
	}
	populated, err := s.events.PopulatedMetadataColumns(ctx, projectID, eventName)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleEventProperties.metadata")
		return
	}
	all := append(populated, props...)
	span.SetAttributes(attribute.Int("event.property_count", len(all)))
	writeJSON(w, http.StatusOK, map[string]any{"properties": all})
}

// handleEventPropertyValues returns distinct values for a property key on a given event name.
func (s *Server) handleEventPropertyValues(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	eventName := r.URL.Query().Get("event_name")
	if eventName == "" {
		jsonError(w, "event_name query parameter required", http.StatusBadRequest)
		return
	}
	property := r.URL.Query().Get("property")
	if property == "" {
		jsonError(w, "property query parameter required", http.StatusBadRequest)
		return
	}

	vals, err := s.events.DistinctPropertyValues(r.Context(), projectID, eventName, property, 50)
	if err != nil {
		mapServiceError(w, err, "handleEventPropertyValues")
		return
	}
	if vals == nil {
		vals = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"values": vals})
}

// handleEnvironments returns the distinct canonical environment values recorded for a project.
func (s *Server) handleEnvironments(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	envs, err := s.events.DistinctEnvironments(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleEnvironments")
		return
	}
	if envs == nil {
		envs = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"environments": envs})
}

// listEventsParams are the query parameters /events understands. Any other
// parameter is rejected: an ignored filter returns the newest events of every
// kind, which a script reads as a wrong number. "stream" is sent by the live
// view and has no effect.
var listEventsParams = map[string]bool{
	"limit": true, "offset": true, "name": true, "environment": true,
	"from": true, "to": true, "stream": true,
}

// parseListEvents reads and validates the /events query string.
func parseListEvents(q url.Values) (f repository.EventListFilter, limit, offset int, err error) {
	for k := range q {
		if !listEventsParams[k] {
			return f, 0, 0, fmt.Errorf("unknown query parameter %q (use limit, offset, name, environment, from, to)", k)
		}
	}
	if limit, offset, err = parsePaging(q); err != nil {
		return f, 0, 0, err
	}
	f.Name = q.Get("name")
	f.Environment = q.Get("environment")
	if f.From, err = parseOptionalTime(q, "from"); err != nil {
		return f, 0, 0, err
	}
	if f.To, err = parseOptionalTime(q, "to"); err != nil {
		return f, 0, 0, err
	}
	if !f.From.IsZero() && !f.To.IsZero() && !f.To.After(f.From) {
		return f, 0, 0, errors.New("to must be after from")
	}
	return f, limit, offset, nil
}

func parsePaging(q url.Values) (limit, offset int, err error) {
	limit = 50
	if v := q.Get("limit"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n <= 0 || n > 500 {
			return 0, 0, errors.New("limit must be an integer between 1 and 500")
		}
		limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 0 {
			return 0, 0, errors.New("offset must be a non-negative integer")
		}
		offset = n
	}
	return limit, offset, nil
}

func parseOptionalTime(q url.Values, key string) (time.Time, error) {
	v := q.Get(key)
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be an RFC3339 timestamp", key)
	}
	return t.UTC(), nil
}

// handleListEvents returns a paginated list of events for a project, newest
// first, optionally narrowed by name, environment and an occurred_at window.
func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	filter, limit, offset, err := parseListEvents(r.URL.Query())
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "events.list",
		attribute.String("project.id", projectID),
		attribute.Int("limit", limit),
		attribute.Int("offset", offset),
		attribute.String("event.name", filter.Name),
		attribute.String("environment", filter.Environment),
	)
	defer span.End()

	events, err := s.events.ListEventsFiltered(ctx, projectID, filter, limit, offset)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleListEvents")
		return
	}

	span.SetAttributes(attribute.Int("event.result_count", len(events)))
	addPaginationHeaders(w, r, limit, offset, len(events))
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"limit":  limit,
		"offset": offset,
	})
}
