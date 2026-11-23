package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// seedEnvSessions stores two production sessions and one staging session.
func seedEnvSessions(t *testing.T, s interface {
	UpsertSession(context.Context, domain.Session) error
}, projectID string) {
	t.Helper()
	now := time.Now().UTC()
	for _, c := range []struct{ id, env, device string }{
		{"env-sess-1", "production", "mobile"},
		{"env-sess-2", "production", "mobile"},
		{"env-sess-3", "staging", "desktop"},
	} {
		require.NoError(t, s.UpsertSession(context.Background(), domain.Session{
			ID: c.id, ProjectID: projectID, FirstSeenAt: now, LastSeenAt: now,
			Environment: c.env, DeviceType: c.device,
		}))
	}
}

func TestStore_SessionReadsFilterByEnvironment(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Env Sessions", "env-sessions")
	require.NoError(t, err)
	seedEnvSessions(t, s, p.ID)

	for _, c := range []struct {
		env  string
		want int
	}{{"", 3}, {"production", 2}, {"staging", 1}, {"testing", 0}} {
		list, err := s.ListSessions(ctx, p.ID, c.env, 10, 0)
		require.NoError(t, err)
		assert.Len(t, list, c.want, "list env=%q", c.env)

		count, err := s.ActiveSessionCount(ctx, p.ID, c.env, 5)
		require.NoError(t, err)
		assert.EqualValues(t, c.want, count, "active env=%q", c.env)
	}

	prod, err := s.SessionDistributions(ctx, p.ID, "production")
	require.NoError(t, err)
	require.NotEmpty(t, prod["device_type"])
	assert.Equal(t, "mobile", prod["device_type"][0].Value)
	assert.EqualValues(t, 2, prod["device_type"][0].Count)

	stg, err := s.SessionDistributions(ctx, p.ID, "staging")
	require.NoError(t, err)
	require.Len(t, stg["device_type"], 1)
	assert.Equal(t, "desktop", stg["device_type"][0].Value)
}
