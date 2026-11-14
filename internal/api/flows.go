package api

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

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

// handlePageFlows returns the Sankey flow graph for a project, centered on the
// given page. Accepts the same ?range/from/to params as the dashboard, plus
// ?page= (URL of the focused page) and ?depth= (1-10, default 5).
func (s *Server) handlePageFlows(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	rng := timerange.ParseAt(q, time.Now().UTC())
	from, to := rng.From, rng.To
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
