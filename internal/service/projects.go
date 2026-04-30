package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ProjectService orchestrates project and user operations.
// It depends on the ProjectRepo port, not the concrete *repository.Store,
// so it can be tested with a lightweight in-memory fake.
type ProjectService struct {
	repo ports.ProjectRepo
}

// NewProjectService creates a ProjectService.
// Any value implementing ports.ProjectRepo is accepted, including *repository.Store
// and test fakes.
func NewProjectService(repo ports.ProjectRepo) *ProjectService {
	return &ProjectService{repo: repo}
}

func (svc *ProjectService) CreateProject(ctx context.Context, name, slug string) (repository.Project, error) {
	return svc.repo.CreateProject(ctx, name, slug)
}

func (svc *ProjectService) ListProjects(ctx context.Context) ([]repository.Project, error) {
	return svc.repo.ListProjects(ctx)
}

func (svc *ProjectService) GetProject(ctx context.Context, id string) (repository.Project, error) {
	return svc.repo.ProjectByID(ctx, id)
}

func (svc *ProjectService) GetProjectBySlug(ctx context.Context, slug string) (repository.Project, error) {
	return svc.repo.ProjectBySlug(ctx, slug)
}

func (svc *ProjectService) UpdateProject(ctx context.Context, id, name string) (repository.Project, error) {
	return svc.repo.UpdateProject(ctx, id, name)
}

func (svc *ProjectService) DeleteProject(ctx context.Context, id string) error {
	return svc.repo.DeleteProject(ctx, id)
}

func (svc *ProjectService) ApproveProject(ctx context.Context, id string) (repository.Project, error) {
	return svc.repo.ApproveProject(ctx, id)
}

func (svc *ProjectService) EnsureProjectPending(ctx context.Context, name, slug string) (repository.Project, error) {
	return svc.repo.EnsureProjectPending(ctx, name, slug)
}

func (svc *ProjectService) EnsureSetupAPIKey(ctx context.Context, projectID, keySHA256 string) error {
	return svc.repo.EnsureSetupAPIKey(ctx, projectID, keySHA256)
}

func (svc *ProjectService) EnsureProject(ctx context.Context, slug string) (repository.Project, error) {
	return svc.repo.EnsureProject(ctx, slug)
}

func (svc *ProjectService) HasProjects(ctx context.Context) (bool, error) {
	return svc.repo.HasProjects(ctx)
}

func (svc *ProjectService) UserByUsername(ctx context.Context, username string) (repository.User, error) {
	return svc.repo.UserByUsername(ctx, username)
}
