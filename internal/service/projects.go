package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
)

// envSuffixToken returns the environment alias a project slug ends with
// ("shop-staging" gives "staging"), or "" when it has none. A slug that is only
// an alias ("test") has no product part and is left alone.
func envSuffixToken(slug string) string {
	i := strings.LastIndex(slug, "-")
	if i <= 0 {
		return ""
	}
	tail := slug[i+1:]
	if tail != "" && environment.IsKnown(tail) {
		return tail
	}
	return ""
}

// refuseEnvSuffix rejects a new project whose slug names an environment. An
// environment is an attribute of the event, never a separate project (#545).
func refuseEnvSuffix(slug string) error {
	if tok := envSuffixToken(slug); tok != "" {
		return &domain.ValidationError{
			Field:   "slug",
			Message: fmt.Sprintf("must not end in an environment name (%q): create one project per product and send environment=%s on its events", tok, environment.Normalize(tok)),
		}
	}
	return nil
}

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
	store ports.ProjectRepo
}

// NewProjectService creates a new ProjectService.
func NewProjectService(store ports.ProjectRepo) *ProjectService {
	return &ProjectService{store: store}
}

func (svc *ProjectService) CreateProject(ctx context.Context, name, slug string) (domain.Project, error) {
	if strings.TrimSpace(name) == "" {
		return domain.Project{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	slug = normalizeSlug(slug)
	if slug == "" {
		return domain.Project{}, &domain.ValidationError{Field: "slug", Message: "required"}
	}
	if err := refuseEnvSuffix(slug); err != nil {
		return domain.Project{}, err
	}
	p, err := svc.store.CreateProject(ctx, strings.TrimSpace(name), slug)
	if err != nil {
		if domain.IsConflict(err) {
			return domain.Project{}, fmt.Errorf("%w: slug %q", domain.ErrConflict, slug)
		}
		return domain.Project{}, err
	}
	return p, nil
}

func (svc *ProjectService) ListProjects(ctx context.Context) ([]domain.Project, error) {
	return svc.store.ListProjects(ctx)
}

func (svc *ProjectService) GetProject(ctx context.Context, id string) (domain.Project, error) {
	p, err := svc.store.ProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, fmt.Errorf("%w: project %s", domain.ErrNotFound, id)
		}
		return domain.Project{}, err
	}
	return p, nil
}

func (svc *ProjectService) GetProjectBySlug(ctx context.Context, slug string) (domain.Project, error) {
	p, err := svc.store.ProjectBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, fmt.Errorf("%w: project slug %s", domain.ErrNotFound, slug)
		}
		return domain.Project{}, err
	}
	return p, nil
}

func (svc *ProjectService) UpdateProject(ctx context.Context, id, name, projectDomain string) (domain.Project, error) {
	if strings.TrimSpace(name) == "" {
		return domain.Project{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	p, err := svc.store.UpdateProject(ctx, id, name, strings.TrimSpace(projectDomain))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, fmt.Errorf("%w: project %s", domain.ErrNotFound, id)
		}
		return domain.Project{}, err
	}
	return p, nil
}

func (svc *ProjectService) DeleteProject(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: project id required", domain.ErrNotFound)
	}
	return svc.store.DeleteProject(ctx, id)
}

func (svc *ProjectService) ApproveProject(ctx context.Context, id string) (domain.Project, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Project{}, fmt.Errorf("%w: project id required", domain.ErrNotFound)
	}
	p, err := svc.store.ApproveProject(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, fmt.Errorf("%w: project %s", domain.ErrNotFound, id)
		}
		return domain.Project{}, err
	}
	return p, nil
}

// EnsureProjectPending returns the project for slug, creating it as pending when
// it is new. An existing project is returned as it is, whatever its name; only a
// new project with an environment suffix is refused.
func (svc *ProjectService) EnsureProjectPending(ctx context.Context, name, slug string) (domain.Project, error) {
	if err := refuseEnvSuffix(normalizeSlug(slug)); err != nil {
		if p, perr := svc.store.ProjectBySlug(ctx, slug); perr == nil {
			return p, nil
		}
		return domain.Project{}, err
	}
	return svc.store.EnsureProjectPending(ctx, name, slug)
}

func (svc *ProjectService) EnsureProject(ctx context.Context, slug string) (domain.Project, error) {
	return svc.store.EnsureProject(ctx, slug)
}

func (svc *ProjectService) HasProjects(ctx context.Context) (bool, error) {
	return svc.store.HasProjects(ctx)
}

func (svc *ProjectService) UserByUsername(ctx context.Context, username string) (domain.User, error) {
	return svc.store.UserByUsername(ctx, username)
}
