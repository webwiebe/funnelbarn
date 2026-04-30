package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// FunnelService handles funnel business logic.
type FunnelService struct {
	store repository.Querier
}

// NewFunnelService creates a new FunnelService.
func NewFunnelService(store repository.Querier) *FunnelService {
	return &FunnelService{store: store}
}

func (svc *FunnelService) CreateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error) {
	if strings.TrimSpace(f.ProjectID) == "" {
		return repository.Funnel{}, &domain.ValidationError{Field: "project_id", Message: "required"}
	}
	if strings.TrimSpace(f.Name) == "" {
		return repository.Funnel{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	if len(f.Steps) == 0 {
		return repository.Funnel{}, &domain.ValidationError{Field: "steps", Message: "at least one step required"}
	}
	for i, step := range f.Steps {
		if strings.TrimSpace(step.EventName) == "" {
			return repository.Funnel{}, &domain.ValidationError{Field: fmt.Sprintf("steps[%d].event_name", i), Message: "required"}
		}
	}
	return svc.store.CreateFunnel(ctx, f)
}

func (svc *FunnelService) ListFunnels(ctx context.Context, projectID string) ([]repository.Funnel, error) {
	return svc.store.ListFunnels(ctx, projectID)
}

func (svc *FunnelService) GetFunnel(ctx context.Context, id string) (repository.Funnel, error) {
	f, err := svc.store.FunnelByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.Funnel{}, fmt.Errorf("%w: funnel %s", domain.ErrNotFound, id)
		}
		return repository.Funnel{}, err
	}
	return f, nil
}

func (svc *FunnelService) UpdateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error) {
	return svc.store.UpdateFunnel(ctx, f)
}

func (svc *FunnelService) DeleteFunnel(ctx context.Context, id string) error {
	return svc.store.DeleteFunnel(ctx, id)
}

func (svc *FunnelService) AnalyzeFunnel(ctx context.Context, f repository.Funnel, from, to time.Time, seg *repository.SegmentFilter) ([]repository.FunnelStepResult, error) {
	return svc.store.AnalyzeFunnel(ctx, f, from, to, seg)
}

func (svc *FunnelService) FunnelSegmentData(ctx context.Context, projectID string) (repository.FunnelSegments, error) {
	return svc.store.FunnelSegmentData(ctx, projectID)
}
