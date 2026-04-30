package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func newTestStore(t *testing.T) *repository.Store {
	t.Helper()
	s, err := repository.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// --------------------------------------------------------------------------
// Projects
// --------------------------------------------------------------------------

func TestCreateProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "My Project", "my-project")
	require.NoError(t, err)
	require.NotEmpty(t, p.ID)
	require.Equal(t, "My Project", p.Name)
	require.Equal(t, "my-project", p.Slug)
	require.Equal(t, "active", p.Status)
}

func TestListProjects(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.CreateProject(ctx, "Alpha", "alpha")
	require.NoError(t, err)
	_, err = s.CreateProject(ctx, "Beta", "beta")
	require.NoError(t, err)

	projects, err := s.ListProjects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 2)
	// Ordered by name.
	require.Equal(t, "Alpha", projects[0].Name)
	require.Equal(t, "Beta", projects[1].Name)
}

func TestEnsureProjectPending(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.EnsureProjectPending(ctx, "New Site", "new-site")
	require.NoError(t, err)
	require.Equal(t, "pending", p.Status)

	// Second call should return same project unchanged.
	p2, err := s.EnsureProjectPending(ctx, "New Site", "new-site")
	require.NoError(t, err)
	require.Equal(t, p.ID, p2.ID)
}

func TestApproveProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.EnsureProjectPending(ctx, "Site", "site")
	require.NoError(t, err)
	require.Equal(t, "pending", p.Status)

	approved, err := s.ApproveProject(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "active", approved.Status)
}

func TestDeleteProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Temp", "temp")
	require.NoError(t, err)

	err = s.DeleteProject(ctx, p.ID)
	require.NoError(t, err)

	projects, err := s.ListProjects(ctx)
	require.NoError(t, err)
	require.Empty(t, projects)
}

// --------------------------------------------------------------------------
// Funnels
// --------------------------------------------------------------------------

func TestCreateFunnel(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj")
	require.NoError(t, err)

	f, err := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID:   p.ID,
		Name:        "Sign Up Funnel",
		Description: "Tracks sign-up flow",
		Steps: []repository.FunnelStep{
			{EventName: "page-view"},
			{EventName: "signup-started"},
			{EventName: "signup-completed"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, f.ID)
	require.Equal(t, "Sign Up Funnel", f.Name)
	require.Len(t, f.Steps, 3)
	require.Equal(t, 1, f.Steps[0].StepOrder)
	require.Equal(t, 2, f.Steps[1].StepOrder)
	require.Equal(t, 3, f.Steps[2].StepOrder)
}

func TestListFunnels(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj")
	require.NoError(t, err)

	_, err = s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel A",
		Steps:     []repository.FunnelStep{{EventName: "step-1"}},
	})
	require.NoError(t, err)

	_, err = s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel B",
		Steps:     []repository.FunnelStep{{EventName: "step-1"}},
	})
	require.NoError(t, err)

	funnels, err := s.ListFunnels(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, funnels, 2)
}

func TestDeleteFunnel(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj")
	require.NoError(t, err)

	f, err := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Temp Funnel",
		Steps:     []repository.FunnelStep{{EventName: "step-1"}},
	})
	require.NoError(t, err)

	err = s.DeleteFunnel(ctx, f.ID)
	require.NoError(t, err)

	funnels, err := s.ListFunnels(ctx, p.ID)
	require.NoError(t, err)
	require.Empty(t, funnels)
}

// --------------------------------------------------------------------------
// API Keys
// --------------------------------------------------------------------------

func TestCreateAPIKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj")
	require.NoError(t, err)

	k, err := s.CreateAPIKey(ctx, "my-key", p.ID, "abc123sha256hashvalue0000000000000000000000000000000000000000000", "ingest")
	require.NoError(t, err)
	require.NotEmpty(t, k.ID)
	require.Equal(t, "my-key", k.Name)
	require.Equal(t, "ingest", k.Scope)
}

func TestListAPIKeys(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj")
	require.NoError(t, err)

	_, err = s.CreateAPIKey(ctx, "key-1", p.ID, "hash1111111111111111111111111111111111111111111111111111111111111", "ingest")
	require.NoError(t, err)
	_, err = s.CreateAPIKey(ctx, "key-2", p.ID, "hash2222222222222222222222222222222222222222222222222222222222222", "full")
	require.NoError(t, err)

	keys, err := s.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, keys, 2)
}

func TestDeleteAPIKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj")
	require.NoError(t, err)

	k, err := s.CreateAPIKey(ctx, "temp-key", p.ID, "hash3333333333333333333333333333333333333333333333333333333333333", "ingest")
	require.NoError(t, err)

	err = s.DeleteAPIKey(ctx, k.ID)
	require.NoError(t, err)

	keys, err := s.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	require.Empty(t, keys)
}

func TestValidAPIKeySHA256(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj")
	require.NoError(t, err)

	const hash = "hash4444444444444444444444444444444444444444444444444444444444444"
	_, err = s.CreateAPIKey(ctx, "lookup-key", p.ID, hash, "ingest")
	require.NoError(t, err)

	projectID, scope, found, err := s.ValidAPIKeySHA256(ctx, hash)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, p.ID, projectID)
	require.Equal(t, "ingest", scope)

	_, _, found, err = s.ValidAPIKeySHA256(ctx, "notexist")
	require.NoError(t, err)
	require.False(t, found)
}

// --------------------------------------------------------------------------
// Users
// --------------------------------------------------------------------------

func TestUpsertUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	err := s.UpsertUser(ctx, "admin", "$2a$10$fakehash")
	require.NoError(t, err)

	u, err := s.UserByUsername(ctx, "admin")
	require.NoError(t, err)
	require.Equal(t, "admin", u.Username)

	// Upsert again with new hash.
	err = s.UpsertUser(ctx, "admin", "$2a$10$newhash")
	require.NoError(t, err)

	u2, err := s.UserByUsername(ctx, "admin")
	require.NoError(t, err)
	require.Equal(t, "$2a$10$newhash", u2.PasswordHash)
}
