package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// OverviewService handles cross-project ("instance-wide") analytics: GA-like
// rollups, the canonical-event vocabulary, and aggregate cross-project funnels.
type OverviewService struct {
	store ports.OverviewRepo
}

// NewOverviewService creates a new OverviewService.
func NewOverviewService(store ports.OverviewRepo) *OverviewService {
	return &OverviewService{store: store}
}

// ---- GA-like overview ----

func (svc *OverviewService) ProjectRollups(ctx context.Context, from, to time.Time, env string) ([]repository.ProjectRollup, error) {
	return svc.store.ProjectRollups(ctx, from, to, env)
}

func (svc *OverviewService) OverviewTotals(ctx context.Context, from, to time.Time, env string) (int64, int64, error) {
	return svc.store.OverviewTotals(ctx, from, to, env)
}

func (svc *OverviewService) OverviewVisitorsByProjectDaily(ctx context.Context, from, to time.Time, env string) ([]repository.ProjectDayCount, error) {
	return svc.store.OverviewVisitorsByProjectDaily(ctx, from, to, env)
}

func (svc *OverviewService) OverviewTopPages(ctx context.Context, from, to time.Time, limit int, env string) ([]repository.OverviewPageStat, error) {
	return svc.store.OverviewTopPages(ctx, from, to, limit, env)
}

func (svc *OverviewService) OverviewTopReferrers(ctx context.Context, from, to time.Time, limit int, env string) ([]repository.OverviewReferrerStat, error) {
	return svc.store.OverviewTopReferrers(ctx, from, to, limit, env)
}

func (svc *OverviewService) OverviewTopCountries(ctx context.Context, from, to time.Time, limit int, env string) ([]repository.OverviewCountryStat, error) {
	return svc.store.OverviewTopCountries(ctx, from, to, limit, env)
}

func (svc *OverviewService) OverviewDimensionBreakdown(ctx context.Context, dimension string, from, to time.Time, limit int, env string) ([]repository.DimensionStat, error) {
	return svc.store.OverviewDimensionBreakdown(ctx, dimension, from, to, limit, env)
}

func (svc *OverviewService) ListAllEvents(ctx context.Context, f repository.EventFilter, limit int) ([]repository.Event, error) {
	return svc.store.ListAllEvents(ctx, f, limit)
}

// ---- Canonical events + mappings ----

func (svc *OverviewService) ListCanonicalEvents(ctx context.Context) ([]repository.CanonicalEvent, error) {
	return svc.store.ListCanonicalEvents(ctx)
}

func (svc *OverviewService) CreateCanonicalEvent(ctx context.Context, c repository.CanonicalEvent) (repository.CanonicalEvent, error) {
	c.Key = strings.TrimSpace(c.Key)
	if c.Key == "" {
		return repository.CanonicalEvent{}, &domain.ValidationError{Field: "key", Message: "required"}
	}
	if strings.TrimSpace(c.Label) == "" {
		return repository.CanonicalEvent{}, &domain.ValidationError{Field: "label", Message: "required"}
	}
	return svc.store.CreateCanonicalEvent(ctx, c)
}

func (svc *OverviewService) UpdateCanonicalEvent(ctx context.Context, c repository.CanonicalEvent) (repository.CanonicalEvent, error) {
	if strings.TrimSpace(c.Label) == "" {
		return repository.CanonicalEvent{}, &domain.ValidationError{Field: "label", Message: "required"}
	}
	return svc.store.UpdateCanonicalEvent(ctx, c)
}

func (svc *OverviewService) DeleteCanonicalEvent(ctx context.Context, key string) error {
	return svc.store.DeleteCanonicalEvent(ctx, key)
}

func (svc *OverviewService) ListMappings(ctx context.Context, projectID string) ([]repository.EventNameMapping, error) {
	return svc.store.ListMappings(ctx, projectID)
}

// SetMappings bulk-upserts a project's raw→canonical mappings. Each canonical
// key must exist in the catalog (validated up front so a bad row doesn't leave a
// partial write).
func (svc *OverviewService) SetMappings(ctx context.Context, projectID string, mappings []repository.EventNameMapping) error {
	keys, err := svc.store.CanonicalKeySet(ctx)
	if err != nil {
		return err
	}
	for _, m := range mappings {
		if strings.TrimSpace(m.RawName) == "" {
			return &domain.ValidationError{Field: "raw_name", Message: "required"}
		}
		if strings.TrimSpace(m.CanonicalKey) == "" {
			return &domain.ValidationError{Field: "canonical_key", Message: "required"}
		}
		if !keys[m.CanonicalKey] {
			return &domain.ValidationError{Field: "canonical_key", Message: fmt.Sprintf("unknown canonical event %q", m.CanonicalKey)}
		}
	}
	for _, m := range mappings {
		if err := svc.store.UpsertMapping(ctx, projectID, m.RawName, m.CanonicalKey); err != nil {
			return err
		}
	}
	return nil
}

func (svc *OverviewService) DeleteMapping(ctx context.Context, projectID, rawName string) error {
	return svc.store.DeleteMapping(ctx, projectID, rawName)
}

func (svc *OverviewService) MappingSuggestions(ctx context.Context, projectID string) ([]repository.MappingSuggestion, error) {
	return svc.store.MappingSuggestions(ctx, projectID)
}

// ---- Canonical funnels ----

func (svc *OverviewService) CreateCanonicalFunnel(ctx context.Context, f repository.CanonicalFunnel) (repository.CanonicalFunnel, error) {
	if err := svc.validateFunnelWithCatalog(ctx, f); err != nil {
		return repository.CanonicalFunnel{}, err
	}
	return svc.store.CreateCanonicalFunnel(ctx, f)
}

func (svc *OverviewService) ListCanonicalFunnels(ctx context.Context) ([]repository.CanonicalFunnel, error) {
	return svc.store.ListCanonicalFunnels(ctx)
}

func (svc *OverviewService) GetCanonicalFunnel(ctx context.Context, id string) (repository.CanonicalFunnel, error) {
	f, err := svc.store.CanonicalFunnelByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.CanonicalFunnel{}, fmt.Errorf("%w: canonical funnel %s", domain.ErrNotFound, id)
		}
		return repository.CanonicalFunnel{}, err
	}
	return f, nil
}

func (svc *OverviewService) UpdateCanonicalFunnel(ctx context.Context, f repository.CanonicalFunnel) (repository.CanonicalFunnel, error) {
	if err := svc.validateFunnelWithCatalog(ctx, f); err != nil {
		return repository.CanonicalFunnel{}, err
	}
	updated, err := svc.store.UpdateCanonicalFunnel(ctx, f)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.CanonicalFunnel{}, fmt.Errorf("%w: canonical funnel %s", domain.ErrNotFound, f.ID)
		}
		return repository.CanonicalFunnel{}, err
	}
	return updated, nil
}

func (svc *OverviewService) DeleteCanonicalFunnel(ctx context.Context, id string) error {
	return svc.store.DeleteCanonicalFunnel(ctx, id)
}

func (svc *OverviewService) AnalyzeCanonicalFunnel(ctx context.Context, f repository.CanonicalFunnel, projectIDs []string, from, to time.Time, seg *repository.SegmentFilter, rules ...repository.SegmentRule) (repository.CanonicalFunnelResult, error) {
	return svc.store.AnalyzeCanonicalFunnel(ctx, f, projectIDs, from, to, seg, rules...)
}

func validateCanonicalFunnel(f repository.CanonicalFunnel) error {
	if strings.TrimSpace(f.Name) == "" {
		return &domain.ValidationError{Field: "name", Message: "required"}
	}
	if len(f.Steps) == 0 {
		return &domain.ValidationError{Field: "steps", Message: "at least one step required"}
	}
	for i, step := range f.Steps {
		if strings.TrimSpace(step.CanonicalKey) == "" {
			return &domain.ValidationError{Field: fmt.Sprintf("steps[%d].canonical_key", i), Message: "required"}
		}
	}
	return nil
}

// validateFunnelWithCatalog validates the funnel shape and that every step
// references an existing canonical event.
func (svc *OverviewService) validateFunnelWithCatalog(ctx context.Context, f repository.CanonicalFunnel) error {
	if err := validateCanonicalFunnel(f); err != nil {
		return err
	}
	keys, err := svc.store.CanonicalKeySet(ctx)
	if err != nil {
		return err
	}
	for i, step := range f.Steps {
		if !keys[step.CanonicalKey] {
			return &domain.ValidationError{Field: fmt.Sprintf("steps[%d].canonical_key", i), Message: fmt.Sprintf("unknown canonical event %q", step.CanonicalKey)}
		}
	}
	return nil
}
