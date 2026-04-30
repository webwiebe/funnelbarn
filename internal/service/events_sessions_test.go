package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// ---------------------------------------------------------------------------
// APIKeyService
// ---------------------------------------------------------------------------

func TestAPIKeyService_CRUD(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	keySvc := service.NewAPIKeyService(store)

	p, _ := projSvc.CreateProject(ctx, "KeySvc", "keysvc")

	const hash = "hash9999999999999999999999999999999999999999999999999999999999999"

	key, err := keySvc.CreateAPIKey(ctx, "my-key", p.ID, hash, "ingest")
	require.NoError(t, err)
	require.NotEmpty(t, key.ID)
	require.Equal(t, "my-key", key.Name)
	require.Equal(t, "ingest", key.Scope)

	// List by project.
	keys, err := keySvc.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, keys, 1)

	// List all.
	all, err := keySvc.ListAllAPIKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, all)

	// Validate SHA256.
	projectID, scope, found, err := keySvc.ValidAPIKeySHA256(ctx, hash)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, p.ID, projectID)
	require.Equal(t, "ingest", scope)

	// Touch last_used_at.
	require.NoError(t, keySvc.TouchAPIKey(ctx, hash))

	// Delete.
	require.NoError(t, keySvc.DeleteAPIKey(ctx, key.ID))
	keys, err = keySvc.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	require.Empty(t, keys)
}

// ---------------------------------------------------------------------------
// ProjectService — remaining gaps
// ---------------------------------------------------------------------------

func TestProjectService_GetProjectBySlug(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	p, _ := svc.CreateProject(ctx, "Slug Site", "slug-site")

	got, err := svc.GetProjectBySlug(ctx, "slug-site")
	require.NoError(t, err)
	require.Equal(t, p.ID, got.ID)
}

func TestProjectService_EnsureProject(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	// First call: creates project.
	p, err := svc.EnsureProject(ctx, "auto-slug")
	require.NoError(t, err)
	require.NotEmpty(t, p.ID)

	// Second call: returns existing project.
	p2, err := svc.EnsureProject(ctx, "auto-slug")
	require.NoError(t, err)
	require.Equal(t, p.ID, p2.ID)
}

func TestProjectService_UserByUsername(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc := service.NewProjectService(store)

	require.NoError(t, store.UpsertUser(ctx, "testuser", "$2b$hash"))

	u, err := svc.UserByUsername(ctx, "testuser")
	require.NoError(t, err)
	require.Equal(t, "testuser", u.Username)
}

// ---------------------------------------------------------------------------
// ABTestService.AnalyzeABTest and FunnelService.AnalyzeFunnel
// ---------------------------------------------------------------------------

func TestABTestService_AnalyzeABTest(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	abtestSvc := service.NewABTestService(store)

	p, _ := projSvc.CreateProject(ctx, "ABAnalyze", "abanalyze")
	test, err := abtestSvc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Button Colour",
		Status:          "running",
		ConversionEvent: "purchase",
	})
	require.NoError(t, err)

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	results, err := abtestSvc.AnalyzeABTest(ctx, test, from, to)
	require.NoError(t, err)
	_ = results // may be empty with no events, just no error
}

func TestFunnelService_AnalyzeFunnel(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	funnelSvc := service.NewFunnelService(store)

	p, _ := projSvc.CreateProject(ctx, "FunnelAnalyze", "funnelanalyze")
	f, err := funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Checkout",
		Steps: []repository.FunnelStep{
			{EventName: "cart_view"},
			{EventName: "checkout"},
		},
	})
	require.NoError(t, err)

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	results, err := funnelSvc.AnalyzeFunnel(ctx, f, from, to, nil)
	require.NoError(t, err)
	_ = results
}
