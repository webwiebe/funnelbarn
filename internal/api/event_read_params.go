package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/environment"
)

// The query parameters each token-readable event read understands. Any other
// parameter is a 400: an ignored filter answers 200 with the wrong number, and
// a script has no way to notice.
var (
	listEventsParams     = []string{"limit", "offset", "name", "environment", "from", "to", "stream"}
	eventCountsParams    = []string{"range", "from", "to", "limit", "environment", "exclude_bots", "property", "value"}
	eventBreakdownParams = []string{"event", "by", "range", "from", "to", "environment", "exclude_bots", "property", "value"}
)

// rejectUnknownParams returns an error naming the first parameter in q, in
// sorted order, that allowed does not list.
func rejectUnknownParams(q url.Values, allowed []string) error {
	var unknown []string
	for k := range q {
		if !slices.Contains(allowed, k) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("unknown query parameter %q (use %s)", unknown[0], strings.Join(allowed, ", "))
}

// resolveEnvironment turns an environment query value into the value stored on
// events. Empty keeps every environment. A canonical name or a known alias
// ("prod") maps to the canonical name, as ingest and funnel analysis do. Any
// other value must be one the project has recorded events under, the list
// GET /environments serves.
//
// For an unknown value it returns ok=false and the values that are valid: the
// canonical names plus the project's recorded ones. A canonical name the
// project has no events under yet is valid and counts zero.
func (s *Server) resolveEnvironment(ctx context.Context, projectID, raw string) (env string, valid []string, ok bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, true, nil
	}
	if environment.IsKnown(raw) {
		return environment.Normalize(raw), nil, true, nil
	}
	recorded, err := s.events.DistinctEnvironments(ctx, projectID)
	if err != nil {
		return "", nil, false, err
	}
	if slices.Contains(recorded, raw) {
		return raw, nil, true, nil
	}
	valid = environment.Canonical()
	for _, e := range recorded {
		if !slices.Contains(valid, e) {
			valid = append(valid, e)
		}
	}
	return "", valid, false, nil
}

// environmentParam resolves the environment query parameter for a handler. On
// an unknown value or a failed lookup it writes the response and returns
// ok=false.
func (s *Server) environmentParam(w http.ResponseWriter, r *http.Request, projectID, op string) (string, bool) {
	raw := r.URL.Query().Get("environment")
	env, valid, ok, err := s.resolveEnvironment(r.Context(), projectID, raw)
	if err != nil {
		mapServiceError(w, err, op)
		return "", false
	}
	if !ok {
		jsonError(w, fmt.Sprintf("unknown environment %q (use %s)", strings.TrimSpace(raw), strings.Join(valid, ", ")), http.StatusBadRequest)
		return "", false
	}
	return env, true
}
