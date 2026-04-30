package service

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ABTestService handles A/B test business logic.
type ABTestService struct {
	store *repository.Store
}

// NewABTestService creates a new ABTestService.
func NewABTestService(store *repository.Store) *ABTestService {
	return &ABTestService{store: store}
}

func (svc *ABTestService) CreateABTest(ctx context.Context, t repository.ABTest) (repository.ABTest, error) {
	return svc.store.CreateABTest(ctx, t)
}

func (svc *ABTestService) ListABTests(ctx context.Context, projectID string) ([]repository.ABTest, error) {
	return svc.store.ListABTests(ctx, projectID)
}

func (svc *ABTestService) GetABTest(ctx context.Context, id string) (repository.ABTest, error) {
	return svc.store.ABTestByID(ctx, id)
}

func (svc *ABTestService) AnalyzeABTest(ctx context.Context, t repository.ABTest, from, to time.Time) ([]repository.ABTestResult, error) {
	return svc.store.AnalyzeABTest(ctx, t, from, to)
}
