package repository_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func fullWebSession(idHash string) repository.WebSession {
	now := time.Now().Unix()
	return repository.WebSession{
		IDHash:            idHash,
		Username:          "alice",
		AuthMethod:        "oidc",
		IdpSub:            "sub-1",
		IdpSid:            "sid-1",
		IDToken:           "idt",
		AccessToken:       "at",
		RefreshToken:      "rt",
		AccessExpiresAt:   now + 900,
		ClaimsJSON:        `{"groups":["funnelbarn-users"]}`,
		CreatedAt:         now,
		AbsoluteExpiresAt: now + 43200,
	}
}

func TestWebSessions_CreateGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	in := fullWebSession("hash-1")
	require.NoError(t, s.CreateWebSession(ctx, in))

	got, err := s.GetWebSession(ctx, "hash-1")
	require.NoError(t, err)
	assert.Equal(t, in, got)

	_, err = s.GetWebSession(ctx, "unknown")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestWebSessions_LocalSessionNullTokens(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().Unix()
	require.NoError(t, s.CreateWebSession(ctx, repository.WebSession{
		IDHash:            "local-1",
		Username:          "admin",
		AuthMethod:        "local",
		CreatedAt:         now,
		AbsoluteExpiresAt: now + 3600,
	}))

	got, err := s.GetWebSession(ctx, "local-1")
	require.NoError(t, err)
	assert.Equal(t, "local", got.AuthMethod)
	assert.Empty(t, got.AccessToken)
	assert.Empty(t, got.RefreshToken)
	assert.Empty(t, got.IDToken)
	assert.Zero(t, got.AccessExpiresAt)
}

func TestWebSessions_UpdateTokensRotatesAndClearsFailing(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	require.NoError(t, s.CreateWebSession(ctx, fullWebSession("hash-2")))
	require.NoError(t, s.MarkWebSessionRefreshFailing(ctx, "hash-2", 111))

	now := time.Now().Unix()
	require.NoError(t, s.UpdateWebSessionTokens(ctx, "hash-2",
		"idt-new", "at-new", "rt-new", now+900, `{"groups":["a","b"]}`, now))

	got, err := s.GetWebSession(ctx, "hash-2")
	require.NoError(t, err)
	assert.Equal(t, "idt-new", got.IDToken)
	assert.Equal(t, "at-new", got.AccessToken)
	assert.Equal(t, "rt-new", got.RefreshToken)
	assert.Equal(t, `{"groups":["a","b"]}`, got.ClaimsJSON)
	assert.Equal(t, now, got.LastRefreshAt)
	assert.Zero(t, got.RefreshFailingSince, "successful refresh must clear refresh_failing_since")
}

func TestWebSessions_UpdateTokensKeepsClaimsWhenEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	require.NoError(t, s.CreateWebSession(ctx, fullWebSession("hash-3")))
	now := time.Now().Unix()
	// A refresh response without an id_token must not wipe the snapshot.
	require.NoError(t, s.UpdateWebSessionTokens(ctx, "hash-3", "", "at-new", "rt-new", now+900, "", now))

	got, err := s.GetWebSession(ctx, "hash-3")
	require.NoError(t, err)
	assert.Equal(t, "idt", got.IDToken, "empty id_token must keep the previous value")
	assert.Equal(t, `{"groups":["funnelbarn-users"]}`, got.ClaimsJSON)
}

func TestWebSessions_MarkRefreshFailingSetsOnlyOnce(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	require.NoError(t, s.CreateWebSession(ctx, fullWebSession("hash-4")))
	require.NoError(t, s.MarkWebSessionRefreshFailing(ctx, "hash-4", 100))
	// A later retry must NOT move the stamp — grace is measured from the
	// FIRST failure.
	require.NoError(t, s.MarkWebSessionRefreshFailing(ctx, "hash-4", 200))

	got, err := s.GetWebSession(ctx, "hash-4")
	require.NoError(t, err)
	assert.Equal(t, int64(100), got.RefreshFailingSince)
}

func TestWebSessions_DeleteBySidAndSub(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	a := fullWebSession("h-a") // sub-1 / sid-1
	b := fullWebSession("h-b")
	b.IdpSid = "sid-2" // sub-1 / sid-2
	c := fullWebSession("h-c")
	c.IdpSub, c.IdpSid = "sub-other", "sid-other"
	for _, ws := range []repository.WebSession{a, b, c} {
		require.NoError(t, s.CreateWebSession(ctx, ws))
	}

	// By sid: only the exact IdP session dies.
	n, err := s.DeleteWebSessionsByIdpSid(ctx, "sid-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	_, err = s.GetWebSession(ctx, "h-a")
	assert.ErrorIs(t, err, sql.ErrNoRows)

	// By sub: every remaining session of the subject dies; others survive.
	n, err = s.DeleteWebSessionsByIdpSub(ctx, "sub-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	_, err = s.GetWebSession(ctx, "h-c")
	assert.NoError(t, err)
}

func TestWebSessions_DeleteIdempotentAndPrune(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now()
	live := fullWebSession("h-live")
	expired := fullWebSession("h-expired")
	expired.AbsoluteExpiresAt = now.Add(-time.Minute).Unix()
	require.NoError(t, s.CreateWebSession(ctx, live))
	require.NoError(t, s.CreateWebSession(ctx, expired))

	// Deleting a nonexistent row is not an error.
	require.NoError(t, s.DeleteWebSession(ctx, "never-existed"))

	n, err := s.DeleteExpiredWebSessions(ctx, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	_, err = s.GetWebSession(ctx, "h-live")
	assert.NoError(t, err)
	_, err = s.GetWebSession(ctx, "h-expired")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}
