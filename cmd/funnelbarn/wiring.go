package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	selfsdk "github.com/webwiebe/funnelbarn/sdks/go"
	bb "github.com/wiebe-xyz/bugbarn-go"

	"github.com/wiebe-xyz/funnelbarn/internal/api"
	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/geoip"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
	"github.com/wiebe-xyz/funnelbarn/internal/workerhealth"
)

// cleanupStack runs shutdown steps in the reverse order they were added, the
// way deferred calls do. run() hands it to every wiring stage so a stage can
// register what it opened without owning the process lifetime.
type cleanupStack struct{ steps []func() }

func (c *cleanupStack) push(step func()) { c.steps = append(c.steps, step) }

func (c *cleanupStack) run() {
	for i := len(c.steps) - 1; i >= 0; i-- {
		c.steps[i]()
	}
	c.steps = nil
}

// runtimeVersion is the version to report. The deploy injects the actual
// release tag via FUNNELBARN_VERSION (images are SHA-tagged and reused across
// environments), which overrides the build-time default baked into the binary.
func runtimeVersion(cfg config.Config) string {
	if cfg.Version != "" {
		return cfg.Version
	}
	return Version
}

// checkSessionSecret accepts a strong secret. An empty secret is tolerated for
// local dev (a random per-process secret is generated) but a weak explicit one
// is refused: a short secret is worse than none because it looks deliberate.
// All deployed environments set this via SOPS.
func checkSessionSecret(secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		slog.Warn("FUNNELBARN_SESSION_SECRET is not set; a random per-process secret will be used and sessions will not persist across restarts",
			"handled", false)
		return nil
	}
	if len(secret) < 32 {
		return fmt.Errorf("FUNNELBARN_SESSION_SECRET must be at least 32 characters (got %d)", len(secret))
	}
	return nil
}

// telemetry is what the rest of the wiring needs from the telemetry stage.
type telemetry struct {
	selfReporting bool
	spanRelay     *tracing.SpanRelay
}

// startTelemetry wires BugBarn self-reporting, the multi-sink logger, dogfood
// analytics, the tracer and the OTel MeterProvider.
//
// It runs before repository.Open: otelsql resolves otel.GetMeterProvider()
// exactly once, at Open time, and binds to whatever provider is current then.
// If InitMetrics ran later, otelsql would permanently bind to the no-op default
// provider and its db.sql.* histograms would never export.
func startTelemetry(ctx context.Context, cfg config.Config, version string, cleanup *cleanupStack) (telemetry, error) {
	selfReporting := cfg.SelfEndpoint != "" && cfg.SelfAPIKey != ""
	if selfReporting {
		bb.Init(bb.Options{
			APIKey:      cfg.SelfAPIKey,
			Endpoint:    cfg.SelfEndpoint,
			ProjectSlug: cfg.SelfProject,
			Environment: cfg.SelfEnvironment,
		})
		cleanup.push(func() { bb.Shutdown(2 * time.Second) })
	}
	otelCfg := tracing.Config{
		Endpoint:    cfg.SpanBarnEndpoint,
		APIKey:      cfg.SpanBarnAPIKey,
		ServiceName: "funnelbarn",
		Version:     version,
		Environment: cfg.SelfEnvironment,
	}

	// SpanBarn OTLP logs handler — must be created before the logger is built.
	spanbarnLogHandler, shutdownLogs, err := tracing.InitLogs(ctx, otelCfg)
	if err != nil {
		return telemetry{}, fmt.Errorf("init logs: %w", err)
	}
	cleanup.push(func() {
		logShutdown(slog.LevelWarn, "logs", func() error { return shutdownLogs(context.Background()) })
	})

	// Rewire the global logger with the appropriate sinks.
	slog.SetDefault(buildLogger(os.Stderr, cfg, spanbarnLogHandler))
	logSelfReporting(cfg, selfReporting)
	startDogfood(cfg, cleanup)

	shutdownTracer, err := tracing.Init(ctx, otelCfg)
	if err != nil {
		return telemetry{}, fmt.Errorf("init tracing: %w", err)
	}
	cleanup.push(func() {
		logShutdown(slog.LevelWarn, "tracer", func() error { return shutdownTracer(context.Background()) })
	})

	// Must run before repository.Open: this is what sets the real OTel
	// MeterProvider via otel.SetMeterProvider.
	shutdownMetrics, err := tracing.InitMetrics(ctx, otelCfg)
	if err != nil {
		return telemetry{}, fmt.Errorf("init metrics: %w", err)
	}
	cleanup.push(func() {
		logShutdown(slog.LevelWarn, "metrics", func() error { return shutdownMetrics(context.Background()) })
	})

	// Relays the dashboard's own browser spans to SpanBarn, so a trace shows the
	// page and the client-side view of each API call above the server spans that
	// already join it via the traceparent header.
	spanRelay := tracing.NewSpanRelay(otelCfg)
	cleanup.push(spanRelay.Shutdown)

	if cfg.SpanBarnEndpoint != "" {
		slog.Info("spanbarn telemetry enabled", "endpoint", cfg.SpanBarnEndpoint, "signals", "traces,metrics,logs,browser-spans")
	}
	return telemetry{selfReporting: selfReporting, spanRelay: spanRelay}, nil
}

// logSelfReporting says whether errors reach BugBarn, and whether dogfood
// analytics is on.
func logSelfReporting(cfg config.Config, selfReporting bool) {
	if selfReporting {
		slog.Info("self-reporting enabled", "endpoint", cfg.SelfEndpoint)
	} else {
		slog.Warn("self-reporting disabled; errors will not be reported to BugBarn — set FUNNELBARN_SELF_ENDPOINT and FUNNELBARN_SELF_API_KEY to enable it",
			"handled", true)
	}
	if cfg.DogfoodAPIKey != "" {
		slog.Info("dogfood analytics enabled", "project", cfg.DogfoodProject)
	}
}

// startDogfood starts backend self-tracking: events (e.g. first_event_ingested)
// that can only be observed server-side, sent to the same dogfood project used
// for frontend analytics and flag evaluation, via this repo's own Go SDK hitting
// this app's own public ingest endpoint.
func startDogfood(cfg config.Config, cleanup *cleanupStack) {
	if cfg.DogfoodAPIKey == "" {
		return
	}
	if cfg.PublicURL == "" {
		slog.Warn("dogfood analytics enabled but FUNNELBARN_PUBLIC_URL is unset; backend self-tracking disabled",
			"handled", true)
		return
	}
	selfsdk.Init(selfsdk.Options{
		APIKey:      cfg.DogfoodAPIKey,
		Endpoint:    strings.TrimRight(cfg.PublicURL, "/"),
		ProjectName: cfg.DogfoodProject,
		// Without this the SDK falls back to its own log.Printf, which
		// bypasses slog and so never reaches BugBarn. A dogfood key minted
		// for the wrong project rejects every event permanently; that has
		// to look like a problem, not like silence.
		OnError: func(e selfsdk.Event, err error) {
			slog.Warn("dogfood analytics event was rejected",
				"event", e.Name, "err", err, "handled", true)
		},
	})
	cleanup.push(func() {
		logShutdown(slog.LevelWarn, "self-reporting", func() error { return selfsdk.Shutdown(2 * time.Second) })
	})
}

// dataLayer is the storage and the command path in front of it.
type dataLayer struct {
	store           *repository.Store
	health          service.ProjectHealth
	commands        command.Bus
	flagBookkeeping *command.Async
}

// openData opens the store and starts the command bus and the flag
// bookkeeping queue. The queues drain before the store closes.
func openData(ctx context.Context, cfg config.Config, cleanup *cleanupStack) (dataLayer, error) {
	slog.Info("process mode", "mode", cfg.Mode)
	store, err := openStore(cfg)
	if err != nil {
		return dataLayer{}, fmt.Errorf("open storage: %w", err)
	}
	cleanup.push(func() { logShutdown(slog.LevelError, "store", store.Close) })

	healthSvc := service.NewProjectHealthService(store)
	commands, err := newCommandBus(ctx, cfg, command.Deps{
		Store:              store,
		MarkFlagsEvaluated: healthSvc.MarkFlagsEvaluated,
		MarkProjectHealth:  service.ApplyHealthMark(healthSvc),
	}, slog.Default())
	if err != nil {
		return dataLayer{}, fmt.Errorf("command bus: %w", err)
	}
	cleanup.push(func() { drainCommands(commands) }) // runs before store.Close
	health := service.ProjectHealth(healthSvc)
	if isReader(cfg) {
		health = service.NewProjectHealthService(service.HealthViaCommands{ProjectHealthRepo: store, Bus: commands})
	}
	flagBookkeeping := newFlagBookkeeping(ctx, commands, slog.Default())
	cleanup.push(func() { drainCommands(flagBookkeeping) }) // hands its queue to commands before they drain
	return dataLayer{store: store, health: health, commands: commands, flagBookkeeping: flagBookkeeping}, nil
}

// newServerConfig builds the API server's configuration from the services and
// the settings. The ingest handler, the recordings, the authentication objects
// and the readiness check are filled in by later stages. It also returns the
// API key snapshot the authorizer reads.
func newServerConfig(ctx context.Context, cfg config.Config, version string, d dataLayer) (api.ServerConfig, *auth.KeySnapshot) {
	store := d.store
	flagSnapshot, keySnapshot := startEvaluateSnapshots(ctx, store)
	flagsSvc := service.NewFlagService(store).
		WithConfigCacheTTL(time.Duration(cfg.ConfigFlagCacheSeconds) * time.Second).
		WithCommands(d.commands).
		WithBackgroundCommands(d.flagBookkeeping).
		WithSnapshot(flagSnapshot)

	return api.ServerConfig{
		InstanceSettings:      store,
		GeoAnonymizer:         store,
		Segments:              service.NewSegmentService(store),
		Distributions:         store,
		Projects:              service.NewProjectService(store),
		Funnels:               service.NewFunnelService(store),
		ABTests:               service.NewABTestService(store),
		Flags:                 flagsSvc,
		Events:                service.NewEventService(store),
		Overview:              service.NewOverviewService(store),
		Sessions:              service.NewSessionService(store),
		APIKeys:               service.NewAPIKeyService(store),
		Widgets:               service.NewWidgetService(store),
		AllowedOrigins:        cfg.AllowedOrigins,
		PublicURL:             cfg.PublicURL,
		LoginRatePerMinute:    cfg.LoginRatePerMinute,
		LoginRateBurst:        cfg.LoginRateBurst,
		APIRatePerMinute:      cfg.APIRatePerMinute,
		APIRateBurst:          cfg.APIRateBurst,
		IngestRatePerMinute:   cfg.IngestRatePerMinute,
		IngestRateBurst:       cfg.IngestRateBurst,
		SetupRatePerMinute:    cfg.SetupRatePerMinute,
		SetupRateBurst:        cfg.SetupRateBurst,
		DB:                    store,
		Version:               version,
		Commit:                Commit,
		BuildTime:             BuildTime,
		TrustedProxies:        cfg.TrustedProxies,
		BugbarnEndpoint:       cfg.SelfEndpoint,
		BugbarnIngestKey:      cfg.SelfAPIKey,
		BugbarnProject:        cfg.SelfProject,
		DogfoodAPIKey:         cfg.DogfoodAPIKey,
		DogfoodProject:        cfg.DogfoodProject,
		IAMBarnUsers:          store,
		PostLogoutRedirectURI: cfg.PostLogoutRedirectURI,
		WebSessions:           store,
		Environment:           cfg.SelfEnvironment,
		OIDCRefreshGrace:      time.Duration(cfg.OIDCRefreshGraceSeconds) * time.Second,
		RecordingSettings:     store,
		ProjectHealth:         d.health,
		Agents:                service.NewAgentService(store),
		FlagAutoRegisterMax:   cfg.AutoRegisterMaxFlags,
		Commands:              d.commands,
		ReadPoolWait:          func() time.Duration { return store.ReadDB().Stats().WaitDuration },
		WriterURL:             forwardTarget(cfg),
		InternalToken:         cfg.InternalToken,
		MCPResourceURL:        cfg.MCPResourceURL,
	}, keySnapshot
}

// ingestLayer is what the ingest stage hands to the API.
type ingestLayer struct {
	spool      *spool.Spool
	recordings service.Recordings
	ready      func(context.Context) error
}

// startIngest opens the spool, the recordings service, the GeoIP lookup and
// the ingest queue, and starts the worker. Each registers its shutdown step.
func startIngest(ctx context.Context, cfg config.Config, d dataLayer, cleanup *cleanupStack) (ingestLayer, error) {
	eventSpool, err := spool.NewWithLimit(cfg.SpoolDir, cfg.MaxSpoolBytes)
	if err != nil {
		return ingestLayer{}, fmt.Errorf("open spool: %w", err)
	}
	cleanup.push(func() { logShutdown(slog.LevelError, "spool", eventSpool.Close) })

	recordingsSvc, recordingsQ, err := newRecordings(ctx, cfg, d.store, slog.Default())
	if err != nil {
		return ingestLayer{}, fmt.Errorf("recordings queue: %w", err)
	}
	cleanup.push(func() { drainQueue("recordings queue", recordingsQ) }) // runs before store.Close

	geoLookup := openGeo(cfg)
	if geoLookup != nil {
		cleanup.push(geoLookup.Close)
	}

	applier := &ingestApplier{store: d.store, geo: geoLookup, health: workerhealth.New(workerhealth.Options{})}
	ingestQ, err := newIngestQueue(ctx, cfg, applier, slog.Default())
	if err != nil {
		return ingestLayer{}, fmt.Errorf("ingest queue: %w", err)
	}
	cleanup.push(func() { drainQueue("ingest queue", ingestQ) }) // runs before store.Close

	ready, err := startWorker(ctx, cfg, d.store, eventSpool, applier, ingestQ, recordingsQ, recordingsSvc, d.commands)
	if err != nil {
		return ingestLayer{}, err
	}
	return ingestLayer{spool: eventSpool, recordings: recordingsSvc, ready: ready}, nil
}

// openGeo opens the GeoIP databases. It returns nil when none is configured or
// when opening fails: geo enrichment is then off.
func openGeo(cfg config.Config) *geoip.Lookup {
	if cfg.GeoIPCityDB == "" {
		return nil
	}
	lookup, err := geoip.Open(cfg.GeoIPCityDB, cfg.GeoIPASNDB)
	if err != nil {
		// Error (not Warn) so a missing/unreadable geo DB raises a BugBarn
		// issue instead of silently disabling geo enrichment.
		slog.Error("geoip: failed to open database, geo enrichment disabled",
			"err", err, "handled", false,
			"city_db", cfg.GeoIPCityDB, "asn_db", cfg.GeoIPASNDB)
		return nil
	}
	slog.Info("geoip enabled", "city_db", cfg.GeoIPCityDB, "asn_db", cfg.GeoIPASNDB)
	return lookup
}

// authStack is the API authorizer and the dashboard login objects.
type authStack struct {
	authorizer *auth.Authorizer
	users      *auth.UserAuthenticator
	sessions   *auth.SessionManager
}

// newAuthStack builds the API key authorizer, the env-admin authenticator and
// the session manager, and prunes expired web sessions.
func newAuthStack(ctx context.Context, cfg config.Config, d dataLayer, keys *auth.KeySnapshot) (authStack, error) {
	apiAuthorizer, err := newAPIAuthorizer(cfg, d.store, d.commands)
	if err != nil {
		return authStack{}, err
	}
	apiAuthorizer = apiAuthorizer.WithEvaluateKeys(keys)
	userAuth, err := auth.NewUserAuthenticator(cfg.AdminUsername, cfg.AdminPassword, cfg.AdminPasswordBcrypt)
	if err != nil {
		return authStack{}, err
	}
	// Sessions are token-bound server-side rows (web_sessions): the cookie is
	// an opaque handle, so a logout/revocation is simply a row deletion —
	// durable by construction, no separate revocation list needed.
	sessionManager := auth.NewSessionManager(cfg.SessionSecret, cfg.SessionTTL)
	pruneExpiredWebSessions(ctx, cfg, d.store)
	return authStack{authorizer: apiAuthorizer, users: userAuth, sessions: sessionManager}, nil
}

// pruneExpiredWebSessions deletes sessions past their absolute cap. A reader
// cannot write; the writer prunes.
func pruneExpiredWebSessions(ctx context.Context, cfg config.Config, store *repository.Store) {
	if isReader(cfg) {
		return
	}
	n, err := store.DeleteExpiredWebSessions(ctx, time.Now().UTC())
	if err != nil {
		slog.Warn("prune expired web sessions", "err", err)
	} else if n > 0 {
		slog.Info("pruned expired web sessions", "count", n)
	}
}

// newIngestHandler builds the ingest handler and starts its background flush.
// The flush stops before the spool closes, so its final write reaches the file.
func newIngestHandler(ctx context.Context, cfg config.Config, authorizer *auth.Authorizer, eventSpool *spool.Spool, health service.ProjectHealth, cleanup *cleanupStack) *ingest.Handler {
	handler := ingest.NewHandler(authorizer, eventSpool, cfg.MaxBodyBytes)
	handler.OnEventsReceived = func(ctx context.Context, projectID string) {
		firstEvent, err := health.MarkEventsReceived(ctx, projectID)
		if err != nil {
			slog.Warn("ingest: mark events received", "project_id", projectID, "err", err)
			return
		}
		// firstEvent is true only on the call that flips this project's
		// events_received health flag from false to true — i.e. this
		// project's first ever ingested event. Reusing that existing
		// per-project flag (rather than a new migration/counter) gives us an
		// activation signal that can only be observed here, server-side: it
		// happens via the customer's own SDK hitting this app's ingest
		// endpoint, never through this app's own UI.
		if firstEvent && projectID != "" {
			selfsdk.Track("first_event_ingested", map[string]any{
				"project_id": projectID,
			})
		}
	}
	cleanup.push(handler.StartBackground(ctx))
	return handler
}

// checkAuthentication counts the local users, logs which sign-in mechanisms are
// on, and refuses a production start with no way to authenticate. It returns
// the local user count.
//
// A deployment that authenticates only via CLI-created users (no admin env, no
// OIDC) must still enforce sessions on its dashboard routes, so the API fails
// closed on this count.
func checkAuthentication(ctx context.Context, cfg config.Config, store *repository.Store, a authStack, oidcClient *auth.OIDCClient) (int, error) {
	localUserCount, err := store.CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	slog.Info("authentication mechanisms",
		"env_admin", a.users.Enabled(),
		"oidc_confidential", oidcClient != nil,
		"local_users", localUserCount,
		"mcp", oidcClient != nil && cfg.MCPResourceURL != "",
		"mcp_resource_url", cfg.MCPResourceURL,
	)
	authConfigured := a.users.Enabled() || oidcClient != nil || localUserCount > 0
	if err := validateFailClosed(environment.Normalize(cfg.SelfEnvironment), a.authorizer.Enabled(), authConfigured); err != nil {
		return 0, err
	}
	if !authConfigured {
		slog.Warn("no authentication mechanism configured; dashboard API routes will be served UNAUTHENTICATED — set FUNNELBARN_ADMIN_*, configure OIDC, or run 'funnelbarn user create'",
			"handled", false)
	}
	return localUserCount, nil
}

// serve starts the API server's cleanup loop and the HTTP listener, and blocks
// until the context ends or the listener fails.
func serve(ctx context.Context, cfg config.Config, version string, apiServer *api.Server, selfReporting bool) error {
	apiServer.SetGitHubWebhook(cfg.GitHubWebhookSecret, cfg.GitHubWebhookProject)
	if cfg.MetricsToken != "" {
		apiServer.SetMetricsToken(cfg.MetricsToken)
	} else {
		slog.Warn("/metrics is served without authentication (FUNNELBARN_METRICS_TOKEN unset); ensure it is not exposed on a public route",
			"handled", false)
	}

	apiServer.StartCleanup(ctx)

	var httpHandler http.Handler = apiServer
	if selfReporting {
		httpHandler = bb.RecoverMiddleware(httpHandler)
	}
	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: httpHandler,
	}

	slog.Info("funnelbarn starting", "addr", cfg.Addr, "version", version)

	errCh := make(chan error, 1)
	bblog.Go("http-server", func() {
		errCh <- server.ListenAndServe()
	})
	return waitServer(ctx, server, errCh)
}
