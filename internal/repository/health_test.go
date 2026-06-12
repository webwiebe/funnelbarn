package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_GetProjectHealth_NoRow(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Health", "health-norow")
	require.NoError(t, err)

	h, err := s.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, h.ProjectID)
	assert.False(t, h.SetupCalled)
	assert.False(t, h.EventsReceived)
	assert.False(t, h.FlagsEvaluated)
	assert.False(t, h.RecordingsReceived)
}

func TestStore_MarkProjectHealthSetupCalled(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Health", "health-setup")
	require.NoError(t, err)

	require.NoError(t, s.MarkProjectHealthSetupCalled(ctx, p.ID))

	h, err := s.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h.SetupCalled)
	assert.False(t, h.EventsReceived)

	// Idempotent — calling again must not error.
	require.NoError(t, s.MarkProjectHealthSetupCalled(ctx, p.ID))
	h2, err := s.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h2.SetupCalled)
}

func TestStore_MarkProjectHealthEventsReceived(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Health", "health-events")
	require.NoError(t, err)

	require.NoError(t, s.MarkProjectHealthEventsReceived(ctx, p.ID))

	h, err := s.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h.EventsReceived)
	assert.False(t, h.SetupCalled)
}

func TestStore_MarkProjectHealthFlagsEvaluated(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Health", "health-flags")
	require.NoError(t, err)

	require.NoError(t, s.MarkProjectHealthFlagsEvaluated(ctx, p.ID))

	h, err := s.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h.FlagsEvaluated)
}

func TestStore_MarkProjectHealthRecordingsReceived(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Health", "health-recordings")
	require.NoError(t, err)

	require.NoError(t, s.MarkProjectHealthRecordingsReceived(ctx, p.ID))

	h, err := s.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h.RecordingsReceived)
}

func TestStore_ResetProjectHealth(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Health", "health-reset")
	require.NoError(t, err)

	require.NoError(t, s.MarkProjectHealthSetupCalled(ctx, p.ID))
	require.NoError(t, s.MarkProjectHealthEventsReceived(ctx, p.ID))
	require.NoError(t, s.MarkProjectHealthFlagsEvaluated(ctx, p.ID))
	require.NoError(t, s.MarkProjectHealthRecordingsReceived(ctx, p.ID))

	h, err := s.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h.SetupCalled)
	assert.True(t, h.EventsReceived)

	require.NoError(t, s.ResetProjectHealth(ctx, p.ID))

	h2, err := s.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, h2.SetupCalled)
	assert.False(t, h2.EventsReceived)
	assert.False(t, h2.FlagsEvaluated)
	assert.False(t, h2.RecordingsReceived)
}

func TestStore_GetProjectHealth_AllFlags(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Health", "health-allflags")
	require.NoError(t, err)

	require.NoError(t, s.MarkProjectHealthSetupCalled(ctx, p.ID))
	require.NoError(t, s.MarkProjectHealthEventsReceived(ctx, p.ID))
	require.NoError(t, s.MarkProjectHealthFlagsEvaluated(ctx, p.ID))
	require.NoError(t, s.MarkProjectHealthRecordingsReceived(ctx, p.ID))

	h, err := s.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, h.ProjectID)
	assert.True(t, h.SetupCalled)
	assert.True(t, h.EventsReceived)
	assert.True(t, h.FlagsEvaluated)
	assert.True(t, h.RecordingsReceived)
	assert.False(t, h.UpdatedAt.IsZero())
}
