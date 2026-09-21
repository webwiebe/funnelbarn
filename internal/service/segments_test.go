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

func TestSegmentService_CRUD(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	segSvc := service.NewSegmentService(store)

	p, err := projSvc.CreateProject(ctx, "Seg Svc Project", "seg-svc-project")
	require.NoError(t, err)

	rules := []repository.SegmentRule{
		{Field: "country_code", Operator: "eq", Value: "NL"},
	}

	// CreateSegment
	seg, err := segSvc.CreateSegment(ctx, p.ID, "Dutch visitors", rules)
	require.NoError(t, err)
	assert.NotEmpty(t, seg.ID)
	assert.Equal(t, "Dutch visitors", seg.Name)

	// GetSegment
	got, err := segSvc.GetSegment(ctx, seg.ID)
	require.NoError(t, err)
	assert.Equal(t, seg.ID, got.ID)

	// ListSegments
	list, err := segSvc.ListSegments(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// UpdateSegment
	updated, err := segSvc.UpdateSegment(ctx, seg.ID, "NL visitors", rules)
	require.NoError(t, err)
	assert.Equal(t, "NL visitors", updated.Name)

	// DeleteSegment
	require.NoError(t, segSvc.DeleteSegment(ctx, seg.ID))
	_, err = segSvc.GetSegment(ctx, seg.ID)
	require.Error(t, err)
	assert.True(t, domain.IsNotFound(err))
}

func TestSegmentService_GetSegment_NotFound(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	segSvc := service.NewSegmentService(store)

	_, err := segSvc.GetSegment(ctx, "does-not-exist")
	require.Error(t, err)
	assert.True(t, domain.IsNotFound(err))
}

func TestSegmentService_CreateSegment_Validation(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	segSvc := service.NewSegmentService(store)

	// Empty name.
	_, err := segSvc.CreateSegment(ctx, "proj-1", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name: required")
	assert.True(t, domain.IsValidation(err))

	// Invalid field.
	_, err = segSvc.CreateSegment(ctx, "proj-1", "bad field", []repository.SegmentRule{
		{Field: "invalid_field", Operator: "eq", Value: "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported segment field")
	assert.True(t, domain.IsValidation(err))

	// Invalid operator.
	_, err = segSvc.CreateSegment(ctx, "proj-1", "bad op", []repository.SegmentRule{
		{Field: "country_code", Operator: "like", Value: "NL"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operator")
	assert.True(t, domain.IsValidation(err))
}

func TestSegmentService_UpdateSegment_Validation(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	segSvc := service.NewSegmentService(store)

	_, err := segSvc.UpdateSegment(ctx, "some-id", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name: required")
	assert.True(t, domain.IsValidation(err))
}
