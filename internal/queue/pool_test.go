package queue_test

import (
	"testing"

	"github.com/redis/go-redis/v9/maintnotifications"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/queue"
)

func TestNewClientKeepsConnectionsWarm(t *testing.T) {
	client, err := queue.NewClient("redis://funnelbarn:pw@valkey.example:6379/0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	opts := client.Options()
	require.Equal(t, 4, opts.MinIdleConns)
	require.True(t, opts.DisableIdentity)
	require.NotNil(t, opts.MaintNotificationsConfig)
	require.Equal(t, maintnotifications.ModeDisabled, opts.MaintNotificationsConfig.Mode)
	require.Equal(t, "funnelbarn", opts.Username)
}
