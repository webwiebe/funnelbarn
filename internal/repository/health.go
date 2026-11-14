package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/sqlcgen"
)

func projectHealthFromGen(g sqlcgen.ProjectHealth) domain.ProjectHealth {
	return domain.ProjectHealth{
		ProjectID:          g.ProjectID,
		SetupCalled:        g.SetupCalled != 0,
		EventsReceived:     g.EventsReceived != 0,
		FlagsEvaluated:     g.FlagsEvaluated != 0,
		RecordingsReceived: g.RecordingsReceived != 0,
		UpdatedAt:          g.UpdatedAt,
	}
}

// GetProjectHealth returns the health row for projectID.
// If no row exists (project has never triggered any health event) a zeroed
// ProjectHealth is returned with no error.
func (r *ReadStore) GetProjectHealth(ctx context.Context, projectID string) (domain.ProjectHealth, error) {
	row, err := r.rq.GetProjectHealth(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProjectHealth{ProjectID: projectID}, nil
	}
	if err != nil {
		return domain.ProjectHealth{}, err
	}
	return projectHealthFromGen(row), nil
}

// MarkProjectHealthSetupCalled records that the setup endpoint was called.
func (s *Store) MarkProjectHealthSetupCalled(ctx context.Context, projectID string) error {
	return s.q.MarkProjectHealthSetupCalled(ctx, projectID)
}

// MarkProjectHealthEventsReceived records that at least one event was ingested.
func (s *Store) MarkProjectHealthEventsReceived(ctx context.Context, projectID string) error {
	return s.q.MarkProjectHealthEventsReceived(ctx, projectID)
}

// MarkProjectHealthFlagsEvaluated records that the flag evaluation endpoint was called.
func (s *Store) MarkProjectHealthFlagsEvaluated(ctx context.Context, projectID string) error {
	return s.q.MarkProjectHealthFlagsEvaluated(ctx, projectID)
}

// MarkProjectHealthRecordingsReceived records that at least one recording chunk was received.
func (s *Store) MarkProjectHealthRecordingsReceived(ctx context.Context, projectID string) error {
	return s.q.MarkProjectHealthRecordingsReceived(ctx, projectID)
}

// ResetProjectHealth zeroes all health flags for a project so the implementation can be re-verified.
func (s *Store) ResetProjectHealth(ctx context.Context, projectID string) error {
	return s.q.ResetProjectHealth(ctx, projectID)
}
