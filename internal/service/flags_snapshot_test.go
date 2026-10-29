package service_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/mock"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// stallingFlagRepo blocks FlagByKey while stalled, as SQLite did in the
// layer7 disk stalls, and counts the calls.
type stallingFlagRepo struct {
	ports.FlagRepo
	stalled atomic.Bool
	release chan struct{}
	reads   atomic.Int32
}

func (r *stallingFlagRepo) FlagByKey(ctx context.Context, projectID, key string) (repository.FeatureFlag, error) {
	r.reads.Add(1)
	if r.stalled.Load() {
		select {
		case <-r.release:
		case <-ctx.Done():
			return repository.FeatureFlag{}, ctx.Err()
		}
	}
	return r.FlagRepo.FlagByKey(ctx, projectID, key)
}

func newSnapshotFlagService(t *testing.T) (*service.FlagService, *service.FlagSnapshot, *stallingFlagRepo, *mock.Store) {
	t.Helper()
	store := mock.New()
	repo := &stallingFlagRepo{FlagRepo: store, release: make(chan struct{})}
	t.Cleanup(func() { close(repo.release) })
	fs := service.NewFlagSnapshot(func(ctx context.Context) ([]repository.FeatureFlag, error) {
		if repo.stalled.Load() {
			select {
			case <-repo.release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return store.ListFlags(ctx, "proj-1")
	}, nil)
	return service.NewFlagService(repo).WithSnapshot(fs), fs, repo, store
}

// A blocked database read does not block evaluate once the snapshot holds the
// flag: neither the flag read nor a stalled refresh is on the request path.
func TestFlagSnapshot_BlockedReadDoesNotBlockEvaluate(t *testing.T) {
	svc, fs, repo, _ := newSnapshotFlagService(t)
	createTestFlag(t, svc, "proj-1", "gate", map[string]any{"on": true, "off": false}, map[string]int{"on": 100, "off": 0}, "off")
	require.NoError(t, fs.Refresh(context.Background()))

	repo.stalled.Store(true)
	refreshCtx, cancelRefresh := context.WithCancel(context.Background())
	defer cancelRefresh()
	go func() { _ = fs.Refresh(refreshCtx) }() // a refresh stuck in the stall

	type outcome struct {
		res service.FlagEvalResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := svc.EvaluateFlag(context.Background(), "proj-1", "gate", map[string]any{"targetingKey": "u1"})
		done <- outcome{res, err}
	}()
	select {
	case o := <-done:
		require.NoError(t, o.err)
		require.Equal(t, "on", o.res.Variant)
	case <-time.After(5 * time.Second):
		t.Fatal("evaluate blocked on the stalled database")
	}
	require.Zero(t, repo.reads.Load(), "evaluate must not read the flag from the database")
}

// Before the first load, and for a flag the snapshot does not hold, evaluate
// reads SQLite.
func TestFlagSnapshot_ColdAndMissingFallBackToDatabase(t *testing.T) {
	svc, fs, repo, _ := newSnapshotFlagService(t)
	createTestFlag(t, svc, "proj-1", "gate", map[string]any{"on": true, "off": false}, map[string]int{"on": 100, "off": 0}, "off")

	_, err := svc.EvaluateFlag(context.Background(), "proj-1", "gate", nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, repo.reads.Load(), "a cold snapshot must fall back to the database")

	require.NoError(t, fs.Refresh(context.Background()))
	createTestFlag(t, svc, "proj-1", "newer", map[string]any{"on": true, "off": false}, map[string]int{"on": 100, "off": 0}, "off")
	_, err = svc.EvaluateFlag(context.Background(), "proj-1", "newer", nil)
	require.NoError(t, err)
	require.EqualValues(t, 2, repo.reads.Load(), "a flag created after the last load must come from the database")
}

// A flag edit reaches evaluate on the next refresh, and a failed refresh keeps
// serving the last good value.
func TestFlagSnapshot_EditShowsAfterRefresh(t *testing.T) {
	svc, fs, _, store := newSnapshotFlagService(t)
	f := createTestFlag(t, svc, "proj-1", "gate", map[string]any{"on": true, "off": false}, map[string]int{"on": 100, "off": 0}, "off")
	require.NoError(t, fs.Refresh(context.Background()))

	f.Status = "paused"
	_, err := store.UpdateFlag(context.Background(), f)
	require.NoError(t, err)
	res, err := svc.EvaluateFlag(context.Background(), "proj-1", "gate", nil)
	require.NoError(t, err)
	require.Equal(t, "SPLIT", res.Reason, "until the next refresh the snapshot answers")

	require.NoError(t, fs.Refresh(context.Background()))
	res, err = svc.EvaluateFlag(context.Background(), "proj-1", "gate", nil)
	require.NoError(t, err)
	require.Equal(t, "DISABLED", res.Reason, "the edit must show after the refresh")
	require.Less(t, fs.Age(), time.Second)
	require.LessOrEqual(t, service.FlagSnapshotInterval, 5*time.Second, "a flag edit must show within a few seconds")
}
