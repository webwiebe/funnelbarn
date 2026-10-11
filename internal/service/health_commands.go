package service

import (
	"context"
	"fmt"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
)

// ApplyHealthMark applies a MarkProjectHealth command through the health
// service, which skips the write once a field is known to be set. It is the
// writer's command.Deps.MarkProjectHealth.
func ApplyHealthMark(h ProjectHealth) func(ctx context.Context, projectID, field string) error {
	return func(ctx context.Context, projectID, field string) error {
		switch field {
		case command.HealthSetupCalled:
			return h.MarkSetupCalled(ctx, projectID)
		case command.HealthEventsReceived:
			_, err := h.MarkEventsReceived(ctx, projectID)
			return err
		case command.HealthRecordingsReceived:
			return h.MarkRecordingsReceived(ctx, projectID)
		default:
			return fmt.Errorf("%w: unknown project health field %q", command.ErrPermanent, field)
		}
	}
}

// HealthViaCommands is a reader's project health port (spec 012). Reads go to
// the read-only database; each mark becomes a command for the writer.
type HealthViaCommands struct {
	ports.ProjectHealthRepo
	Bus command.Bus
}

func (h HealthViaCommands) MarkProjectHealthSetupCalled(ctx context.Context, projectID string) error {
	h.Bus.Submit(ctx, command.MarkProjectHealth{ProjectID: projectID, Field: command.HealthSetupCalled})
	return nil
}

func (h HealthViaCommands) MarkProjectHealthEventsReceived(ctx context.Context, projectID string) error {
	h.Bus.Submit(ctx, command.MarkProjectHealth{ProjectID: projectID, Field: command.HealthEventsReceived})
	return nil
}

func (h HealthViaCommands) MarkProjectHealthRecordingsReceived(ctx context.Context, projectID string) error {
	h.Bus.Submit(ctx, command.MarkProjectHealth{ProjectID: projectID, Field: command.HealthRecordingsReceived})
	return nil
}

func (h HealthViaCommands) MarkProjectHealthFlagsEvaluated(ctx context.Context, projectID string) error {
	h.Bus.Submit(ctx, command.MarkFlagsEvaluated{ProjectID: projectID})
	return nil
}
