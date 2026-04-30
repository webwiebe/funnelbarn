package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func TestSessionService_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	sessSvc := service.NewSessionService(store)

	p, err := projSvc.CreateProject(ctx, "Session Project", "session-project")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	sess := repository.Session{
		ID:          "sess-001",
		ProjectID:   p.ID,
		FirstSeenAt: now,
		LastSeenAt:  now,
		EntryURL:    "https://example.com",
		DeviceType:  "desktop",
	}
	err = sessSvc.UpsertSession(ctx, sess)
	require.NoError(t, err)

	got, err := sessSvc.SessionByID(ctx, "sess-001")
	require.NoError(t, err)
	assert.Equal(t, "sess-001", got.ID)
	assert.Equal(t, p.ID, got.ProjectID)
	assert.Equal(t, "desktop", got.DeviceType)
}

func TestSessionService_ListSessions(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	sessSvc := service.NewSessionService(store)

	p, err := projSvc.CreateProject(ctx, "Session Project", "session-project-list")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		sess := repository.Session{
			ID:          "sess-list-" + string(rune('a'+i)),
			ProjectID:   p.ID,
			FirstSeenAt: now,
			LastSeenAt:  now,
		}
		err = sessSvc.UpsertSession(ctx, sess)
		require.NoError(t, err)
	}

	sessions, err := sessSvc.ListSessions(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func TestSessionService_ActiveSessionCount(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	sessSvc := service.NewSessionService(store)

	p, err := projSvc.CreateProject(ctx, "Session Project", "session-project-active")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	sess := repository.Session{
		ID:          "active-sess",
		ProjectID:   p.ID,
		FirstSeenAt: now,
		LastSeenAt:  now,
	}
	err = sessSvc.UpsertSession(ctx, sess)
	require.NoError(t, err)

	count, err := sessSvc.ActiveSessionCount(ctx, p.ID, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}
