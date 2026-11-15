// Package agents is the agent analytics slice: delivery lead times and the
// per-session scoreboard of the agent fleet (#331, #333), from their HTTP
// handlers down to their SQL.
package agents

import "time"

// AgentEvent is one event of the agents project (#331), as the delivery
// scoring reads it: GitHub webhook events and the workstation hook's events.
type AgentEvent struct {
	Name       string
	OccurredAt time.Time
	SessionID  string
	UserIDHash string
	Properties map[string]any
}
