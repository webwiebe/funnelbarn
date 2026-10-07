package api

// Mode equivalence for the Redis command bus (issue #302, phase 2).
//
// The server runs its bookkeeping commands through a RedisBus over miniredis
// (consuming, with a Dispatcher as fallback) and is held to the SAME golden
// files as the in-process dispatcher: the golden API cases and the bookkeeping
// state snapshot. Nothing here writes or adds a golden file.
//
// The two constructors below repeat the server wiring of defaultGoldenServer
// and newSnapStack with one difference, the bus. A parameter on
// newTestDispatcher would remove the copy; those files belong to the harness.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/queue"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// newTestRedisBus starts a consuming RedisBus on a miniredis queue and closes
// it when the test ends. Cleanups run last-in first-out, so it drains before
// the store's own Close cleanup. The fallback is a started Dispatcher, as in
// cmd/funnelbarn.
func newTestRedisBus(t *testing.T, store *repository.Store) *command.RedisBus {
	t.Helper()
	mr := miniredis.RunT(t)
	client, err := queue.NewClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("queue.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	deps := command.Deps{
		Store:              store,
		MarkFlagsEvaluated: service.NewProjectHealthService(store).MarkFlagsEvaluated,
	}
	// The fallback fails the test when used, so the goldens prove the Redis
	// path and not the in-process one.
	fallback := command.New(command.Options{Deps: command.Deps{
		Store: fallbackGuard{t},
		MarkFlagsEvaluated: func(context.Context, string) error {
			t.Errorf("command reached the fallback: mark_flags_evaluated")
			return nil
		},
	}})
	fallback.Start(context.Background())
	bus := command.NewRedisBus(command.RedisOptions{
		Queue:    queue.NewRedisList(client, "bookkeeping", queue.WithPollTimeout(time.Second)),
		Deps:     deps,
		Fallback: fallback,
		Consume:  true,
	})
	bus.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := bus.Close(ctx); err != nil {
			t.Errorf("redis command bus Close: %v", err)
		}
	})
	return bus
}

// fallbackGuard is a command.Store that fails the test on every call.
type fallbackGuard struct{ t *testing.T }

func (g fallbackGuard) fail(kind string) { g.t.Errorf("command reached the fallback: %s", kind) }

func (g fallbackGuard) RecordEvaluation(context.Context, repository.FlagEvaluation) error {
	g.fail("record_evaluation")
	return nil
}

func (g fallbackGuard) TouchAPIKey(context.Context, string) error {
	g.fail("touch_api_key")
	return nil
}

func (g fallbackGuard) FlagByKey(context.Context, string, string) (repository.FeatureFlag, error) {
	g.fail("touch_flag_evaluated")
	return repository.FeatureFlag{}, nil
}

func (g fallbackGuard) TouchFlagEvaluated(context.Context, string) error {
	g.fail("touch_flag_evaluated")
	return nil
}

func (g fallbackGuard) EnsureAutoFlag(_ context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error) {
	g.fail("ensure_auto_flag")
	return f, nil
}

func (g fallbackGuard) CountAutoFlags(context.Context, string) (int, error) {
	g.fail("ensure_auto_flag")
	return 0, nil
}

// redisGoldenServer is defaultGoldenServer with a RedisBus as command bus.
func redisGoldenServer(t *testing.T, deps goldenDeps) goldenTarget {
	t.Helper()
	store := deps.Store
	sp := newTestSpool(t)
	commands := newTestRedisBus(t, store)
	authz := auth.New("").WithDBLookup(store.ValidAPIKeySHA256, dispatcherTouch(commands))
	ingestHandler := ingest.NewHandler(authz, sp, 0)
	healthSvc := service.NewProjectHealthService(store)
	ingestHandler.OnEventsReceived = func(ctx context.Context, projectID string) {
		_, _ = healthSvc.MarkEventsReceived(ctx, projectID)
	}
	sm := auth.NewSessionManager("test-secret", time.Hour)
	userAuth, _ := auth.NewUserAuthenticator("admin", "password", "")

	srv := NewServer(ServerConfig{
		Ingest:              ingestHandler,
		Projects:            service.NewProjectService(store),
		Funnels:             service.NewFunnelService(store),
		ABTests:             service.NewABTestService(store),
		Flags:               service.NewFlagService(store).WithCommands(commands),
		Commands:            commands,
		ReadPoolWait:        func() time.Duration { return store.ReadDB().Stats().WaitDuration },
		Events:              service.NewEventService(store),
		Overview:            service.NewOverviewService(store),
		Sessions:            service.NewSessionService(store),
		APIKeys:             service.NewAPIKeyService(store),
		Widgets:             service.NewWidgetService(store),
		Segments:            service.NewSegmentService(store),
		Recordings:          service.NewRecordingService(store, store, store, deps.Recordings),
		ProjectHealth:       healthSvc,
		InstanceSettings:    store,
		GeoAnonymizer:       store,
		Distributions:       store,
		RecordingSettings:   store,
		IAMBarnUsers:        store,
		UserAuth:            userAuth,
		SessionManager:      sm,
		WebSessions:         store,
		SessionSecret:       "test-secret",
		PublicURL:           "http://localhost",
		LoginRatePerMinute:  100000,
		LoginRateBurst:      100000,
		APIRatePerMinute:    100000,
		APIRateBurst:        100000,
		IngestRatePerMinute: 100000,
		IngestRateBurst:     100000,
		SetupRatePerMinute:  100000,
		SetupRateBurst:      100000,
		FlagAutoRegisterMax: 5,
		DB:                  store,
		Version:             "golden",
	})
	cookie := sessionCookieFor(t, srv, goldenSessionUser)
	spare := sessionCookieFor(t, srv, goldenSessionUser)
	return goldenTarget{
		Handler: srv, Session: cookie, CSRF: srv.sessionManager.CSRFToken(cookie.Value),
		Spare: spare, SpareCSRF: srv.sessionManager.CSRFToken(spare.Value),
		Flush: commands.Flush,
	}
}

// TestRedisBusGoldenAPI runs every golden API case against a server whose
// command bus is a RedisBus, and compares with the golden files of the
// in-process dispatcher.
func TestRedisBusGoldenAPI(t *testing.T) {
	if *update {
		t.Skip("the Redis bus never rewrites golden files")
	}
	runGoldenCases(t, redisGoldenServer)
}

// newRedisSnapStack is newSnapStack with a RedisBus as command bus.
func newRedisSnapStack(t *testing.T) *snapStack {
	t.Helper()
	store, err := repository.Open(filepath.Join(t.TempDir(), "snap.db"))
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	sp := newTestSpool(t)
	var base *auth.Authorizer // nil: only DB-stored keys are accepted
	commands := newTestRedisBus(t, store)
	authz := base.WithDBLookup(store.ValidAPIKeySHA256, dispatcherTouch(commands))
	handler := ingest.NewHandler(authz, sp, 0)
	healthSvc := service.NewProjectHealthService(store)
	handler.OnEventsReceived = func(ctx context.Context, projectID string) {
		if _, err := healthSvc.MarkEventsReceived(ctx, projectID); err != nil {
			t.Errorf("MarkEventsReceived: %v", err)
		}
	}
	userAuth, _ := auth.NewUserAuthenticator("admin", "password", "")
	srv := NewServer(ServerConfig{
		Ingest:              handler,
		Projects:            service.NewProjectService(store),
		Funnels:             service.NewFunnelService(store),
		ABTests:             service.NewABTestService(store),
		Flags:               service.NewFlagService(store).WithCommands(commands),
		Commands:            commands,
		Events:              service.NewEventService(store),
		Overview:            service.NewOverviewService(store),
		Sessions:            service.NewSessionService(store),
		APIKeys:             service.NewAPIKeyService(store),
		Widgets:             service.NewWidgetService(store),
		UserAuth:            userAuth,
		SessionManager:      auth.NewSessionManager("test-secret", time.Hour),
		WebSessions:         store,
		SessionSecret:       "test-secret",
		PublicURL:           "http://localhost",
		LoginRatePerMinute:  1000,
		LoginRateBurst:      1000,
		APIRatePerMinute:    1000,
		APIRateBurst:        1000,
		IngestRatePerMinute: 1000,
		IngestRateBurst:     1000,
		DB:                  store,
		Version:             "test",
		ProjectHealth:       healthSvc,
		FlagAutoRegisterMax: snapAutoMax,
	})
	st := &snapStack{srv: srv, store: store, sp: sp, handler: handler, Flush: commands.Flush}
	snapStacks.Store(store, st)
	t.Cleanup(func() { snapStacks.Delete(store) })
	return st
}

// TestRedisBusStateSnapshot replays the bookkeeping state snapshot scenario
// with a RedisBus and compares with the same golden files.
func TestRedisBusStateSnapshot(t *testing.T) {
	if *updateState {
		t.Skip("the Redis bus never rewrites golden files")
	}
	s := newRedisSnapStack(t)
	p := s.seed(t)

	s.ingestAndPersist(t)
	responses := s.evaluateAll(t, p)

	touched := []string{"exp_split", "exp_target", "cfg_flag", "off_flag", "auto_exp", "auto_cfg", "auto_third"}
	flushAsync(t, s.store, p.ID, touched)

	compareGolden(t, "responses", responses)
	snapshotAll(t, s.store)

	var used int
	if err := s.store.DB().QueryRow(`SELECT COUNT(*) FROM api_keys WHERE last_used_at IS NOT NULL`).Scan(&used); err != nil || used != 1 {
		t.Errorf("api keys with last_used_at: want 1, got %d (err=%v)", used, err)
	}
}
