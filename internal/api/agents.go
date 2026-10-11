package api

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// registerAgentRoutes serves the agents delivery report and scoreboard
// (#331). Like the other read-only analytics routes they also accept a
// project-scoped analytics:read token, for scripted readouts.
func (s *Server) registerAgentRoutes() {
	if s.agents == nil {
		return
	}
	s.mux.HandleFunc("GET /api/v1/projects/{id}/agents/lead-times", s.requireSessionOrToken(repository.APIKeyScopeAnalyticsRead, s.handleAgentLeadTimes))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/agents/scoreboard", s.requireSessionOrToken(repository.APIKeyScopeAnalyticsRead, s.handleAgentScoreboard))
}

// handleAgentLeadTimes reports lead times over ?from/?to (or ?range),
// optionally for one ?repo (owner/name).
func (s *Server) handleAgentLeadTimes(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	rng, err := timerange.ParseStrict(r.URL.Query())
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	repo := r.URL.Query().Get("repo")
	ctx, span := tracing.StartSpan(r.Context(), "agents.leadTimes",
		attribute.String("project.id", projectID), attribute.String("repo", repo))
	defer span.End()

	out, err := s.agents.LeadTimes(ctx, projectID, rng.From, rng.To, repo)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleAgentLeadTimes")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAgentScoreboard scores the agent sessions over one ISO ?week
// (YYYY-Www, the current week by default) or an explicit ?from/?to,
// optionally for one ?session.
func (s *Server) handleAgentScoreboard(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	q := r.URL.Query()
	var from, to time.Time
	if q.Get("from") != "" || q.Get("to") != "" || q.Get("range") != "" {
		rng, err := timerange.ParseStrict(q)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		from, to = rng.From, rng.To
	} else {
		week := q.Get("week")
		if week == "" {
			week = service.CurrentISOWeek(time.Now())
		}
		var err error
		if from, to, err = service.ParseISOWeek(week); err != nil {
			mapServiceError(w, err, "handleAgentScoreboard")
			return
		}
	}
	session := q.Get("session")
	ctx, span := tracing.StartSpan(r.Context(), "agents.scoreboard",
		attribute.String("project.id", projectID), attribute.String("session", session))
	defer span.End()

	out, err := s.agents.Scoreboard(ctx, projectID, from, to, session)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleAgentScoreboard")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
