package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestStore_SessionDistributions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Dist Project", "dist-project")
	require.NoError(t, err)

	now := time.Now().UTC()

	// Insert two sessions with different device types.
	for i, dt := range []string{"mobile", "mobile", "desktop"} {
		sess := repository.Session{
			ID:          "sess-dist-" + string(rune('a'+i)),
			ProjectID:   p.ID,
			FirstSeenAt: now,
			LastSeenAt:  now,
			DeviceType:  dt,
		}
		require.NoError(t, s.UpsertSession(ctx, sess))
	}

	dists, err := s.SessionDistributions(ctx, p.ID)
	require.NoError(t, err)

	dt, ok := dists["device_type"]
	require.True(t, ok, "device_type distribution should exist")
	assert.NotEmpty(t, dt)
	assert.Equal(t, "mobile", dt[0].Value)
	assert.EqualValues(t, 2, dt[0].Count)
}

func TestStore_SessionDistributions_Empty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Empty Dist", "empty-dist")
	require.NoError(t, err)

	dists, err := s.SessionDistributions(ctx, p.ID)
	require.NoError(t, err)
	// No sessions — all fields should have nil/empty slices.
	for _, v := range dists {
		assert.Empty(t, v)
	}
}
