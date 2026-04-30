package service

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// FunnelService handles funnel business logic.
type FunnelService struct {
	store *repository.Store
}

// NewFunnelService creates a new FunnelService.
func NewFunnelService(store *repository.Store) *FunnelService {
	return &FunnelService{store: store}
}

func (svc *FunnelService) CreateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error) {
	return svc.store.CreateFunnel(ctx, f)
}

func (svc *FunnelService) ListFunnels(ctx context.Context, projectID string) ([]repository.Funnel, error) {
	return svc.store.ListFunnels(ctx, projectID)
}

func (svc *FunnelService) GetFunnel(ctx context.Context, id string) (repository.Funnel, error) {
	return svc.store.FunnelByID(ctx, id)
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
