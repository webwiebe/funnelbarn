package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// reNonSlug matches any character that is not a lowercase letter, digit, or hyphen.
var reNonSlug = regexp.MustCompile(`[^a-z0-9-]+`)

// normalizeSlug converts s to a lowercase, hyphen-separated slug.
func normalizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = reNonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	// Collapse consecutive hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

// ProjectService handles project business logic.
type ProjectService struct {
	store repository.Querier
}

// NewProjectService creates a new ProjectService.
func NewProjectService(store repository.Querier) *ProjectService {
	return &ProjectService{store: store}
}

func (svc *ProjectService) CreateProject(ctx context.Context, name, slug string) (repository.Project, error) {
	if strings.TrimSpace(name) == "" {
		return repository.Project{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	slug = normalizeSlug(slug)
	if slug == "" {
		return repository.Project{}, &domain.ValidationError{Field: "slug", Message: "required"}
	}
	p, err := svc.store.CreateProject(ctx, strings.TrimSpace(name), slug)
	if err != nil {
		if isUniqueConstraint(err) {
			return repository.Project{}, fmt.Errorf("%w: slug %q", domain.ErrConflict, slug)
		}
		return repository.Project{}, err
	}
	return p, nil
}

func (svc *ProjectService) ListProjects(ctx context.Context) ([]repository.Project, error) {
	return svc.store.ListProjects(ctx)
}

func (svc *ProjectService) GetProject(ctx context.Context, id string) (repository.Project, error) {
	p, err := svc.store.ProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.Project{}, fmt.Errorf("%w: project %s", domain.ErrNotFound, id)
		}
		return repository.Project{}, err
	}
	return p, nil
}

func (svc *ProjectService) GetProjectBySlug(ctx context.Context, slug string) (repository.Project, error) {
	p, err := svc.store.ProjectBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.Project{}, fmt.Errorf("%w: project slug %s", domain.ErrNotFound, slug)
		}
		return repository.Project{}, err
	}
	return p, nil
}

func (svc *ProjectService) UpdateProject(ctx context.Context, id, name string) (repository.Project, error) {
	if strings.TrimSpace(name) == "" {
		return repository.Project{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	p, err := svc.store.UpdateProject(ctx, id, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.Project{}, fmt.Errorf("%w: project %s", domain.ErrNotFound, id)
		}
		return repository.Project{}, err
	}
	return p, nil
}

func (svc *ProjectService) DeleteProject(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: project id required", domain.ErrNotFound)
	}
	return svc.store.DeleteProject(ctx, id)
}

func (svc *ProjectService) ApproveProject(ctx context.Context, id string) (repository.Project, error) {
	if strings.TrimSpace(id) == "" {
		return repository.Project{}, fmt.Errorf("%w: project id required", domain.ErrNotFound)
	}
	p, err := svc.store.ApproveProject(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.Project{}, fmt.Errorf("%w: project %s", domain.ErrNotFound, id)
		}
		return repository.Project{}, err
	}
	return p, nil
}

func (svc *ProjectService) EnsureProjectPending(ctx context.Context, name, slug string) (repository.Project, error) {
	return svc.store.EnsureProjectPending(ctx, name, slug)
}

func (svc *ProjectService) EnsureSetupAPIKey(ctx context.Context, projectID, keySHA256 string) error {
	return svc.store.EnsureSetupAPIKey(ctx, projectID, keySHA256)
}

func (svc *ProjectService) EnsureProject(ctx context.Context, slug string) (repository.Project, error) {
	return svc.store.EnsureProject(ctx, slug)
}

func (svc *ProjectService) HasProjects(ctx context.Context) (bool, error) {
	return svc.store.HasProjects(ctx)
}

func (svc *ProjectService) UserByUsername(ctx context.Context, username string) (repository.User, error) {
	return svc.store.UserByUsername(ctx, username)
}
