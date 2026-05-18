package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_InstanceSettings(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Missing key returns ("", false, nil).
	val, ok, err := s.GetInstanceSetting(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "", val)

	// Seed default exists from migration.
	val, ok, err = s.GetInstanceSetting(ctx, "geo_enabled")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "true", val)

	// SetInstanceSetting inserts a new key.
	require.NoError(t, s.SetInstanceSetting(ctx, "my_key", "my_value"))
	val, ok, err = s.GetInstanceSetting(ctx, "my_key")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "my_value", val)

	// SetInstanceSetting updates an existing key.
	require.NoError(t, s.SetInstanceSetting(ctx, "my_key", "updated"))
	val, _, _ = s.GetInstanceSetting(ctx, "my_key")
	assert.Equal(t, "updated", val)

	// GetAllInstanceSettings returns all keys.
	all, err := s.GetAllInstanceSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, "updated", all["my_key"])
	assert.Equal(t, "true", all["geo_enabled"])
}
