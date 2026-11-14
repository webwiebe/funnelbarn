package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestStore_ProjectRecordingSettings(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Create a project to attach settings to.
	proj, err := s.CreateProject(ctx, "rec-settings-test", "rec-settings-test")
	require.NoError(t, err)

	// Default: no settings saved yet — returns zero struct with empty rules.
	got, err := s.GetProjectRecordingSettings(ctx, proj.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Enabled)
	assert.Nil(t, got.SampleRate)
	assert.Empty(t, got.Rules)

	// Upsert with enabled=true and a sample rate.
	enabled := true
	rate := 0.5
	settings := &domain.ProjectRecordingSettings{
		ProjectID:  proj.ID,
		Enabled:    &enabled,
		SampleRate: &rate,
		Rules:      []domain.RecordingRule{},
	}
	require.NoError(t, s.UpsertProjectRecordingSettings(ctx, settings))

	got, err = s.GetProjectRecordingSettings(ctx, proj.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Enabled)
	assert.True(t, *got.Enabled)
	require.NotNil(t, got.SampleRate)
	assert.Equal(t, 0.5, *got.SampleRate)
	assert.Empty(t, got.Rules)

	// Update with URL rules and enabled=false.
	disabled := false
	settings.Enabled = &disabled
	settings.Rules = []domain.RecordingRule{
		{Pattern: "/admin/**", Action: "ignore"},
		{Pattern: "/checkout", Action: "capture"},
	}
	require.NoError(t, s.UpsertProjectRecordingSettings(ctx, settings))

	got, err = s.GetProjectRecordingSettings(ctx, proj.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Enabled)
	assert.False(t, *got.Enabled)
	require.Len(t, got.Rules, 2)
	assert.Equal(t, "/admin/**", got.Rules[0].Pattern)
	assert.Equal(t, "ignore", got.Rules[0].Action)
	assert.Equal(t, "/checkout", got.Rules[1].Pattern)
	assert.Equal(t, "capture", got.Rules[1].Action)

	// Upsert with nil Enabled resets to inherit.
	settings.Enabled = nil
	settings.SampleRate = nil
	settings.Rules = []domain.RecordingRule{}
	require.NoError(t, s.UpsertProjectRecordingSettings(ctx, settings))

	got, err = s.GetProjectRecordingSettings(ctx, proj.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Enabled)
	assert.Nil(t, got.SampleRate)
	assert.Empty(t, got.Rules)
}
