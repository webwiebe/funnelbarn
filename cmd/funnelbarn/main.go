package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"

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
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
	"github.com/wiebe-xyz/funnelbarn/internal/workerhealth"
)

// Version and BuildTime are injected at build time via -ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			// Use fmt.Fprintf to stderr since logger may not be available.
			fmt.Fprintf(os.Stderr, `{"level":"ERROR","msg":"unhandled panic","panic":%q,"time":%q}`+"\n",
				fmt.Sprint(r), time.Now().UTC().Format(time.RFC3339))

			// If BugBarn is configured, report the crash.
			bblog.ReportPanic(os.Getenv("FUNNELBARN_SELF_ENDPOINT"), os.Getenv("FUNNELBARN_SELF_API_KEY"), r)

			os.Exit(2)
		}
	}()

	// Use structured JSON logging to stderr by default.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(); err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
}

// buildLogger constructs a multi-sink slog.Logger based on the config.
// It always writes JSON to out, optionally captures Error+ in BugBarn, and
// (when spanbarn != nil) ships >= SpanBarnLogLevel records to SpanBarn via OTLP,
// minus the high-volume health-probe access logs.
func buildLogger(out io.Writer, cfg config.Config, spanbarn slog.Handler) *slog.Logger {
	var handlers []slog.Handler

	// Always: structured JSON to out. With BugBarn configured, the BugBarn
	// handler wraps it and passes every record on, so it replaces the plain
	// handler; listing both wrote every line twice.
	var stdout slog.Handler = slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})
	if cfg.SelfEndpoint != "" && cfg.SelfAPIKey != "" {
		stdout = bblog.NewHandler(stdout)
	}
	handlers = append(handlers, stdout)

	// Optional: SpanBarn OTLP logs (trace-correlated), filtered to keep volume
	// sane on indefinitely-retained log storage.
	if spanbarn != nil {
		handlers = append(handlers, bblog.NewFilterHandler(spanbarn, cfg.SpanBarnLogLevel, isHealthProbeLog))
	}

	return slog.New(bblog.NewMultiHandler(handlers...))
}

// isHealthProbeLog drops the per-request access log emitted for Kubernetes health
// probes, which fire every few seconds and would otherwise flood SpanBarn.
func isHealthProbeLog(r slog.Record) bool {
	if r.Message != "request" {
		return false
	}
	probe := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "user_agent" && strings.HasPrefix(a.Value.String(), "kube-probe") {
			probe = true
			return false
		}
		return true
	})
	return probe
}

// run owns process wiring: opens storage, starts the worker, and serves the API.
func run() error {
	cfg := config.Load()

	// Runtime version: the deploy injects the actual release tag via
	// FUNNELBARN_VERSION (images are SHA-tagged and reused across environments),
	// which overrides the build-time default baked into the binary.
	version := Version
	if cfg.Version != "" {
		version = cfg.Version
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("funnelbarn %s (built %s)\n", Version, BuildTime)
			return nil
		case "worker-once":
			return runWorkerOnce(cfg)
		case "replay-dead-letter":
			return runReplayDeadLetter(cfg, os.Args[2:])
		case "user":
			return runUserCmd(cfg, os.Args[2:])
		case "project":
			return runProjectCmd(cfg, os.Args[2:])
		case "apikey":
			return runAPIKeyCmd(cfg, os.Args[2:])
		}
	}

	// A configured session secret must be strong. An empty secret is tolerated
	// for local dev (a random per-process secret is generated) but refused for a
	// weak explicit one — a short secret is worse than none because it looks
	// deliberate. All deployed environments set this via SOPS.
	if secret := strings.TrimSpace(cfg.SessionSecret); secret == "" {
		slog.Warn("FUNNELBARN_SESSION_SECRET is not set; a random per-process secret will be used and sessions will not persist across restarts",
			"handled", false)
	} else if len(secret) < 32 {
		return fmt.Errorf("FUNNELBARN_SESSION_SECRET must be at least 32 characters (got %d)", len(secret))
	}

	// Telemetry (logging sinks, tracer, and OTel MeterProvider) is wired up
	// front, before repository.Open: otelsql resolves otel.GetMeterProvider()
	// exactly once, at Open time, and binds to whatever provider is current
	// then. If InitMetrics ran later, otelsql would permanently bind to the
	// no-op default provider and its db.sql.* histograms would never export.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wire BugBarn self-reporting and set up multi-sink logger.
	selfReporting := cfg.SelfEndpoint != "" && cfg.SelfAPIKey != ""
	if selfReporting {
		bb.Init(bb.Options{
			APIKey:      cfg.SelfAPIKey,
			Endpoint:    cfg.SelfEndpoint,
			ProjectSlug: cfg.SelfProject,
			Environment: cfg.SelfEnvironment,
		})
		defer bb.Shutdown(2 * time.Second)
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
		return fmt.Errorf("init logs: %w", err)
	}
	defer shutdownLogs(context.Background())

	// Rewire the global logger with the appropriate sinks.
	slog.SetDefault(buildLogger(os.Stderr, cfg, spanbarnLogHandler))
	if selfReporting {
		slog.Info("self-reporting enabled", "endpoint", cfg.SelfEndpoint)
	} else {
		slog.Warn("self-reporting disabled; errors will not be reported to BugBarn — set FUNNELBARN_SELF_ENDPOINT and FUNNELBARN_SELF_API_KEY to enable it",
			"handled", true)
	}
	if cfg.DogfoodAPIKey != "" {
		slog.Info("dogfood analytics enabled", "project", cfg.DogfoodProject)
	}

	// Backend self-tracking: fire events (e.g. first_event_ingested) that can
	// only be observed server-side, against the same dogfood project used for
	// frontend analytics and flag evaluation — via this repo's own Go SDK
	// hitting this app's own public ingest endpoint.
	if cfg.DogfoodAPIKey != "" && cfg.PublicURL != "" {
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
		defer selfsdk.Shutdown(2 * time.Second)
	} else if cfg.DogfoodAPIKey != "" {
		slog.Warn("dogfood analytics enabled but FUNNELBARN_PUBLIC_URL is unset; backend self-tracking disabled",
			"handled", true)
	}

	shutdownTracer, err := tracing.Init(ctx, otelCfg)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer shutdownTracer(context.Background())

	// Must run before repository.Open (see comment above): this is what sets
	// the real OTel MeterProvider via otel.SetMeterProvider.
	shutdownMetrics, err := tracing.InitMetrics(ctx, otelCfg)
	if err != nil {
		return fmt.Errorf("init metrics: %w", err)
	}
	defer shutdownMetrics(context.Background())

	// Relays the dashboard's own browser spans to SpanBarn, so a trace shows the
	// page and the client-side view of each API call above the server spans that
	// already join it via the traceparent header.
	spanRelay := tracing.NewSpanRelay(otelCfg)
	defer spanRelay.Shutdown()

	if cfg.SpanBarnEndpoint != "" {
		slog.Info("spanbarn telemetry enabled", "endpoint", cfg.SpanBarnEndpoint, "signals", "traces,metrics,logs,browser-spans")
	}

	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer store.Close()

	healthSvc := service.NewProjectHealthService(store)
	commands, err := newCommandBus(ctx, cfg, command.Deps{Store: store, MarkFlagsEvaluated: healthSvc.MarkFlagsEvaluated}, slog.Default())
	if err != nil {
		return fmt.Errorf("command bus: %w", err)
	}
	defer drainCommands(commands) // deferred LIFO: drains before store.Close

	// Wire services.
	projectsSvc := service.NewProjectService(store)
	funnelsSvc := service.NewFunnelService(store)
	abtestsSvc := service.NewABTestService(store)
	flagsSvc := service.NewFlagService(store).WithConfigCacheTTL(time.Duration(cfg.ConfigFlagCacheSeconds) * time.Second).WithCommands(commands)
	eventsSvc := service.NewEventService(store)
	overviewSvc := service.NewOverviewService(store)
	sessionsSvc := service.NewSessionService(store)
	apikeysSvc := service.NewAPIKeyService(store)
	widgetsSvc := service.NewWidgetService(store)
	segmentsSvc := service.NewSegmentService(store)

	eventSpool, err := spool.NewWithLimit(cfg.SpoolDir, cfg.MaxSpoolBytes)
	if err != nil {
		return fmt.Errorf("open spool: %w", err)
	}
	defer eventSpool.Close()

	recordingsSvc, recordingsQ, err := newRecordings(ctx, cfg, store, slog.Default())
	if err != nil {
		return fmt.Errorf("recordings queue: %w", err)
	}
	defer drainQueue("recordings queue", recordingsQ) // deferred LIFO: drains before store.Close

	var geoLookup *geoip.Lookup
	if cfg.GeoIPCityDB != "" {
		var geoErr error
		geoLookup, geoErr = geoip.Open(cfg.GeoIPCityDB, cfg.GeoIPASNDB)
		if geoErr != nil {
			// Error (not Warn) so a missing/unreadable geo DB raises a BugBarn
			// issue instead of silently disabling geo enrichment.
			slog.Error("geoip: failed to open database, geo enrichment disabled",
				"err", geoErr, "handled", false,
				"city_db", cfg.GeoIPCityDB, "asn_db", cfg.GeoIPASNDB)
		} else {
			defer geoLookup.Close()
			slog.Info("geoip enabled", "city_db", cfg.GeoIPCityDB, "asn_db", cfg.GeoIPASNDB)
		}
	}

	applier := &ingestApplier{store: store, geo: geoLookup, health: workerhealth.New(workerhealth.Options{})}
	ingestQ, err := newIngestQueue(ctx, cfg, applier, slog.Default())
	if err != nil {
		return fmt.Errorf("ingest queue: %w", err)
	}
	defer drainQueue("ingest queue", ingestQ) // deferred LIFO: drains before store.Close

	bblog.Go("background-worker", func() {
		runBackgroundWorker(ctx, cfg, store, eventSpool, applier, ingestQ, recordingsSvc)
	})

	apiAuthorizer, err := newAPIAuthorizer(cfg, store, commands)
	if err != nil {
		return err
	}
	userAuth, err := auth.NewUserAuthenticator(cfg.AdminUsername, cfg.AdminPassword, cfg.AdminPasswordBcrypt)
	if err != nil {
		return err
	}
	// Sessions are token-bound server-side rows (web_sessions): the cookie is
	// an opaque handle, so a logout/revocation is simply a row deletion —
	// durable by construction, no separate revocation list needed.
	sessionManager := auth.NewSessionManager(cfg.SessionSecret, cfg.SessionTTL)
	if n, err := store.DeleteExpiredWebSessions(ctx, time.Now().UTC()); err != nil {
		slog.Warn("prune expired web sessions", "err", err)
	} else if n > 0 {
		slog.Info("pruned expired web sessions", "count", n)
	}
	handler := ingest.NewHandler(apiAuthorizer, eventSpool, cfg.MaxBodyBytes)
	handler.OnEventsReceived = func(ctx context.Context, projectID string) {
		firstEvent, err := healthSvc.MarkEventsReceived(ctx, projectID)
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
	go handler.Start(ctx)

	oidcClient := buildOIDCClient(cfg)

	// Determine whether local/DB users exist so the API can fail closed. A
	// deployment that authenticates only via CLI-created users (no admin env,
	// no OIDC) must still enforce sessions on its dashboard routes.
	localUserCount, err := store.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	slog.Info("authentication mechanisms",
		"env_admin", userAuth.Enabled(),
		"oidc_confidential", oidcClient != nil,
		"local_users", localUserCount,
		"mcp", oidcClient != nil && cfg.MCPResourceURL != "",
		"mcp_resource_url", cfg.MCPResourceURL,
	)
	authConfigured := userAuth.Enabled() || oidcClient != nil || localUserCount > 0
	if err := validateFailClosed(environment.Normalize(cfg.SelfEnvironment), apiAuthorizer.Enabled(), authConfigured); err != nil {
		return err
	}
	if !authConfigured {
		slog.Warn("no authentication mechanism configured; dashboard API routes will be served UNAUTHENTICATED — set FUNNELBARN_ADMIN_*, configure OIDC, or run 'funnelbarn user create'",
			"handled", false)
	}

	apiServer := api.NewServer(api.ServerConfig{
		InstanceSettings:      store,
		GeoAnonymizer:         store,
		Segments:              segmentsSvc,
		Distributions:         store,
		Ingest:                handler,
		Projects:              projectsSvc,
		Funnels:               funnelsSvc,
		ABTests:               abtestsSvc,
		Flags:                 flagsSvc,
		Events:                eventsSvc,
		Overview:              overviewSvc,
		Sessions:              sessionsSvc,
		APIKeys:               apikeysSvc,
		Widgets:               widgetsSvc,
		UserAuth:              userAuth,
		SessionManager:        sessionManager,
		LocalUsersExist:       localUserCount > 0,
		AllowedOrigins:        cfg.AllowedOrigins,
		SessionSecret:         cfg.SessionSecret,
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
		TrustedProxies:        cfg.TrustedProxies,
		BugbarnEndpoint:       cfg.SelfEndpoint,
		BugbarnIngestKey:      cfg.SelfAPIKey,
		BugbarnProject:        cfg.SelfProject,
		DogfoodAPIKey:         cfg.DogfoodAPIKey,
		DogfoodProject:        cfg.DogfoodProject,
		IAMBarnUsers:          store,
		PostLogoutRedirectURI: cfg.PostLogoutRedirectURI,
		OIDC:                  oidcClient,
		WebSessions:           store,
		Environment:           cfg.SelfEnvironment,
		OIDCRefreshGrace:      time.Duration(cfg.OIDCRefreshGraceSeconds) * time.Second,
		Recordings:            recordingsSvc,
		RecordingSettings:     store,
		ProjectHealth:         healthSvc,
		FlagAutoRegisterMax:   cfg.AutoRegisterMaxFlags,
		Commands:              commands,
		ReadPoolWait:          func() time.Duration { return store.ReadDB().Stats().WaitDuration },
		SpanRelay:             spanRelay,
		MCPResourceURL:        cfg.MCPResourceURL,
	})
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

// resolveEventProject looks up the project a spool record belongs to. A record
// with no slug, or one whose slug can never resolve, comes back wrapped in
// repository.ErrProjectUnresolvable: events.project_id is NOT NULL REFERENCES
// projects(id), so persisting it could only ever fail the foreign key.
func resolveEventProject(ctx context.Context, store *repository.Store, slug string) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("%w: spool record carries no project slug", repository.ErrProjectUnresolvable)
	}
	proj, err := store.EnsureProject(ctx, slug)
	if err != nil {
		return "", err
	}
	return proj.ID, nil
}

func runBackgroundWorker(ctx context.Context, cfg config.Config, store *repository.Store, eventSpool *spool.Spool, applier *ingestApplier, ingestQ *ingestQueue, recordings service.Recordings) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	purgeTicker := time.NewTicker(24 * time.Hour)
	defer purgeTicker.Stop()

	// A restart must not postpone maintenance indefinitely; see
	// startupMaintenanceDelay.
	startupMaintenance := time.NewTimer(startupMaintenanceDelay)
	defer startupMaintenance.Stop()

	offset, err := spool.ReadCursor(cfg.SpoolDir)
	if err != nil {
		slog.Warn("worker: failed to read cursor, starting from 0", "err", err)
		offset = 0
	}

	// Surface silent failure modes (stalled consumer, geo resolving nothing) as
	// BugBarn issues via slog.Error.
	health := applier.health

	retryCounts := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return
		case <-startupMaintenance.C:
			runMaintenance(ctx, cfg, store, recordings)
		case <-purgeTicker.C:
			runMaintenance(ctx, cfg, store, recordings)
		case <-ticker.C:
			tickCtx, tickSpan := tracing.StartSpan(ctx, "worker.tick",
				attribute.Int64("spool.offset", offset),
			)

			entries, err := spool.ReadRecordsFrom(spool.Path(cfg.SpoolDir), offset)
			if err != nil {
				tracing.RecordError(tickSpan, err)
				tickSpan.End()
				slog.Error("worker read spool", "err", err)
				checkSpoolProgress(health, cfg.SpoolDir, offset)
				continue
			}
			tickSpan.SetAttributes(attribute.Int("spool.entries", len(entries)))
			metrics.SpoolQueueDepth.Set(float64(len(entries)))
			checkSpoolProgress(health, cfg.SpoolDir, offset)

			if ingestQ != nil {
				offset = ingestQ.forwarder.forward(tickCtx, entries, offset)
			} else {
				offset = applier.applyEntries(tickCtx, cfg.SpoolDir, entries, offset, retryCounts)
			}

			rotateSpool(tickCtx, eventSpool, tickSpan)
			tickSpan.End()
		}
	}
}
