package command

import (
	"context"
	"errors"
)

// KindMarkProjectHealth is the kind of MarkProjectHealth.
const KindMarkProjectHealth = "mark_project_health"

// Project health fields MarkProjectHealth sets. Flag evaluation has its own
// command, MarkFlagsEvaluated.
const (
	HealthSetupCalled        = "setup_called"
	HealthEventsReceived     = "events_received"
	HealthRecordingsReceived = "recordings_received"
)

// MarkProjectHealth sets one project_health field to true. A reader process
// submits it instead of writing; setting a field that is already true changes
// nothing, so a redelivery is harmless.
type MarkProjectHealth struct {
	ProjectID string `json:"project_id"`
	Field     string `json:"field"`
}

// Kind implements Command.
func (MarkProjectHealth) Kind() string { return KindMarkProjectHealth }

// Project implements Command.
func (c MarkProjectHealth) Project() string { return c.ProjectID }

// Apply implements Command.
func (c MarkProjectHealth) Apply(ctx context.Context, d Deps) error {
	if d.MarkProjectHealth == nil {
		return errors.New("mark project health: no marker configured")
	}
	return d.MarkProjectHealth(ctx, c.ProjectID, c.Field)
}
