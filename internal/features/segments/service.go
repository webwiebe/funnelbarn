package segments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// Queries is the read side of the segment store.
type Queries interface {
	SegmentByID(ctx context.Context, id string) (Segment, error)
	ListSegments(ctx context.Context, projectID string) ([]Segment, error)
}

// Commands is the write side of the segment store.
type Commands interface {
	CreateSegment(ctx context.Context, seg Segment) (Segment, error)
	UpdateSegment(ctx context.Context, seg Segment) (Segment, error)
	DeleteSegment(ctx context.Context, id string) error
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

// Service handles segment business logic.
type Service struct {
	store Repo
}

// NewService returns a segment service over store.
func NewService(store Repo) *Service {
	return &Service{store: store}
}

// New returns a segment service over a store on the write pool db and the
// read pool rdb.
func New(db, rdb *sql.DB) *Service {
	return NewService(NewStore(db, rdb))
}

func (svc *Service) CreateSegment(ctx context.Context, projectID, name string, rules []domain.SegmentRule) (Segment, error) {
	if name == "" {
		return Segment{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	if err := validateRules(rules); err != nil {
		return Segment{}, err
	}
	return svc.store.CreateSegment(ctx, Segment{
		ProjectID: projectID,
		Name:      name,
		Rules:     rules,
	})
}

func (svc *Service) ListSegments(ctx context.Context, projectID string) ([]Segment, error) {
	return svc.store.ListSegments(ctx, projectID)
}

func (svc *Service) GetSegment(ctx context.Context, id string) (Segment, error) {
	seg, err := svc.store.SegmentByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Segment{}, fmt.Errorf("%w: segment %s", domain.ErrNotFound, id)
		}
		return Segment{}, err
	}
	return seg, nil
}

func (svc *Service) UpdateSegment(ctx context.Context, id, name string, rules []domain.SegmentRule) (Segment, error) {
	if name == "" {
		return Segment{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	if err := validateRules(rules); err != nil {
		return Segment{}, err
	}
	return svc.store.UpdateSegment(ctx, Segment{ID: id, Name: name, Rules: rules})
}

func (svc *Service) DeleteSegment(ctx context.Context, id string) error {
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
