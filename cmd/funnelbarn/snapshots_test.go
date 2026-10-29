package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// noFlagRepo finds no flag, so an evaluate that succeeds was answered by the
// snapshot.
type noFlagRepo struct{ ports.FlagRepo }

func (noFlagRepo) FlagByKey(context.Context, string, string) (repository.FeatureFlag, error) {
	return repository.FeatureFlag{}, sql.ErrNoRows
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Both snapshots load on start. Once loaded, an ingest key and a flag are
// answered without the database, and an analytics:read key is left out.
func TestStartEvaluateSnapshots(t *testing.T) {
	store, p, f := newCommandTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := store.CreateAPIKey(ctx, "sdk", p.ID, sha("sdk-key"), repository.APIKeyScopeIngest)
	require.NoError(t, err)
	_, err = store.CreateAPIKey(ctx, "report", p.ID, sha("report-key"), repository.APIKeyScopeAnalyticsRead)
	require.NoError(t, err)

	flags, keys := startEvaluateSnapshots(ctx, store)
	noLookup := func(context.Context, string) (string, string, bool, error) { return "", "", false, nil }
	authz := auth.New("").WithDBLookup(noLookup, nil).WithEvaluateKeys(keys)
	require.Eventually(t, func() bool {
		pid, _, ok := authz.ValidForEvaluate(ctx, "sdk-key")
		return ok && pid == p.ID
	}, 5*time.Second, 10*time.Millisecond, "the key snapshot must load on start")
	_, _, ok := authz.ValidForEvaluate(ctx, "report-key")
	require.False(t, ok, "analytics:read keys stay out of the snapshot")

	svc := service.NewFlagService(noFlagRepo{}).WithSnapshot(flags)
	require.Eventually(t, func() bool {
		_, err := svc.PreviewFlag(ctx, p.ID, f.FlagKey, nil)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond, "the flag snapshot must load on start")
}

func TestKeyGrants(t *testing.T) {
	grants, err := keyGrants(func(context.Context) ([]repository.APIKey, error) {
		return []repository.APIKey{{KeyHash: "aa", ProjectID: "p", Scope: "ingest"}}, nil
	})(context.Background())
	require.NoError(t, err)
	require.Equal(t, []auth.KeyGrant{{KeySHA256: "aa", ProjectID: "p", Scope: "ingest"}}, grants)

	_, err = keyGrants(func(context.Context) ([]repository.APIKey, error) {
		return nil, errors.New("disk stall")
	})(context.Background())
	require.Error(t, err)
}
