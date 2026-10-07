package api

// Golden API harness (issue #302, regression harness).
//
// The golden suite records today's HTTP behaviour (status, selected headers,
// canonical JSON body) for every route: the reads, the dashboard mutations
// (each followed by a read-back that pins the persisted effect), the SDK
// evaluate endpoint and ingest, against a deterministic seeded dataset. Later
// PRs in the CQRS split must leave every golden file untouched; a diff means
// behaviour changed.
//
// Phase 2 reuses the same seed and the same cases against a different server
// setup (for example a read-only pool or a reader/writer split) by passing its
// own goldenConstructor to runGoldenCases. Nothing here depends on how the
// Server is built; the constructor receives the seeded store and returns the
// handler plus the credentials to call it with.
//
// Refresh the files with:  go test ./internal/api -run TestGolden -update

import (
	"context"
	"flag"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// update rewrites the golden files instead of comparing against them.
var update = flag.Bool("update", false, "rewrite golden files in internal/api/testdata/golden")

// goldenDeps is what the seed hands to a server constructor.
type goldenDeps struct {
	Store *repository.Store
	// Path is the SQLite file the store was opened on. A later constructor
	// (read-only pool, reader/writer split) opens its own connections on it.
	Path string
	// Recordings is the chunk blob store the seed wrote its recordings into.
	// A constructor must give the same storage to its RecordingService.
	Recordings service.RecordingStorage
	Seed       *goldenSeed
}

// goldenTarget is a running server under test plus the credentials the cases
// authenticate with.
type goldenTarget struct {
	Handler http.Handler
	// Session is the dashboard session cookie; CSRF the matching token for
	// mutating dashboard requests.
	Session *http.Cookie
	CSRF    string
	// Spare is a second session of the same user that no other case relies on.
	// The logout case revokes it and a read-back proves the revocation.
	Spare     *http.Cookie
	SpareCSRF string
	// Flush, when set, applies every queued async command before returning.
	// The harness calls it at each await point, then still polls the database
	// for the condition. The default server leaves it nil (its bookkeeping is
	// plain goroutines); a server that queues commands behind a dispatcher sets
	// it to the dispatcher's Flush.
	Flush func(context.Context) error
}

// goldenConstructor builds the server under test over a seeded store.
type goldenConstructor func(t *testing.T, deps goldenDeps) goldenTarget

// goldenSessionUser is the username of the minted dashboard session.
const goldenSessionUser = "golden-admin"

// defaultGoldenServer wires the server the way cmd/funnelbarn does: real
// services over the real repository.Store, DB-backed API key authorizer, env
// admin so dashboard routes require a real session cookie.
func defaultGoldenServer(t *testing.T, deps goldenDeps) goldenTarget {
	t.Helper()
	store := deps.Store
	sp := newTestSpool(t)
	commands := newTestDispatcher(t, store)
	authz := auth.New("").WithDBLookup(store.ValidAPIKeySHA256, dispatcherTouch(commands, store))
	ingestHandler := ingest.NewHandler(authz, sp, 0)
	healthSvc := service.NewProjectHealthService(store)
	// Same wiring as cmd/funnelbarn: an accepted ingest request flips the
	// project's events_received health flag, off the request goroutine.
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

// newTestDispatcher starts a command dispatcher on store the way
// cmd/funnelbarn does and closes it when the test ends. Cleanups run last-in
// first-out, so it drains before the store's own Close cleanup.
func newTestDispatcher(t *testing.T, store *repository.Store) *command.Dispatcher {
	t.Helper()
	d := command.New(command.Options{})
	d.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := d.Close(ctx); err != nil {
			t.Errorf("command dispatcher Close: %v", err)
		}
	})
	return d
}

// dispatcherTouch is the DBKeyTouch cmd/funnelbarn builds: a queued
// TouchAPIKey command.
func dispatcherTouch(d *command.Dispatcher, store *repository.Store) auth.DBKeyTouch {
	return func(ctx context.Context, keySHA256 string) error {
		service.AddSubmitWait(ctx, d.Submit(ctx, command.TouchAPIKey{Store: store, KeyHash: keySHA256}))
		return nil
	}
}

// newGoldenEnv opens a fresh temp-file store, seeds it, and builds the server
// under test with ctor. Every call yields an identical dataset. The file lives
// in t.TempDir so a constructor can open further connections on deps.Path.
func newGoldenEnv(t *testing.T, ctor goldenConstructor) (goldenTarget, *goldenSeed, goldenDeps) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "golden.db")
	store, err := repository.Open(path)
	if err != nil {
		t.Fatalf("repository.Open(%s): %v", path, err)
	}
	t.Cleanup(func() { store.Close() })
	storage := newMemStorage()
	seed := seedGolden(t, store, storage)
	deps := goldenDeps{Store: store, Path: path, Recordings: storage, Seed: seed}
	return ctor(t, deps), seed, deps
}
