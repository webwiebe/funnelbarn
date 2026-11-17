package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestAPIKeyRevoke_DeletesOneKeyAndKeepsTheLastOfAProject(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "revoke.db")}
	require.NoError(t, runAPIKeyCmd(cfg, []string{"create", "--project=site", "--name=old", "--scope=ingest"}))
	require.NoError(t, runAPIKeyCmd(cfg, []string{"create", "--project=site", "--name=new", "--scope=ingest"}))
	require.NoError(t, runAPIKeyCmd(cfg, []string{"create", "--project=other", "--name=only", "--scope=ingest"}))

	keysOf := func() map[string]string {
		store, err := repository.Open(cfg.DBPath)
		require.NoError(t, err)
		defer store.Close()
		keys, err := store.ListAllAPIKeys(context.Background())
		require.NoError(t, err)
		out := map[string]string{}
		for _, k := range keys {
			out[k.Name] = k.ID
		}
		return out
	}
	ids := keysOf()
	require.Len(t, ids, 3)

	// The old key goes; a project's only key and an unknown id are refused.
	require.NoError(t, runAPIKeyCmd(cfg, []string{"revoke", "--id=" + ids["old"]}))
	assert.NotContains(t, keysOf(), "old")
	assert.ErrorContains(t, runAPIKeyCmd(cfg, []string{"revoke", "--id=" + ids["new"]}), "only key")
	assert.ErrorContains(t, runAPIKeyCmd(cfg, []string{"revoke", "--id=" + ids["only"]}), "only key")
	assert.ErrorContains(t, runAPIKeyCmd(cfg, []string{"revoke", "--id=nope"}), "no API key")
	assert.ErrorContains(t, runAPIKeyCmd(cfg, []string{"revoke"}), "--id is required")
	assert.Len(t, keysOf(), 2)
}
