package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func TestAPIKeyService_CreateAPIKey_Valid(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	apiKeySvc := service.NewAPIKeyService(store)

	p, err := projSvc.CreateProject(ctx, "Key Project", "key-project")
	require.NoError(t, err)

	const hash = "keyhash111111111111111111111111111111111111111111111111111111111"
	k, err := apiKeySvc.CreateAPIKey(ctx, "Test Key", p.ID, hash, "ingest")
	require.NoError(t, err)
	assert.NotEmpty(t, k.ID)
	assert.Equal(t, "Test Key", k.Name)
	assert.Equal(t, "ingest", k.Scope)
	assert.Equal(t, p.ID, k.ProjectID)
}

func TestAPIKeyService_CreateAPIKey_EmptyName_Extended(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	apiKeySvc := service.NewAPIKeyService(store)

	p, err := projSvc.CreateProject(ctx, "Key Project", "key-project-emptyname")
	require.NoError(t, err)

	const hash = "keyhash222222222222222222222222222222222222222222222222222222222"
	_, err = apiKeySvc.CreateAPIKey(ctx, "", p.ID, hash, "ingest")
	require.Error(t, err)
	assert.True(t, domain.IsValidation(err))
}

func TestAPIKeyService_CreateAPIKey_EmptyScope_Extended(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	apiKeySvc := service.NewAPIKeyService(store)

	p, err := projSvc.CreateProject(ctx, "Key Project", "key-project-emptyscope")
	require.NoError(t, err)

	const hash = "keyhash333333333333333333333333333333333333333333333333333333333"
	_, err = apiKeySvc.CreateAPIKey(ctx, "My Key", p.ID, hash, "")
	require.Error(t, err)
	assert.True(t, domain.IsValidation(err))
}

func TestAPIKeyService_ListAPIKeys(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	apiKeySvc := service.NewAPIKeyService(store)

	p, err := projSvc.CreateProject(ctx, "Key Project", "key-project-list")
	require.NoError(t, err)

	_, err = apiKeySvc.CreateAPIKey(ctx, "Key A", p.ID, "keyhash444444444444444444444444444444444444444444444444444444444", "ingest")
	require.NoError(t, err)
	_, err = apiKeySvc.CreateAPIKey(ctx, "Key B", p.ID, "keyhash555555555555555555555555555555555555555555555555555555555", "full")
	require.NoError(t, err)

	keys, err := apiKeySvc.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

func TestAPIKeyService_DeleteAPIKey(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	apiKeySvc := service.NewAPIKeyService(store)

	p, err := projSvc.CreateProject(ctx, "Key Project", "key-project-delete")
	require.NoError(t, err)

	k, err := apiKeySvc.CreateAPIKey(ctx, "Temp Key", p.ID, "keyhash666666666666666666666666666666666666666666666666666666666", "ingest")
	require.NoError(t, err)

	err = apiKeySvc.DeleteAPIKey(ctx, k.ID)
	require.NoError(t, err)

	keys, err := apiKeySvc.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, keys)
}
