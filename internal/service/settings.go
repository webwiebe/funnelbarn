package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
)

// instanceSettingsService passes instance-level settings through to the store.
// Handlers own error logging, so it returns repository errors unchanged.
type instanceSettingsService struct {
	repo ports.InstanceSettingsRepo
}

// NewInstanceSettingsService creates the InstanceSettings service.
func NewInstanceSettingsService(repo ports.InstanceSettingsRepo) InstanceSettings {
	return &instanceSettingsService{repo: repo}
}

func (s *instanceSettingsService) GetAllInstanceSettings(ctx context.Context) (map[string]string, error) {
	return s.repo.GetAllInstanceSettings(ctx)
}

func (s *instanceSettingsService) SetInstanceSetting(ctx context.Context, key, value string) error {
	return s.repo.SetInstanceSetting(ctx, key, value)
}

// recordingSettingsService passes per-project recording settings through to
// the store.
type recordingSettingsService struct {
	repo ports.ProjectRecordingSettingsRepo
}

// NewRecordingSettingsService creates the RecordingSettings service.
func NewRecordingSettingsService(repo ports.ProjectRecordingSettingsRepo) RecordingSettings {
	return &recordingSettingsService{repo: repo}
}

func (s *recordingSettingsService) GetProjectRecordingSettings(ctx context.Context, projectID string) (*domain.ProjectRecordingSettings, error) {
	return s.repo.GetProjectRecordingSettings(ctx, projectID)
}

func (s *recordingSettingsService) UpsertProjectRecordingSettings(ctx context.Context, settings *domain.ProjectRecordingSettings) error {
	return s.repo.UpsertProjectRecordingSettings(ctx, settings)
}
