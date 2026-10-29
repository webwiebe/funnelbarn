package main

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// startEvaluateSnapshots starts the in-memory flag and API key snapshots that
// let POST /api/v1/evaluate answer during a SQLite stall. They refresh on
// their own goroutines until ctx is done.
func startEvaluateSnapshots(ctx context.Context, store *repository.Store) (*service.FlagSnapshot, *auth.KeySnapshot) {
	flags := service.NewFlagSnapshot(store.ListAllFlags, nil)
	keys := auth.NewKeySnapshot(keyGrants(store.ListAllAPIKeys), repository.APIKeyScopeEvaluatesFlags, nil)
	metrics.SetSnapshotAge("flags", flags.Age)
	metrics.SetSnapshotAge("api_keys", keys.Age)
	flags.Run(ctx)
	keys.Run(ctx)
	return flags, keys
}

// keyGrants adapts the stored key list to what the key snapshot holds.
func keyGrants(list func(context.Context) ([]repository.APIKey, error)) func(context.Context) ([]auth.KeyGrant, error) {
	return func(ctx context.Context) ([]auth.KeyGrant, error) {
		keys, err := list(ctx)
		if err != nil {
			return nil, err
		}
		grants := make([]auth.KeyGrant, 0, len(keys))
		for _, k := range keys {
			grants = append(grants, auth.KeyGrant{KeySHA256: k.KeyHash, ProjectID: k.ProjectID, Scope: k.Scope})
		}
		return grants, nil
	}
}
