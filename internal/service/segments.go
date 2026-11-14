package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
)

// SegmentService handles segment business logic.
type SegmentService struct {
	store ports.SegmentRepo
}

// NewSegmentService creates a SegmentService.
func NewSegmentService(store ports.SegmentRepo) *SegmentService {
	return &SegmentService{store: store}
}

func (svc *SegmentService) CreateSegment(ctx context.Context, projectID, name string, rules []domain.SegmentRule) (domain.Segment, error) {
	if name == "" {
		return domain.Segment{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	if err := validateRules(rules); err != nil {
		return domain.Segment{}, err
	}
	return svc.store.CreateSegment(ctx, domain.Segment{
		ProjectID: projectID,
		Name:      name,
		Rules:     rules,
	})
}

func (svc *SegmentService) ListSegments(ctx context.Context, projectID string) ([]domain.Segment, error) {
	return svc.store.ListSegments(ctx, projectID)
}

func (svc *SegmentService) GetSegment(ctx context.Context, id string) (domain.Segment, error) {
	seg, err := svc.store.SegmentByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Segment{}, fmt.Errorf("%w: segment %s", domain.ErrNotFound, id)
		}
		return domain.Segment{}, err
	}
	return seg, nil
}

func (svc *SegmentService) UpdateSegment(ctx context.Context, id, name string, rules []domain.SegmentRule) (domain.Segment, error) {
	if name == "" {
		return domain.Segment{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	if err := validateRules(rules); err != nil {
		return domain.Segment{}, err
	}
	return svc.store.UpdateSegment(ctx, domain.Segment{ID: id, Name: name, Rules: rules})
}

func (svc *SegmentService) DeleteSegment(ctx context.Context, id string) error {
	return svc.store.DeleteSegment(ctx, id)
}

func validateRules(rules []domain.SegmentRule) error {
	for i, r := range rules {
		if _, ok := domain.AllowedSegmentFields[r.Field]; !ok {
			return &domain.ValidationError{
				Field:   fmt.Sprintf("rules[%d].field", i),
				Message: fmt.Sprintf("unsupported segment field %q", r.Field),
			}
		}
		switch r.Operator {
		case "eq", "neq", "contains", "not_contains", "is_null", "is_not_null":
		default:
			return &domain.ValidationError{
				Field:   fmt.Sprintf("rules[%d].operator", i),
				Message: fmt.Sprintf("unsupported operator %q", r.Operator),
			}
		}
	}
	return nil
}
