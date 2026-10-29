package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// dbStall blocks the reads and writes a test routes through it while stalled,
// as SQLite did in the layer7 disk stalls of 2026-10-29. reads counts the
// request-path reads; touches counts the last_used_at writes.
type dbStall struct {
	stalled atomic.Bool
	release chan struct{}
	reads   atomic.Int32
	touches atomic.Int32
}

func newDBStall(t *testing.T) *dbStall {
	d := &dbStall{release: make(chan struct{})}
	t.Cleanup(func() { close(d.release) })
	return d
}

func (d *dbStall) wait(ctx context.Context) error {
	d.reads.Add(1)
	return d.block(ctx)
}

func (d *dbStall) block(ctx context.Context) error {
	if !d.stalled.Load() {
		return nil
	}
	select {
	case <-d.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// discardBus takes commands and applies none. With it the flag service queues
// its bookkeeping as production does, so the only database reads left on the
// evaluate path are the key lookup and the flag read.
type discardBus struct{}

func (discardBus) Start(context.Context)                                 {}
func (discardBus) Submit(context.Context, command.Command) time.Duration { return 0 }
func (discardBus) Flush(context.Context) error                           { return nil }
func (discardBus) Close(context.Context) error                           { return nil }

type stallingFlagStore struct {
	ports.FlagRepo
	stall *dbStall
}

func (s stallingFlagStore) FlagByKey(ctx context.Context, projectID, key string) (repository.FeatureFlag, error) {
	if err := s.stall.wait(ctx); err != nil {
		return repository.FeatureFlag{}, err
	}
	return s.FlagRepo.FlagByKey(ctx, projectID, key)
}

func storeKeyGrants(store *repository.Store) func(context.Context) ([]auth.KeyGrant, error) {
	return func(ctx context.Context) ([]auth.KeyGrant, error) {
		keys, err := store.ListAllAPIKeys(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]auth.KeyGrant, 0, len(keys))
		for _, k := range keys {
			out = append(out, auth.KeyGrant{KeySHA256: k.KeyHash, ProjectID: k.ProjectID, Scope: k.Scope})
		}
		return out, nil
	}
}

// newSnapshotEvaluateServer wires the evaluate path as production does: the
// key and flag snapshots in front of SQLite reads that stall can block.
func newSnapshotEvaluateServer(t *testing.T) (*Server, *repository.Store, *dbStall, func()) {
	t.Helper()
	stall := newDBStall(t)
	var keys *auth.KeySnapshot
	var flags *service.FlagSnapshot
	srv, store := newFlagTokenServerOpts(t, flagTokenOpts{
		wrapLookup: func(next auth.DBKeyLookupWithProject) auth.DBKeyLookupWithProject {
			return func(ctx context.Context, sum string) (string, string, bool, error) {
				if err := stall.wait(ctx); err != nil {
					return "", "", false, err
				}
				return next(ctx, sum)
			}
		},
		wrapTouch: func(next auth.DBKeyTouch) auth.DBKeyTouch {
			return func(ctx context.Context, sum string) error {
				stall.touches.Add(1)
				if err := stall.block(ctx); err != nil {
					return err
				}
				return next(ctx, sum)
			}
		},
		authz: func(a *auth.Authorizer, store *repository.Store) *auth.Authorizer {
			keys = auth.NewKeySnapshot(storeKeyGrants(store), repository.APIKeyScopeEvaluatesFlags, nil)
			return a.WithEvaluateKeys(keys)
		},
		flags: func(store *repository.Store) *service.FlagService {
			flags = service.NewFlagSnapshot(store.ListAllFlags, nil)
			return service.NewFlagService(stallingFlagStore{FlagRepo: store, stall: stall}).
				WithCommands(discardBus{}).WithSnapshot(flags)
		},
	})
	refresh := func() {
		t.Helper()
		if err := keys.Refresh(context.Background()); err != nil {
			t.Fatalf("key snapshot: %v", err)
		}
		if err := flags.Refresh(context.Background()); err != nil {
			t.Fatalf("flag snapshot: %v", err)
		}
	}
	return srv, store, stall, refresh
}

// A blocked database does not block evaluate: with both snapshots loaded,
// POST /api/v1/evaluate answers while the key lookup, the flag read and the
// last_used_at touch would all wait on the stall.
func TestEvaluate_BlockedDatabaseReadDoesNotBlockEvaluate(t *testing.T) {
	srv, store, stall, refresh := newSnapshotEvaluateServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedFlag(t, store, p.ID, "gate")
	key := scopedKey(t, store, p.ID, "sdk-key", repository.APIKeyScopeIngest)
	refresh()

	stall.stalled.Store(true)
	before := stall.reads.Load()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- withKey(t, srv, http.MethodPost, "/api/v1/evaluate", key, map[string]any{"flag_key": "gate"})
	}()
	select {
	case w := <-done:
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("evaluate blocked on the stalled database")
	}
	if n := stall.reads.Load() - before; n != 0 {
		t.Fatalf("evaluate must not read the key or the flag from the database, got %d reads", n)
	}
	// The touch did run, off the request, and is still blocked in the stall.
	deadline := time.Now().Add(5 * time.Second)
	for stall.touches.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if stall.touches.Load() != 1 {
		t.Fatalf("want the blocked last_used_at touch to have started once, got %d", stall.touches.Load())
	}
}

// Before the snapshots load, evaluate does the live reads.
func TestEvaluate_ColdSnapshotsReadDatabase(t *testing.T) {
	srv, store, stall, _ := newSnapshotEvaluateServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedFlag(t, store, p.ID, "gate")
	key := scopedKey(t, store, p.ID, "sdk-key", repository.APIKeyScopeIngest)

	w := withKey(t, srv, http.MethodPost, "/api/v1/evaluate", key, map[string]any{"flag_key": "gate"})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if n := stall.reads.Load(); n != 2 {
		t.Fatalf("a cold reader must look up the key and the flag in the database, got %d reads", n)
	}
}

// The key snapshot answers the evaluate route only. A revoked full key still in
// the snapshot may evaluate flags until the next refresh, and every other
// route refuses it at once.
func TestEvaluate_RevokedFullKeyRefusedElsewhereAtOnce(t *testing.T) {
	srv, store, _, refresh := newSnapshotEvaluateServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedFlag(t, store, p.ID, "gate")
	key := scopedKey(t, store, p.ID, "full-key", repository.APIKeyScopeFull)
	refresh()

	keys, err := store.ListAPIKeys(context.Background(), p.ID)
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListAPIKeys: %v, %d keys", err, len(keys))
	}
	if err := store.DeleteAPIKey(context.Background(), keys[0].ID); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}

	if w := withKey(t, srv, http.MethodPost, "/api/v1/evaluate", key, map[string]any{"flag_key": "gate"}); w.Code != http.StatusOK {
		t.Fatalf("within the snapshot window evaluate answers, got %d", w.Code)
	}
	if w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/flags", key, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("a revoked key must be refused on the flags list at once, got %d", w.Code)
	}
	if w := withKey(t, srv, http.MethodPost, "/api/v1/events", key, map[string]any{"name": "x"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("a revoked key must be refused on ingest at once, got %d", w.Code)
	}

	refresh()
	if w := withKey(t, srv, http.MethodPost, "/api/v1/evaluate", key, map[string]any{"flag_key": "gate"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("after the next refresh evaluate must refuse the revoked key, got %d", w.Code)
	}
}
