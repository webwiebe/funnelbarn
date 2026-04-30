package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/mock"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// These tests use the in-memory mock — no SQLite, no migrations, runs in microseconds.

func TestFunnelService_Mock_CreateFunnel(t *testing.T) {
	store := mock.New()
	projSvc := service.NewProjectService(store)
	funnelSvc := service.NewFunnelService(store)
	ctx := context.Background()

	p, err := projSvc.CreateProject(ctx, "Mock Project", "mock-project")
	require.NoError(t, err)

	f, err := funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Checkout Funnel",
		Steps: []repository.FunnelStep{
			{EventName: "cart-viewed"},
			{EventName: "checkout-started"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, f.ID)
	require.Equal(t, "Checkout Funnel", f.Name)
	require.Len(t, f.Steps, 2)
	require.Equal(t, 1, f.Steps[0].StepOrder)
	require.Equal(t, 2, f.Steps[1].StepOrder)
}

func TestFunnelService_Mock_EmptyName(t *testing.T) {
	svc := service.NewFunnelService(mock.New())
	_, err := svc.CreateFunnel(context.Background(), repository.Funnel{
		ProjectID: "proj-1",
		Name:      "",
		Steps:     []repository.FunnelStep{{EventName: "page-view"}},
	})
	require.True(t, domain.IsValidation(err), "expected ErrValidation, got %v", err)
}

func TestFunnelService_Mock_NoStepsRequired(t *testing.T) {
	svc := service.NewFunnelService(mock.New())
	_, err := svc.CreateFunnel(context.Background(), repository.Funnel{
		ProjectID: "proj-1",
		Name:      "Empty Funnel",
		Steps:     []repository.FunnelStep{},
	})
	require.True(t, domain.IsValidation(err), "expected ErrValidation for empty steps, got %v", err)
}

func TestFunnelService_Mock_GetNotFound(t *testing.T) {
	svc := service.NewFunnelService(mock.New())
	_, err := svc.GetFunnel(context.Background(), "nonexistent-funnel-id")
	require.True(t, domain.IsNotFound(err), "expected ErrNotFound, got %v", err)
}

func TestFunnelService_Mock_ListFunnels(t *testing.T) {
	store := mock.New()
	projSvc := service.NewProjectService(store)
	funnelSvc := service.NewFunnelService(store)
	ctx := context.Background()

	p, err := projSvc.CreateProject(ctx, "List Project", "list-project")
	require.NoError(t, err)

	_, err = funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel One",
		Steps:     []repository.FunnelStep{{EventName: "step-1"}},
	})
	require.NoError(t, err)

	_, err = funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel Two",
		Steps:     []repository.FunnelStep{{EventName: "step-a"}, {EventName: "step-b"}},
	})
	require.NoError(t, err)

	list, err := funnelSvc.ListFunnels(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
}
