package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func newTestStore(t *testing.T) *repository.Store {
	t.Helper()
	s, err := repository.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProjectService_CreateListDelete(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	p, err := svc.CreateProject(ctx, "Test Project", "test-project")
	require.NoError(t, err)
	require.Equal(t, "Test Project", p.Name)
	require.Equal(t, "active", p.Status)

	projects, err := svc.ListProjects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, p.ID, projects[0].ID)

	err = svc.DeleteProject(ctx, p.ID)
	require.NoError(t, err)

	projects, err = svc.ListProjects(ctx)
	require.NoError(t, err)
	require.Empty(t, projects)
}

func TestProjectService_GetProject(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	p, err := svc.CreateProject(ctx, "Get Me", "get-me")
	require.NoError(t, err)

	got, err := svc.GetProject(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, p.ID, got.ID)
	require.Equal(t, "Get Me", got.Name)
}

func TestProjectService_UpdateProject(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	p, err := svc.CreateProject(ctx, "Old Name", "old-name")
	require.NoError(t, err)

	updated, err := svc.UpdateProject(ctx, p.ID, "New Name", "example.com")
	require.NoError(t, err)
	require.Equal(t, "New Name", updated.Name)
	require.Equal(t, "example.com", updated.Domain)
}

func TestProjectService_ApproveProject(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	p, err := svc.EnsureProjectPending(ctx, "Pending Site", "pending-site")
	require.NoError(t, err)
	require.Equal(t, "pending", p.Status)

	approved, err := svc.ApproveProject(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "active", approved.Status)
}

func TestProjectService_HasProjects(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	has, err := svc.HasProjects(ctx)
	require.NoError(t, err)
	require.False(t, has)

	_, err = svc.CreateProject(ctx, "First", "first")
	require.NoError(t, err)

	has, err = svc.HasProjects(ctx)
	require.NoError(t, err)
	require.True(t, has)
}

func TestProjectService_EnsureSetupAPIKey(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc := service.NewProjectService(store)

	p, err := svc.CreateProject(ctx, "Setup Test", "setup-test")
	require.NoError(t, err)

	const hash = "hash5555555555555555555555555555555555555555555555555555555555555"
	err = svc.EnsureSetupAPIKey(ctx, p.ID, hash)
	require.NoError(t, err)

	// Calling again should be idempotent.
	err = svc.EnsureSetupAPIKey(ctx, p.ID, hash)
	require.NoError(t, err)
}

func TestProjectService_GetProject_NotFound_Extended(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	_, err := svc.GetProject(ctx, "nonexistent-id")
	require.Error(t, err)
	assert.True(t, domain.IsNotFound(err))
}

func TestProjectService_UpdateProject_Extended(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	p, err := svc.CreateProject(ctx, "Original Name", "update-slug")
	require.NoError(t, err)

	updated, err := svc.UpdateProject(ctx, p.ID, "Updated Name", "")
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, "update-slug", updated.Slug)
}

func TestProjectService_DeleteProject_Extended(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	p, err := svc.CreateProject(ctx, "To Delete", "to-delete")
	require.NoError(t, err)

	err = svc.DeleteProject(ctx, p.ID)
	require.NoError(t, err)

	projects, err := svc.ListProjects(ctx)
	require.NoError(t, err)
	assert.Empty(t, projects)
}
