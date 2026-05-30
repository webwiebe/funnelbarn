package api

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// handlePageFlows returns the Sankey flow graph for a project, centered on the
// given page. Accepts the same ?range/from/to params as the dashboard, plus
// ?page= (URL of the focused page) and ?depth= (1-10, default 5).
func (s *Server) handlePageFlows(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

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

	page := r.URL.Query().Get("page")

	depth := 5
	if v := r.URL.Query().Get("depth"); v != "" {
		n := 0
		for _, c := range v {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		if n >= 1 && n <= 10 {
			depth = n
		}
	}

	ctx, span := tracing.StartSpan(r.Context(), "flows.pageFlows",
		attribute.String("project.id", projectID),
		attribute.String("page", page),
	)
	defer span.End()

	result, err := s.events.PageFlows(ctx, projectID, page, depth, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handlePageFlows")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
