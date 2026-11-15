package api

import (
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/features/agents"
)

// AgentsAPI is what the server needs from the agents slice's service.
type AgentsAPI = agents.API

// agentsWiring holds the agents service for the MCP tools and the handlers
// for the routes. Both are nil when the server has no agents service, which
// leaves the routes unregistered.
type agentsWiring struct {
	api agents.API
	*agents.Handler
}

func newAgentsWiring(api agents.API) agentsWiring {
	if api == nil {
		return agentsWiring{}
	}
	return agentsWiring{api: api, Handler: agents.NewHandler(api)}
}

// registerAgentRoutes serves the agents delivery report and scoreboard
// (#331). Like the other read-only analytics routes they also accept a
// project-scoped analytics:read token, for scripted readouts.
func (s *Server) registerAgentRoutes() {
	if s.agents.Handler == nil {
		return
	}
	s.mux.HandleFunc("GET /api/v1/projects/{id}/agents/lead-times", s.requireSessionOrToken(domain.APIKeyScopeAnalyticsRead, s.agents.LeadTimes))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/agents/scoreboard", s.requireSessionOrToken(domain.APIKeyScopeAnalyticsRead, s.agents.Scoreboard))
}
