package domain

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
