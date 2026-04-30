package service

import (
	"context"
	"math"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ABTestService orchestrates A/B test operations.
type ABTestService struct {
	repo ports.ABTestRepo
}

func NewABTestService(repo ports.ABTestRepo) *ABTestService {
	return &ABTestService{repo: repo}
}

func (svc *ABTestService) CreateABTest(ctx context.Context, t repository.ABTest) (repository.ABTest, error) {
	return svc.repo.CreateABTest(ctx, t)
}

func (svc *ABTestService) ListABTests(ctx context.Context, projectID string) ([]repository.ABTest, error) {
	return svc.repo.ListABTests(ctx, projectID)
}

func (svc *ABTestService) GetABTest(ctx context.Context, id string) (repository.ABTest, error) {
	return svc.repo.ABTestByID(ctx, id)
}

func (svc *ABTestService) AnalyzeABTest(ctx context.Context, t repository.ABTest, from, to time.Time) ([]repository.ABTestResult, error) {
	return svc.repo.AnalyzeABTest(ctx, t, from, to)
}

// ZTest performs a two-proportion z-test for A/B significance.
// Returns (zScore, significant) where significant = |z| > 1.96 (95% CI).
// Moved here from the API handler: statistical logic belongs in the service
// layer, not in HTTP handler code.
func ZTest(n1, x1, n2, x2 int64) (zScore float64, significant bool) {
	if n1 == 0 || n2 == 0 {
		return 0, false
	}
	p1 := float64(x1) / float64(n1)
	p2 := float64(x2) / float64(n2)
	pPool := float64(x1+x2) / float64(n1+n2)
	if pPool == 0 || pPool == 1 {
		return 0, false
	}
	se := math.Sqrt(pPool * (1 - pPool) * (1/float64(n1) + 1/float64(n2)))
	if se == 0 {
		return 0, false
	}
	z := math.Abs((p1 - p2) / se)
	return z, z > 1.96
}
