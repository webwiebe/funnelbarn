package repository_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestStore_CreateProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Happy Path", "happy-path")
	require.NoError(t, err)
	assert.NotEmpty(t, p.ID)
	assert.Equal(t, "Happy Path", p.Name)
	assert.Equal(t, "happy-path", p.Slug)
	assert.Equal(t, "active", p.Status)
	assert.False(t, p.CreatedAt.IsZero())
}

func TestStore_CreateProject_DuplicateSlug(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.CreateProject(ctx, "First", "duplicate-slug")
	require.NoError(t, err)

	_, err = s.CreateProject(ctx, "Second", "duplicate-slug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UNIQUE")
}

func TestStore_ProjectByID_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.ProjectByID(ctx, "nonexistent-id")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestStore_ListProjects_Empty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	projects, err := s.ListProjects(ctx)
	require.NoError(t, err)
	assert.NotNil(t, projects)
	assert.Empty(t, projects)
}

func TestStore_UpdateProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Original", "the-slug")
	require.NoError(t, err)

	updated, err := s.UpdateProject(ctx, p.ID, "Updated Name", "example.org")
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, "example.org", updated.Domain)
	assert.Equal(t, "the-slug", updated.Slug) // slug unchanged
}

func TestStore_DeleteProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Temp", "temp")
	require.NoError(t, err)

	err = s.DeleteProject(ctx, p.ID)
	require.NoError(t, err)

	_, err = s.ProjectByID(ctx, p.ID)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestStore_ApproveProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.EnsureProjectPending(ctx, "Site", "site-approve")
	require.NoError(t, err)
	assert.Equal(t, "pending", p.Status)

	approved, err := s.ApproveProject(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", approved.Status)
}

func TestStore_EnsureProjectPending(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.EnsureProjectPending(ctx, "New Site", "new-site")
	require.NoError(t, err)
	assert.Equal(t, "pending", p.Status)
	assert.Equal(t, "New Site", p.Name)

	// Second call should return same project unchanged.
	p2, err := s.EnsureProjectPending(ctx, "New Site", "new-site")
	require.NoError(t, err)
	assert.Equal(t, p.ID, p2.ID)
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

	p, err := s.EnsureProjectPending(ctx, "New Site", "new-site-legacy")
	require.NoError(t, err)
	require.Equal(t, "pending", p.Status)

	// Second call should return same project unchanged.
	p2, err := s.EnsureProjectPending(ctx, "New Site", "new-site-legacy")
	require.NoError(t, err)
	require.Equal(t, p.ID, p2.ID)
}

func TestApproveProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.EnsureProjectPending(ctx, "Site", "site-legacy")
	require.NoError(t, err)
	require.Equal(t, "pending", p.Status)

	approved, err := s.ApproveProject(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "active", approved.Status)
}

func TestDeleteProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Temp", "temp-legacy")
	require.NoError(t, err)

	err = s.DeleteProject(ctx, p.ID)
	require.NoError(t, err)

	projects, err := s.ListProjects(ctx)
	require.NoError(t, err)
	require.Empty(t, projects)
}

// --------------------------------------------------------------------------
// API Keys
// --------------------------------------------------------------------------

func TestStore_CreateAPIKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-apikey")
	require.NoError(t, err)

	k, err := s.CreateAPIKey(ctx, "my-key", p.ID, "abc123sha256hashvalue0000000000000000000000000000000000000000001", "ingest")
	require.NoError(t, err)
	assert.NotEmpty(t, k.ID)
	assert.Equal(t, "my-key", k.Name)
	assert.Equal(t, p.ID, k.ProjectID)
	assert.Equal(t, "ingest", k.Scope)
	assert.False(t, k.CreatedAt.IsZero())
}

func TestStore_ListAPIKeys_ByProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p1, err := s.CreateProject(ctx, "Proj1", "proj1-apikey")
	require.NoError(t, err)
	p2, err := s.CreateProject(ctx, "Proj2", "proj2-apikey")
	require.NoError(t, err)

	_, err = s.CreateAPIKey(ctx, "key-p1", p1.ID, "hash1111111111111111111111111111111111111111111111111111111111111", "ingest")
	require.NoError(t, err)
	_, err = s.CreateAPIKey(ctx, "key-p2", p2.ID, "hash2222222222222222222222222222222222222222222222222222222222222", "full")
	require.NoError(t, err)

	keys, err := s.ListAPIKeys(ctx, p1.ID)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, "key-p1", keys[0].Name)
}

func TestStore_DeleteAPIKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-deletekey")
	require.NoError(t, err)

	k, err := s.CreateAPIKey(ctx, "temp-key", p.ID, "hash3333333333333333333333333333333333333333333333333333333333333", "ingest")
	require.NoError(t, err)

	err = s.DeleteAPIKey(ctx, k.ID)
	require.NoError(t, err)

	keys, err := s.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestStore_ValidAPIKeySHA256_Found(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-validkey")
	require.NoError(t, err)

	const hash = "hash4444444444444444444444444444444444444444444444444444444444444"
	_, err = s.CreateAPIKey(ctx, "lookup-key", p.ID, hash, "ingest")
	require.NoError(t, err)

	projectID, scope, found, err := s.ValidAPIKeySHA256(ctx, hash)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, p.ID, projectID)
	assert.Equal(t, "ingest", scope)
}

func TestStore_ValidAPIKeySHA256_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, _, found, err := s.ValidAPIKeySHA256(ctx, "nonexistent-hash")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestStore_TouchAPIKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-touchkey")
	require.NoError(t, err)

	const hash = "hash5555555555555555555555555555555555555555555555555555555555555"
	_, err = s.CreateAPIKey(ctx, "touch-key", p.ID, hash, "ingest")
	require.NoError(t, err)

	err = s.TouchAPIKey(ctx, hash)
	require.NoError(t, err)

	// Verify last_used_at was updated via direct query on DB.
	var lastUsedAt sql.NullTime
	err = s.DB().QueryRowContext(ctx, `SELECT last_used_at FROM api_keys WHERE key_hash = ?`, hash).Scan(&lastUsedAt)
	require.NoError(t, err)
	assert.True(t, lastUsedAt.Valid)
}

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

	p, err := s.CreateProject(ctx, "Proj", "proj-list")
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

	p, err := s.CreateProject(ctx, "Proj", "proj-del")
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

	p, err := s.CreateProject(ctx, "Proj", "proj-valid")
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
// Funnels
// --------------------------------------------------------------------------

func TestStore_CreateFunnel_WithSteps(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-funnel")
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
	assert.NotEmpty(t, f.ID)
	assert.Equal(t, "Sign Up Funnel", f.Name)
	assert.Equal(t, "Tracks sign-up flow", f.Description)
	require.Len(t, f.Steps, 3)
	assert.Equal(t, 1, f.Steps[0].StepOrder)
	assert.Equal(t, "page-view", f.Steps[0].EventName)
	assert.Equal(t, 2, f.Steps[1].StepOrder)
	assert.Equal(t, 3, f.Steps[2].StepOrder)
	assert.Equal(t, "signup-completed", f.Steps[2].EventName)
}

func TestStore_FunnelByID_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.FunnelByID(ctx, "nonexistent-funnel-id")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestStore_ListFunnels_IncludesSteps(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-listfunnel")
	require.NoError(t, err)

	_, err = s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Funnel With Steps",
		Steps: []repository.FunnelStep{
			{EventName: "step-a"},
			{EventName: "step-b"},
		},
	})
	require.NoError(t, err)

	funnels, err := s.ListFunnels(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, funnels, 1)
	assert.Len(t, funnels[0].Steps, 2)
	assert.Equal(t, "step-a", funnels[0].Steps[0].EventName)
}

func TestStore_UpdateFunnel(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-updatefunnel")
	require.NoError(t, err)

	f, err := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Original",
		Steps:     []repository.FunnelStep{{EventName: "step-1"}},
	})
	require.NoError(t, err)

	updated, err := s.UpdateFunnel(ctx, repository.Funnel{
		ID:          f.ID,
		ProjectID:   p.ID,
		Name:        "Updated Name",
		Description: "New description",
		Steps: []repository.FunnelStep{
			{EventName: "new-step-1"},
			{EventName: "new-step-2"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, "New description", updated.Description)
	require.Len(t, updated.Steps, 2)
	assert.Equal(t, "new-step-1", updated.Steps[0].EventName)
	assert.Equal(t, 1, updated.Steps[0].StepOrder)
	assert.Equal(t, 2, updated.Steps[1].StepOrder)
}

func TestStore_DeleteFunnel(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-delfunnel")
	require.NoError(t, err)

	f, err := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "To Delete",
		Steps:     []repository.FunnelStep{{EventName: "step-1"}},
	})
	require.NoError(t, err)

	err = s.DeleteFunnel(ctx, f.ID)
	require.NoError(t, err)

	_, err = s.FunnelByID(ctx, f.ID)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestCreateFunnel(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-cf")
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

	p, err := s.CreateProject(ctx, "Proj", "proj-lf")
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

	p, err := s.CreateProject(ctx, "Proj", "proj-df")
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
// Sessions
// --------------------------------------------------------------------------

func TestStore_UpsertSession_Create(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-session")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	sess := repository.Session{
		ID:          "session-001",
		ProjectID:   p.ID,
		FirstSeenAt: now,
		LastSeenAt:  now,
		EntryURL:    "https://example.com",
		DeviceType:  "desktop",
	}
	err = s.UpsertSession(ctx, sess)
	require.NoError(t, err)

	got, err := s.SessionByID(ctx, "session-001")
	require.NoError(t, err)
	assert.Equal(t, "session-001", got.ID)
	assert.Equal(t, p.ID, got.ProjectID)
	assert.Equal(t, "https://example.com", got.EntryURL)
	assert.Equal(t, 1, got.EventCount)
}

func TestStore_UpsertSession_Update(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-sessionupdate")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	sess := repository.Session{
		ID:          "session-update",
		ProjectID:   p.ID,
		FirstSeenAt: now,
		LastSeenAt:  now,
	}
	err = s.UpsertSession(ctx, sess)
	require.NoError(t, err)

	// Upsert again should increment event_count.
	sess.LastSeenAt = now.Add(time.Minute)
	err = s.UpsertSession(ctx, sess)
	require.NoError(t, err)

	got, err := s.SessionByID(ctx, "session-update")
	require.NoError(t, err)
	assert.Equal(t, 2, got.EventCount)
}

// --------------------------------------------------------------------------
// Users
// --------------------------------------------------------------------------

func TestStore_UpsertUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	err := s.UpsertUser(ctx, "testuser", "$2a$10$fakehash")
	require.NoError(t, err)

	u, err := s.UserByUsername(ctx, "testuser")
	require.NoError(t, err)
	assert.Equal(t, "testuser", u.Username)
	assert.Equal(t, "$2a$10$fakehash", u.PasswordHash)
	assert.NotEmpty(t, u.ID)
}

func TestStore_UserByUsername_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.UserByUsername(ctx, "nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

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

// --------------------------------------------------------------------------
// A/B Tests
// --------------------------------------------------------------------------

func TestStore_CreateABTest(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-abtest")
	require.NoError(t, err)

	test, err := s.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Button Color Test",
		Status:          "running",
		ConversionEvent: "purchase",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, test.ID)
	assert.Equal(t, "Button Color Test", test.Name)
	assert.Equal(t, "running", test.Status)
	assert.Equal(t, "purchase", test.ConversionEvent)
	assert.Equal(t, p.ID, test.ProjectID)
	assert.False(t, test.CreatedAt.IsZero())
}

func TestStore_ABTestByID_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.ABTestByID(ctx, "nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestStore_ListABTests(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-listabtests")
	require.NoError(t, err)

	_, err = s.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Test A",
		Status:          "running",
		ConversionEvent: "click",
	})
	require.NoError(t, err)

	_, err = s.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Test B",
		Status:          "paused",
		ConversionEvent: "signup",
	})
	require.NoError(t, err)

	tests, err := s.ListABTests(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, tests, 2)
}

// --------------------------------------------------------------------------
// Events
// --------------------------------------------------------------------------

var eventCounter int

func newEventID() string {
	eventCounter++
	return fmt.Sprintf("event-%06d", eventCounter)
}

func newTestEvent(projectID, sessionID string) repository.Event {
	return repository.Event{
		ID:         newEventID(),
		ProjectID:  projectID,
		SessionID:  sessionID,
		Name:       "page-view",
		URL:        "https://example.com/page",
		IngestID:   newEventID(),
		OccurredAt: time.Now().UTC(),
	}
}

func TestStore_InsertAndListEvents(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-events")
	require.NoError(t, err)

	e := newTestEvent(p.ID, "session-ev-001")
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	events, err := s.ListEvents(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, e.Name, events[0].Name)
}

func TestStore_CountEvents(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-countevents")
	require.NoError(t, err)

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		e := newTestEvent(p.ID, "session-count")
		e.OccurredAt = now
		err = s.InsertEvent(ctx, e)
		require.NoError(t, err)
	}

	n, err := s.CountEvents(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
}

func TestStore_TopPages(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-toppages")
	require.NoError(t, err)

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		e := newTestEvent(p.ID, "session-pages")
		e.URL = "https://example.com/home"
		e.OccurredAt = now
		err = s.InsertEvent(ctx, e)
		require.NoError(t, err)
	}
	e2 := newTestEvent(p.ID, "session-pages2")
	e2.URL = "https://example.com/about"
	e2.OccurredAt = now
	err = s.InsertEvent(ctx, e2)
	require.NoError(t, err)

	pages, err := s.TopPages(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, pages)
	// Home should be first with 3 views.
	assert.Equal(t, "https://example.com/home", pages[0].URL)
	assert.Equal(t, int64(3), pages[0].Views)
}

func TestStore_PurgeOldEvents(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-purge")
	require.NoError(t, err)

	past := time.Now().UTC().Add(-48 * time.Hour)
	e := newTestEvent(p.ID, "session-purge")
	e.OccurredAt = past
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	deleted, err := s.PurgeOldEvents(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	events, err := s.ListEvents(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestStore_GetEventByIngestID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-ingestid")
	require.NoError(t, err)

	e := newTestEvent(p.ID, "session-ingestid")
	e.IngestID = "unique-ingest-id-abc123"
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	found, err := s.GetEventByIngestID(ctx, "unique-ingest-id-abc123")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, e.Name, found.Name)
}

func TestStore_TopReferrers(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-topreferrers")
	require.NoError(t, err)

	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		e := newTestEvent(p.ID, "session-referrer")
		e.ReferrerDomain = "google.com"
		e.OccurredAt = now
		err = s.InsertEvent(ctx, e)
		require.NoError(t, err)
	}

	refs, err := s.TopReferrers(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, refs)
}

func TestStore_UniqueSessionCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-uniqsessions")
	require.NoError(t, err)

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		e := newTestEvent(p.ID, fmt.Sprintf("session-uniq-%d", i))
		e.OccurredAt = now
		err = s.InsertEvent(ctx, e)
		require.NoError(t, err)
	}

	n, err := s.UniqueSessionCount(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
}

func TestStore_DailyEventCounts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-daily")
	require.NoError(t, err)

	now := time.Now().UTC()
	// No events inserted — query on empty range returns empty series without error.
	series, err := s.DailyEventCounts(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, series)
}

func TestStore_BounceRate(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-bounce")
	require.NoError(t, err)

	now := time.Now().UTC()
	// One session with one event (bounce).
	e := newTestEvent(p.ID, "session-bounce")
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	rate, err := s.BounceRate(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1.0, rate) // 100% bounce rate
}

func TestStore_TopBrowsers(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-browsers")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := newTestEvent(p.ID, "session-browser")
	e.Browser = "Chrome"
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	browsers, err := s.TopBrowsers(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, browsers)
}

func TestStore_TopDeviceTypes(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-devices")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := newTestEvent(p.ID, "session-device")
	e.DeviceType = "desktop"
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	devices, err := s.TopDeviceTypes(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.NotEmpty(t, devices)
}

func TestStore_EnsureSetupAPIKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-setupkey")
	require.NoError(t, err)

	const hash = "hash9999999999999999999999999999999999999999999999999999999999999"
	err = s.EnsureSetupAPIKey(ctx, p.ID, hash)
	require.NoError(t, err)

	// Idempotent
	err = s.EnsureSetupAPIKey(ctx, p.ID, hash)
	require.NoError(t, err)

	keys, err := s.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "setup", keys[0].Name)
}

func TestStore_ListAllAPIKeys(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p1, err := s.CreateProject(ctx, "P1", "p1-allkeys")
	require.NoError(t, err)
	p2, err := s.CreateProject(ctx, "P2", "p2-allkeys")
	require.NoError(t, err)

	_, err = s.CreateAPIKey(ctx, "k1", p1.ID, "allhash1111111111111111111111111111111111111111111111111111111111", "ingest")
	require.NoError(t, err)
	_, err = s.CreateAPIKey(ctx, "k2", p2.ID, "allhash2222222222222222222222222222222222222222222222222222222222", "full")
	require.NoError(t, err)

	all, err := s.ListAllAPIKeys(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestStore_HasProjects(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	has, err := s.HasProjects(ctx)
	require.NoError(t, err)
	assert.False(t, has)

	_, err = s.CreateProject(ctx, "First", "first-has")
	require.NoError(t, err)

	has, err = s.HasProjects(ctx)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestStore_ActiveSessionCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-activesess")
	require.NoError(t, err)

	now := time.Now().UTC()
	sess := repository.Session{
		ID:          "active-sess-001",
		ProjectID:   p.ID,
		FirstSeenAt: now,
		LastSeenAt:  now,
	}
	err = s.UpsertSession(ctx, sess)
	require.NoError(t, err)

	count, err := s.ActiveSessionCount(ctx, p.ID, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestStore_ListSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-listsess")
	require.NoError(t, err)

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		sess := repository.Session{
			ID:          fmt.Sprintf("list-sess-%d", i),
			ProjectID:   p.ID,
			FirstSeenAt: now,
			LastSeenAt:  now,
		}
		err = s.UpsertSession(ctx, sess)
		require.NoError(t, err)
	}

	sessions, err := s.ListSessions(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func TestStore_DailyUniqueSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-dailyuniq")
	require.NoError(t, err)

	now := time.Now().UTC()
	// No events inserted — query on empty range returns empty series without error.
	series, err := s.DailyUniqueSessions(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, series)
}

func TestStore_AvgEventsPerSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-avgepss")
	require.NoError(t, err)

	now := time.Now().UTC()
	// Session 1: 2 events
	for i := 0; i < 2; i++ {
		e := newTestEvent(p.ID, "session-avg-1")
		e.OccurredAt = now
		err = s.InsertEvent(ctx, e)
		require.NoError(t, err)
	}
	// Session 2: 1 event
	e := newTestEvent(p.ID, "session-avg-2")
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	avg, err := s.AvgEventsPerSession(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.InDelta(t, 1.5, avg, 0.01)
}

func TestStore_TopEventNames(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-topevents")
	require.NoError(t, err)

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		e := newTestEvent(p.ID, "session-topev")
		e.Name = "page-view"
		e.OccurredAt = now
		err = s.InsertEvent(ctx, e)
		require.NoError(t, err)
	}

	names, err := s.TopEventNames(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, names, 1)
	assert.Equal(t, "page-view", names[0].Name)
	assert.Equal(t, int64(3), names[0].Count)
}

func TestStore_TopUTMSources(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-utmsrc")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := newTestEvent(p.ID, "session-utm")
	e.UTMSource = "google"
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	srcs, err := s.TopUTMSources(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, srcs)
}

func TestStore_ProjectBySlug(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Slug Test", "slug-test")
	require.NoError(t, err)

	got, err := s.ProjectBySlug(ctx, "slug-test")
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
}

func TestStore_EnsureProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// First call should create the project.
	p, err := s.EnsureProject(ctx, "ensure-slug")
	require.NoError(t, err)
	assert.NotEmpty(t, p.ID)

	// Second call should return same project.
	p2, err := s.EnsureProject(ctx, "ensure-slug")
	require.NoError(t, err)
	assert.Equal(t, p.ID, p2.ID)
}

func TestStore_AnalyzeABTest(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-analyzeab")
	require.NoError(t, err)

	test, err := s.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Test",
		Status:          "running",
		ConversionEvent: "purchase",
	})
	require.NoError(t, err)

	now := time.Now().UTC()
	results, err := s.AnalyzeABTest(ctx, test, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestStore_TopCountries(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-countries")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := newTestEvent(p.ID, "session-countries")
	e.CountryCode = "US"
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	countries, err := s.TopCountries(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, countries)
}

func TestStore_TopOSSystems(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-os")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := newTestEvent(p.ID, "session-os")
	e.OS = "macOS"
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	oss, err := s.TopOSSystems(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, oss)
}

func TestStore_TopUTMCampaigns(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-utmcampaign")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := newTestEvent(p.ID, "session-utmcampaign")
	e.UTMCampaign = "summer-sale"
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	campaigns, err := s.TopUTMCampaigns(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, campaigns)
}

func TestStore_TopUTMMediums(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-utmmedium")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := newTestEvent(p.ID, "session-utmmedium")
	e.UTMMedium = "email"
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	mediums, err := s.TopUTMMediums(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, mediums)
}

func TestStore_CountNewEvents(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Proj", "proj-countnewe")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := newTestEvent(p.ID, "session-countnew")
	e.OccurredAt = now
	err = s.InsertEvent(ctx, e)
	require.NoError(t, err)

	n, err := s.CountNewEvents(ctx, p.ID, now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}
