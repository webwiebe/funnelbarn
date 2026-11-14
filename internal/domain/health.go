package domain

import "time"

// ProjectHealth holds the integration health status for a project.
type ProjectHealth struct {
	ProjectID          string    `json:"project_id"`
	SetupCalled        bool      `json:"setup_called"`
	EventsReceived     bool      `json:"events_received"`
	FlagsEvaluated     bool      `json:"flags_evaluated"`
	RecordingsReceived bool      `json:"recordings_received"`
	UpdatedAt          time.Time `json:"updated_at"`
}
