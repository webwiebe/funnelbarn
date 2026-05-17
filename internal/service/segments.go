package service

import (
	"context"
	"fmt"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// SegmentService handles segment business logic.
type SegmentService struct {
	store repository.Querier
}

// NewSegmentService creates a SegmentService.
func NewSegmentService(store repository.Querier) *SegmentService {
	return &SegmentService{store: store}
}

func (svc *SegmentService) CreateSegment(ctx context.Context, projectID, name string, rules []repository.SegmentRule) (repository.Segment, error) {
	if name == "" {
		return repository.Segment{}, fmt.Errorf("name is required")
	}
	if err := validateRules(rules); err != nil {
		return repository.Segment{}, err
	}
	return svc.store.CreateSegment(ctx, repository.Segment{
		ProjectID: projectID,
		Name:      name,
		Rules:     rules,
	})
}

func (svc *SegmentService) ListSegments(ctx context.Context, projectID string) ([]repository.Segment, error) {
	return svc.store.ListSegments(ctx, projectID)
}

func (svc *SegmentService) GetSegment(ctx context.Context, id string) (repository.Segment, error) {
	return svc.store.SegmentByID(ctx, id)
}

func (svc *SegmentService) UpdateSegment(ctx context.Context, id, name string, rules []repository.SegmentRule) (repository.Segment, error) {
	if name == "" {
		return repository.Segment{}, fmt.Errorf("name is required")
	}
	if err := validateRules(rules); err != nil {
		return repository.Segment{}, err
	}
	return svc.store.UpdateSegment(ctx, repository.Segment{ID: id, Name: name, Rules: rules})
}

func (svc *SegmentService) DeleteSegment(ctx context.Context, id string) error {
	return svc.store.DeleteSegment(ctx, id)
}

func validateRules(rules []repository.SegmentRule) error {
	for _, r := range rules {
		if _, ok := repository.AllowedSegmentFields[r.Field]; !ok {
			return fmt.Errorf("unsupported segment field %q", r.Field)
		}
		switch r.Operator {
		case "eq", "neq", "contains", "not_contains", "is_null", "is_not_null":
		default:
			return fmt.Errorf("unsupported operator %q", r.Operator)
		}
	}
	return nil
}
