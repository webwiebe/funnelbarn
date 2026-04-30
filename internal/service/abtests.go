package service

import (
	"context"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ABTestService handles A/B test business logic.
type ABTestService struct {
	store repository.Querier
}

// NewABTestService creates a new ABTestService.
func NewABTestService(store repository.Querier) *ABTestService {
	return &ABTestService{store: store}
}

// validABTestStatuses is the set of allowed values for ABTest.Status.
var validABTestStatuses = map[string]bool{
	"running":   true,
	"paused":    true,
	"completed": true,
}

func (svc *ABTestService) CreateABTest(ctx context.Context, t repository.ABTest) (repository.ABTest, error) {
	if strings.TrimSpace(t.ProjectID) == "" {
		return repository.ABTest{}, &domain.ValidationError{Field: "project_id", Message: "required"}
	}
	if strings.TrimSpace(t.Name) == "" {
		return repository.ABTest{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	if strings.TrimSpace(t.ConversionEvent) == "" {
		return repository.ABTest{}, &domain.ValidationError{Field: "conversion_event", Message: "required"}
	}
	if t.Status != "" && !validABTestStatuses[t.Status] {
		return repository.ABTest{}, &domain.ValidationError{Field: "status", Message: "must be \"running\", \"paused\", or \"completed\""}
	}
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
