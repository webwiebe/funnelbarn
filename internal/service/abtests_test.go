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

func TestABTestService_CreateABTest_Valid(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	abSvc := service.NewABTestService(store)

	p, err := projSvc.CreateProject(ctx, "AB Project", "ab-project-valid")
	require.NoError(t, err)

	test, err := abSvc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "My Test",
		Status:          "running",
		ConversionEvent: "purchase",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, test.ID)
	assert.Equal(t, "My Test", test.Name)
	assert.Equal(t, "purchase", test.ConversionEvent)
	assert.Equal(t, p.ID, test.ProjectID)
}

func TestABTestService_CreateABTest_EmptyName(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	abSvc := service.NewABTestService(store)

	p, err := projSvc.CreateProject(ctx, "AB Project", "ab-project-emptyname")
	require.NoError(t, err)

	_, err = abSvc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "",
		ConversionEvent: "purchase",
	})
	require.Error(t, err)
	assert.True(t, domain.IsValidation(err))
}

func TestABTestService_CreateABTest_EmptyConversionEvent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	abSvc := service.NewABTestService(store)

	p, err := projSvc.CreateProject(ctx, "AB Project", "ab-project-emptycv")
	require.NoError(t, err)

	_, err = abSvc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "My Test",
		ConversionEvent: "",
	})
	require.Error(t, err)
	assert.True(t, domain.IsValidation(err))
}

func TestABTestService_ListABTests(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	abSvc := service.NewABTestService(store)

	p, err := projSvc.CreateProject(ctx, "AB Project", "ab-project-list")
	require.NoError(t, err)

	_, err = abSvc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Test A",
		Status:          "running",
		ConversionEvent: "click",
	})
	require.NoError(t, err)

	_, err = abSvc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Test B",
		Status:          "paused",
		ConversionEvent: "signup",
	})
	require.NoError(t, err)

	tests, err := abSvc.ListABTests(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, tests, 2)
}
