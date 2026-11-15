package agents

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/httpx"
	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// API is what the agents handlers, and the MCP agents tools, need from the
// service.
type API interface {
	LeadTimes(ctx context.Context, projectID string, from, to time.Time, repo string) (AgentLeadTimes, error)
	Scoreboard(ctx context.Context, projectID string, from, to time.Time, session string) (AgentScoreboard, error)
}

var _ API = (*Service)(nil)

// Handler serves the agents delivery report and scoreboard (#331). The API
// server registers the routes, which also accept a project-scoped
// analytics:read token for scripted readouts.
type Handler struct {
	svc API
}

// NewHandler returns the agents handlers over svc.
func NewHandler(svc API) *Handler {
	return &Handler{svc: svc}
}

// LeadTimes reports lead times over ?from/?to (or ?range),
// optionally for one ?repo (owner/name).
func (h *Handler) LeadTimes(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	rng, err := timerange.ParseStrict(r.URL.Query())
	if err != nil {
		httpx.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	repo := r.URL.Query().Get("repo")
	ctx, span := tracing.StartSpan(r.Context(), "agents.leadTimes",
		attribute.String("project.id", projectID), attribute.String("repo", repo))
	defer span.End()

	out, err := h.svc.LeadTimes(ctx, projectID, rng.From, rng.To, repo)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleAgentLeadTimes")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Scoreboard scores the agent sessions over one ISO ?week
// (YYYY-Www, the current week by default) or an explicit ?from/?to,
// optionally for one ?session.
func (h *Handler) Scoreboard(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	q := r.URL.Query()
	var from, to time.Time
	if q.Get("from") != "" || q.Get("to") != "" || q.Get("range") != "" {
		rng, err := timerange.ParseStrict(q)
		if err != nil {
			httpx.JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		from, to = rng.From, rng.To
	} else {
		week := q.Get("week")
		if week == "" {
			week = CurrentISOWeek(time.Now())
		}
		var err error
		if from, to, err = ParseISOWeek(week); err != nil {
			httpx.MapServiceError(w, err, "handleAgentScoreboard")
			return
		}
	}
	session := q.Get("session")
	ctx, span := tracing.StartSpan(r.Context(), "agents.scoreboard",
		attribute.String("project.id", projectID), attribute.String("session", session))
	defer span.End()

	out, err := h.svc.Scoreboard(ctx, projectID, from, to, session)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleAgentScoreboard")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
