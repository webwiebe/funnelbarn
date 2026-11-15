package abtests

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// Queries is the read side of the A/B test store.
type Queries interface {
	ABTestByID(ctx context.Context, id string) (ABTest, error)
	ListABTests(ctx context.Context, projectID string) ([]ABTest, error)
	AnalyzeABTest(ctx context.Context, t ABTest, from, to time.Time) ([]ABTestResult, error)
}

// Commands is the write side of the A/B test store.
type Commands interface {
	CreateABTest(ctx context.Context, t ABTest) (ABTest, error)
}

// Repo is everything the service needs from storage.
type Repo interface {
	Queries
	Commands
}

var (
	_ Queries = (*ReadStore)(nil)
	_ Repo    = (*Store)(nil)
)

// Service is the application layer of the A/B test slice.
type Service struct {
	store Repo
}

// NewService returns an A/B test service over store.
func NewService(store Repo) *Service {
	return &Service{store: store}
}

// New returns an A/B test service over a store on the write pool db and the
// read pool rdb.
func New(db, rdb *sql.DB) *Service {
	return NewService(NewStore(db, rdb))
}

// validABTestStatuses is the set of allowed values for ABTest.Status.
var validABTestStatuses = map[string]bool{
	"running":   true,
	"paused":    true,
	"completed": true,
}

func (svc *Service) CreateABTest(ctx context.Context, t ABTest) (ABTest, error) {
	if strings.TrimSpace(t.ProjectID) == "" {
		return ABTest{}, &domain.ValidationError{Field: "project_id", Message: "required"}
	}
	if strings.TrimSpace(t.Name) == "" {
		return ABTest{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	if strings.TrimSpace(t.ConversionEvent) == "" {
		return ABTest{}, &domain.ValidationError{Field: "conversion_event", Message: "required"}
	}
	if t.Status != "" && !validABTestStatuses[t.Status] {
		return ABTest{}, &domain.ValidationError{Field: "status", Message: "must be \"running\", \"paused\", or \"completed\""}
	}
	return svc.store.CreateABTest(ctx, t)
}

func (svc *Service) ListABTests(ctx context.Context, projectID string) ([]ABTest, error) {
	return svc.store.ListABTests(ctx, projectID)
}

func (svc *Service) GetABTest(ctx context.Context, id string) (ABTest, error) {
	t, err := svc.store.ABTestByID(ctx, id)
	if err == sql.ErrNoRows {
		return ABTest{}, fmt.Errorf("%w: ab test %s", domain.ErrNotFound, id)
	}
	return t, err
}

func (svc *Service) AnalyzeABTest(ctx context.Context, t ABTest, from, to time.Time) ([]ABTestResult, error) {
	return svc.store.AnalyzeABTest(ctx, t, from, to)
}
