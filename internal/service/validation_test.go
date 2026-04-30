package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// ---------------------------------------------------------------------------
// ProjectService validation tests
// ---------------------------------------------------------------------------

func TestProjectService_CreateProject_EmptyName(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	_, err := svc.CreateProject(ctx, "", "my-slug")
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestProjectService_CreateProject_EmptySlug(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	_, err := svc.CreateProject(ctx, "My Project", "")
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestProjectService_CreateProject_WhitespaceOnlyName(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	_, err := svc.CreateProject(ctx, "   ", "my-slug")
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestProjectService_CreateProject_DuplicateSlug_ReturnsConflict(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	_, err := svc.CreateProject(ctx, "Project A", "same-slug")
	require.NoError(t, err)

	_, err = svc.CreateProject(ctx, "Project B", "same-slug")
	require.Error(t, err)
	require.True(t, domain.IsConflict(err), "expected conflict error, got: %v", err)
}

func TestProjectService_GetProject_NotFound(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	_, err := svc.GetProject(ctx, "nonexistent-id")
	require.Error(t, err)
	require.True(t, domain.IsNotFound(err), "expected not found error, got: %v", err)
}

// ---------------------------------------------------------------------------
// FunnelService validation tests
// ---------------------------------------------------------------------------

func TestFunnelService_CreateFunnel_EmptyName(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	funnelSvc := service.NewFunnelService(store)
	projectSvc := service.NewProjectService(store)

	p, err := projectSvc.CreateProject(ctx, "My Project", "my-project")
	require.NoError(t, err)

	_, err = funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "",
		Steps:     []repository.FunnelStep{{EventName: "step1"}},
	})
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestFunnelService_CreateFunnel_NoSteps(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	funnelSvc := service.NewFunnelService(store)
	projectSvc := service.NewProjectService(store)

	p, err := projectSvc.CreateProject(ctx, "My Project", "my-project-2")
	require.NoError(t, err)

	_, err = funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "My Funnel",
		Steps:     []repository.FunnelStep{},
	})
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

// ---------------------------------------------------------------------------
// APIKeyService validation tests
// ---------------------------------------------------------------------------

func TestAPIKeyService_CreateAPIKey_EmptyName(t *testing.T) {
	ctx := context.Background()
	svc := service.NewAPIKeyService(newTestStore(t))

	_, err := svc.CreateAPIKey(ctx, "", "project-id", "sha256hash", "ingest")
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestAPIKeyService_CreateAPIKey_EmptyScope(t *testing.T) {
	ctx := context.Background()
	svc := service.NewAPIKeyService(newTestStore(t))

	_, err := svc.CreateAPIKey(ctx, "my-key", "project-id", "sha256hash", "")
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}
