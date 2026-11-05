package api

import (
	"net/http"
	"net/url"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// handlePageFlows returns the Sankey flow graph for a project, centered on the
// given page. Accepts the same ?range/from/to params as the dashboard, plus
// ?page= (URL of the focused page) and ?depth= (1-10, default 5).
// parseFlowsRange returns the page-flows window ending at now. range picks 24h,
// 7d or 30d (the default); from and to, as RFC 3339 times, override either end.
// A value that does not parse is ignored.
func parseFlowsRange(q url.Values, now time.Time) (from, to time.Time) {
	to = now
	from = to.AddDate(0, 0, -30)
	switch q.Get("range") {
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.AddDate(0, 0, -7)
	}
	if t, err := time.Parse(time.RFC3339, q.Get("from")); err == nil {
		from = t
	}
	if t, err := time.Parse(time.RFC3339, q.Get("to")); err == nil {
		to = t
	}
	return from, to
}

// parseFlowsDepth reads the flow depth from its digits, ignoring any other
// character. It returns 5 unless the result is 1 to 10.
func parseFlowsDepth(v string) int {
	n := 0
	for _, c := range v {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	if n >= 1 && n <= 10 {
		return n
	}
	return 5
}

func (s *Server) handlePageFlows(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	from, to := parseFlowsRange(q, time.Now().UTC())
	page := q.Get("page")
	env := q.Get("environment")
	depth := parseFlowsDepth(q.Get("depth"))

	ctx, span := tracing.StartSpan(r.Context(), "flows.pageFlows",
		attribute.String("project.id", projectID),
		attribute.String("page", page),
		attribute.Int("depth", depth),
		attribute.String("environment", env),
	)
	defer span.End()

	result, err := s.events.PageFlows(ctx, projectID, page, depth, from, to, env)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handlePageFlows")
		return
	}

	span.SetAttributes(
		attribute.Int("flows.node_count", len(result.Nodes)),
		attribute.Int("flows.link_count", len(result.Links)),
	)

	writeJSON(w, http.StatusOK, result)
}
