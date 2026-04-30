package service

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// FunnelService orchestrates funnel operations.
type FunnelService struct {
	repo ports.FunnelRepo
}

func NewFunnelService(repo ports.FunnelRepo) *FunnelService {
	return &FunnelService{repo: repo}
}

func (svc *FunnelService) CreateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error) {
	return svc.repo.CreateFunnel(ctx, f)
}

func (svc *FunnelService) ListFunnels(ctx context.Context, projectID string) ([]repository.Funnel, error) {
	return svc.repo.ListFunnels(ctx, projectID)
}

func (svc *FunnelService) GetFunnel(ctx context.Context, id string) (repository.Funnel, error) {
	return svc.repo.FunnelByID(ctx, id)
}

func (svc *FunnelService) UpdateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error) {
	return svc.repo.UpdateFunnel(ctx, f)
}

func (svc *FunnelService) DeleteFunnel(ctx context.Context, id string) error {
	return svc.repo.DeleteFunnel(ctx, id)
}

func (svc *FunnelService) AnalyzeFunnel(ctx context.Context, f repository.Funnel, from, to time.Time, seg *repository.SegmentFilter) ([]repository.FunnelStepResult, error) {
	return svc.repo.AnalyzeFunnel(ctx, f, from, to, seg)
}

func (svc *FunnelService) FunnelSegmentData(ctx context.Context, projectID string) (repository.FunnelSegments, error) {
	return svc.repo.FunnelSegmentData(ctx, projectID)
}

// ParseSegment maps a preset segment name to a SegmentFilter.
// Returns nil for "all" or empty (no filter applied).
// Moved here from the API handler layer: segment mapping is a business rule,
// not a presentation concern.
func ParseSegment(name string) *repository.SegmentFilter {
	switch name {
	case "logged_in":
		return &repository.SegmentFilter{Field: "user_id_hash", Op: "is_not_null"}
	case "not_logged_in":
		return &repository.SegmentFilter{Field: "user_id_hash", Op: "is_null"}
	case "mobile":
		return &repository.SegmentFilter{Field: "device_type", Op: "eq", Value: "mobile"}
	case "desktop":
		return &repository.SegmentFilter{Field: "device_type", Op: "eq", Value: "desktop"}
	case "tablet":
		return &repository.SegmentFilter{Field: "device_type", Op: "eq", Value: "tablet"}
	case "new_visitor":
		return &repository.SegmentFilter{Field: "session_returning", Op: "eq", Value: "false"}
	case "returning":
		return &repository.SegmentFilter{Field: "session_returning", Op: "eq", Value: "true"}
	default:
		return nil
	}
}
