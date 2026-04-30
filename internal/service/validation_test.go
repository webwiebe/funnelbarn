package service_test

import (
	"context"
	"fmt"
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

func TestAPIKeyService_CreateAPIKey_InvalidScope(t *testing.T) {
	ctx := context.Background()
	svc := service.NewAPIKeyService(newTestStore(t))

	_, err := svc.CreateAPIKey(ctx, "my-key", "project-id", "sha256hash", "superadmin")
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestAPIKeyService_CreateAPIKey_EmptyProjectID(t *testing.T) {
	ctx := context.Background()
	svc := service.NewAPIKeyService(newTestStore(t))

	_, err := svc.CreateAPIKey(ctx, "my-key", "", "sha256hash", "ingest")
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

// ---------------------------------------------------------------------------
// ProjectService additional validation tests
// ---------------------------------------------------------------------------

func TestProjectService_UpdateProject_EmptyName(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	p, err := svc.CreateProject(ctx, "My Project", "my-update-project")
	require.NoError(t, err)

	_, err = svc.UpdateProject(ctx, p.ID, "")
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestProjectService_DeleteProject_EmptyID(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	err := svc.DeleteProject(ctx, "")
	require.Error(t, err)
	require.True(t, domain.IsNotFound(err), "expected not-found error, got: %v", err)
}

func TestProjectService_ApproveProject_EmptyID(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	_, err := svc.ApproveProject(ctx, "")
	require.Error(t, err)
	require.True(t, domain.IsNotFound(err), "expected not-found error, got: %v", err)
}

// ---------------------------------------------------------------------------
// FunnelService additional validation tests
// ---------------------------------------------------------------------------

func TestFunnelService_CreateFunnel_EmptyProjectID(t *testing.T) {
	ctx := context.Background()
	funnelSvc := service.NewFunnelService(newTestStore(t))

	_, err := funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: "",
		Name:      "My Funnel",
		Steps:     []repository.FunnelStep{{EventName: "step1"}},
	})
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestFunnelService_CreateFunnel_EmptyStepEventName(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	funnelSvc := service.NewFunnelService(store)
	projectSvc := service.NewProjectService(store)

	p, err := projectSvc.CreateProject(ctx, "My Project", "my-project-step-name")
	require.NoError(t, err)

	_, err = funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "My Funnel",
		Steps:     []repository.FunnelStep{{EventName: ""}},
	})
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

// ---------------------------------------------------------------------------
// ABTestService validation tests
// ---------------------------------------------------------------------------

func TestABTestService_CreateABTest_EmptyProjectID(t *testing.T) {
	ctx := context.Background()
	svc := service.NewABTestService(newTestStore(t))

	_, err := svc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       "",
		Name:            "My Test",
		ConversionEvent: "purchase",
	})
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestABTestService_CreateABTest_InvalidStatus(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	abSvc := service.NewABTestService(store)

	p, err := projSvc.CreateProject(ctx, "AB Project", "ab-project-invalid-status")
	require.NoError(t, err)

	_, err = abSvc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "My Test",
		Status:          "launched",
		ConversionEvent: "purchase",
	})
	require.Error(t, err)
	require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
}

func TestABTestService_CreateABTest_ValidStatuses(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	abSvc := service.NewABTestService(store)

	p, err := projSvc.CreateProject(ctx, "AB Project", "ab-project-valid-statuses")
	require.NoError(t, err)

	for i, status := range []string{"running", "paused", "completed"} {
		_, err = abSvc.CreateABTest(ctx, repository.ABTest{
			ProjectID:       p.ID,
			Name:            fmt.Sprintf("Test %d", i),
			Status:          status,
			ConversionEvent: "purchase",
		})
		require.NoError(t, err, "status %q should be valid", status)
	}
}
