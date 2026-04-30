package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/mock"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// These tests use the in-memory mock — no SQLite, no migrations, runs in microseconds.

func TestProjectService_Mock_CreateProject(t *testing.T) {
	svc := service.NewProjectService(mock.New())
	p, err := svc.CreateProject(context.Background(), "My Site", "my-site")
	require.NoError(t, err)
	require.Equal(t, "My Site", p.Name)
	require.Equal(t, "my-site", p.Slug)
}

func TestProjectService_Mock_DuplicateSlug(t *testing.T) {
	store := mock.New()
	svc := service.NewProjectService(store)
	ctx := context.Background()
	_, err := svc.CreateProject(ctx, "First", "dup-slug")
	require.NoError(t, err)
	_, err = svc.CreateProject(ctx, "Second", "dup-slug")
	require.True(t, domain.IsConflict(err), "expected ErrConflict, got %v", err)
}

func TestProjectService_Mock_NotFound(t *testing.T) {
	svc := service.NewProjectService(mock.New())
	_, err := svc.GetProject(context.Background(), "nonexistent")
	require.True(t, domain.IsNotFound(err), "expected ErrNotFound, got %v", err)
}

func TestProjectService_Mock_Validation_EmptyName(t *testing.T) {
	svc := service.NewProjectService(mock.New())
	_, err := svc.CreateProject(context.Background(), "", "slug")
	require.True(t, domain.IsValidation(err))
}

func TestProjectService_Mock_ListProjects(t *testing.T) {
	store := mock.New()
	svc := service.NewProjectService(store)
	ctx := context.Background()
	_, _ = svc.CreateProject(ctx, "B Project", "proj-b")
	_, _ = svc.CreateProject(ctx, "A Project", "proj-a")
	list, err := svc.ListProjects(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	// Should be sorted by name
	require.Equal(t, "A Project", list[0].Name)
}
