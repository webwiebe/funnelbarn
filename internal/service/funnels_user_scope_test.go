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

func TestFunnelService_ValidatesScopeAndWindow(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	p, err := service.NewProjectService(store).CreateProject(ctx, "Window Project", "window-project")
	require.NoError(t, err)
	funnelSvc := service.NewFunnelService(store)

	week := int64(7 * 24 * 3600)
	zero := int64(0)
	steps := []repository.FunnelStep{{EventName: "a"}, {EventName: "b"}}

	cases := []struct {
		name   string
		scope  string
		window *int64
		field  string
	}{
		{"unknown scope", "account", nil, "scope"},
		{"window on a session funnel", repository.FunnelScopeSession, &week, "conversion_window_seconds"},
		{"window on a page_view funnel", repository.FunnelScopePageView, &week, "conversion_window_seconds"},
		{"zero window", repository.FunnelScopeUser, &zero, "conversion_window_seconds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := funnelSvc.CreateFunnel(ctx, repository.Funnel{
				ProjectID: p.ID, Name: "F", Scope: tc.scope, ConversionWindowSeconds: tc.window, Steps: steps,
			})
			require.Error(t, err)
			assert.True(t, domain.IsValidation(err))
			assert.Contains(t, err.Error(), tc.field)
		})
	}

	f, err := funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID, Name: "F", Scope: repository.FunnelScopeUser, ConversionWindowSeconds: &week, Steps: steps,
	})
	require.NoError(t, err)

	// An update that leaves scope out keeps both the scope and the window.
	updated, err := funnelSvc.UpdateFunnel(ctx, repository.Funnel{
		ID: f.ID, ProjectID: p.ID, Name: "Renamed", Steps: steps,
	})
	require.NoError(t, err)
	assert.Equal(t, repository.FunnelScopeUser, updated.Scope)
	require.NotNil(t, updated.ConversionWindowSeconds)
	assert.Equal(t, week, *updated.ConversionWindowSeconds)

	// Switching to a session funnel without a window clears it.
	updated, err = funnelSvc.UpdateFunnel(ctx, repository.Funnel{
		ID: f.ID, ProjectID: p.ID, Name: "Renamed", Scope: repository.FunnelScopeSession, Steps: steps,
	})
	require.NoError(t, err)
	assert.Equal(t, repository.FunnelScopeSession, updated.Scope)
	assert.Nil(t, updated.ConversionWindowSeconds)
}
