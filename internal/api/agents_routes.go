package api

import (
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/features/agents"
)

// newAgentsHandler returns nil when the server has no agents service, which
// leaves the routes unregistered.
func newAgentsHandler(api agents.API) *agents.Handler {
	if api == nil {
		return nil
	}
	return agents.NewHandler(api)
}

// registerAgentRoutes serves the agents delivery report and scoreboard
// (#331). Like the other read-only analytics routes they also accept a
// project-scoped analytics:read token, for scripted readouts.
func (s *Server) registerAgentRoutes() {
	if s.agents == nil {
		return
	}
	s.mux.HandleFunc("GET /api/v1/projects/{id}/agents/lead-times", s.requireSessionOrToken(domain.APIKeyScopeAnalyticsRead, s.agents.LeadTimes))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/agents/scoreboard", s.requireSessionOrToken(domain.APIKeyScopeAnalyticsRead, s.agents.Scoreboard))
}
