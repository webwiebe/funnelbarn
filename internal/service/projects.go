package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ProjectService handles project business logic.
type ProjectService struct {
	store *repository.Store
}

// NewProjectService creates a new ProjectService.
func NewProjectService(store *repository.Store) *ProjectService {
	return &ProjectService{store: store}
}

func (svc *ProjectService) CreateProject(ctx context.Context, name, slug string) (repository.Project, error) {
	return svc.store.CreateProject(ctx, name, slug)
}

func (svc *ProjectService) ListProjects(ctx context.Context) ([]repository.Project, error) {
	return svc.store.ListProjects(ctx)
}

func (svc *ProjectService) GetProject(ctx context.Context, id string) (repository.Project, error) {
	return svc.store.ProjectByID(ctx, id)
}

func (svc *ProjectService) GetProjectBySlug(ctx context.Context, slug string) (repository.Project, error) {
	return svc.store.ProjectBySlug(ctx, slug)
}

func (svc *ProjectService) UpdateProject(ctx context.Context, id, name string) (repository.Project, error) {
	return svc.store.UpdateProject(ctx, id, name)
}

func (svc *ProjectService) DeleteProject(ctx context.Context, id string) error {
	return svc.store.DeleteProject(ctx, id)
}

func (svc *ProjectService) ApproveProject(ctx context.Context, id string) (repository.Project, error) {
	return svc.store.ApproveProject(ctx, id)
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
