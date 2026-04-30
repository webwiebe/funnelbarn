package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func TestFunnelService_CreateListDelete(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	funnelSvc := service.NewFunnelService(store)

	p, err := projSvc.CreateProject(ctx, "Funnel Project", "funnel-project")
	require.NoError(t, err)

	f, err := funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID:   p.ID,
		Name:        "Checkout Funnel",
		Description: "Tracks checkout",
		Steps: []repository.FunnelStep{
			{EventName: "cart-viewed"},
			{EventName: "checkout-started"},
			{EventName: "checkout-completed"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, f.ID)
	require.Equal(t, "Checkout Funnel", f.Name)
	require.Len(t, f.Steps, 3)

	funnels, err := funnelSvc.ListFunnels(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, funnels, 1)

	got, err := funnelSvc.GetFunnel(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, f.ID, got.ID)
	require.Equal(t, 3, len(got.Steps))

	err = funnelSvc.DeleteFunnel(ctx, f.ID)
	require.NoError(t, err)

	funnels, err = funnelSvc.ListFunnels(ctx, p.ID)
	require.NoError(t, err)
	require.Empty(t, funnels)
}

func TestFunnelService_UpdateFunnel(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	funnelSvc := service.NewFunnelService(store)

	p, err := projSvc.CreateProject(ctx, "Update Project", "update-project")
	require.NoError(t, err)

	f, err := funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Original Funnel",
		Steps:     []repository.FunnelStep{{EventName: "step-1"}},
	})
	require.NoError(t, err)

	updated, err := funnelSvc.UpdateFunnel(ctx, repository.Funnel{
		ID:          f.ID,
		ProjectID:   p.ID,
		Name:        "Updated Funnel",
		Description: "Now with more steps",
		Steps: []repository.FunnelStep{
			{EventName: "step-a"},
			{EventName: "step-b"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Updated Funnel", updated.Name)
	require.Len(t, updated.Steps, 2)
	require.Equal(t, "step-a", updated.Steps[0].EventName)
	require.Equal(t, 1, updated.Steps[0].StepOrder)
	require.Equal(t, 2, updated.Steps[1].StepOrder)
}

func TestFunnelService_FunnelSegmentData(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	funnelSvc := service.NewFunnelService(store)

	p, err := projSvc.CreateProject(ctx, "Seg Project", "seg-project")
	require.NoError(t, err)

	// With no events, segment data should be empty but not error.
	segs, err := funnelSvc.FunnelSegmentData(ctx, p.ID)
	require.NoError(t, err)
	require.Empty(t, segs.DeviceTypes)
	require.Empty(t, segs.Browsers)
	require.Empty(t, segs.Countries)
}

// ---------------------------------------------------------------------------
// ParseSegment (pure function — no DB required)
// ---------------------------------------------------------------------------

func TestParseSegment_KnownValues(t *testing.T) {
	cases := []struct {
		name  string
		field string
		op    string
		value string
	}{
		{"logged_in", "user_id_hash", "is_not_null", ""},
		{"not_logged_in", "user_id_hash", "is_null", ""},
		{"mobile", "device_type", "eq", "mobile"},
		{"desktop", "device_type", "eq", "desktop"},
		{"tablet", "device_type", "eq", "tablet"},
		{"new_visitor", "session_returning", "eq", "false"},
		{"returning", "session_returning", "eq", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seg := service.ParseSegment(tc.name)
			if seg == nil {
				t.Fatal("expected non-nil SegmentFilter")
			}
			if seg.Field != tc.field {
				t.Errorf("Field: want %q, got %q", tc.field, seg.Field)
			}
			if seg.Op != tc.op {
				t.Errorf("Op: want %q, got %q", tc.op, seg.Op)
			}
			if seg.Value != tc.value {
				t.Errorf("Value: want %q, got %q", tc.value, seg.Value)
			}
		})
	}
}

func TestParseSegment_NoFilter(t *testing.T) {
	for _, name := range []string{"", "all", "unknown", "admin"} {
		if seg := service.ParseSegment(name); seg != nil {
			t.Errorf("ParseSegment(%q): expected nil, got %+v", name, seg)
		}
	}
}
