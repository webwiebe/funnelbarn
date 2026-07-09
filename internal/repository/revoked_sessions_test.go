package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevokedSessions_Lifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Second)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	require.NoError(t, s.RevokeSession(ctx, "active-hash", future))
	require.NoError(t, s.RevokeSession(ctx, "expired-hash", past))

	// Only the future-expiry revocation is active as of now.
	active, err := s.ActiveRevokedSessions(ctx, now)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "active-hash", active[0].TokenHash)
	assert.True(t, active[0].ExpiresAt.After(now))

	// Pruning removes the expired one; the active one survives.
	require.NoError(t, s.DeleteExpiredRevokedSessions(ctx, now))

	active, err = s.ActiveRevokedSessions(ctx, now)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "active-hash", active[0].TokenHash)
}

func TestRevokeSession_Idempotent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Second)
	future := now.Add(time.Hour)

	require.NoError(t, s.RevokeSession(ctx, "dup-hash", future))
	// Revoking the same hash again must not error and must not duplicate.
	require.NoError(t, s.RevokeSession(ctx, "dup-hash", future.Add(time.Hour)))

	active, err := s.ActiveRevokedSessions(ctx, now)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "dup-hash", active[0].TokenHash)
}

func TestActiveRevokedSessions_Empty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	active, err := s.ActiveRevokedSessions(ctx, time.Now().UTC())
	require.NoError(t, err)
	assert.Empty(t, active)
}
