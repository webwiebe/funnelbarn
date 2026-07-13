package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"

	selfsdk "github.com/webwiebe/funnelbarn/sdks/go"
	bb "github.com/wiebe-xyz/bugbarn-go"

	"github.com/wiebe-xyz/funnelbarn/internal/api"
	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/geoip"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/storage"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
	"github.com/wiebe-xyz/funnelbarn/internal/worker"
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
// It always writes JSON to stderr, optionally fans out Warn+ to BugBarn, and
// (when spanbarn != nil) ships >= SpanBarnLogLevel records to SpanBarn via OTLP,
// minus the high-volume health-probe access logs.
func buildLogger(cfg config.Config, spanbarn slog.Handler) *slog.Logger {
	var handlers []slog.Handler

	// Always: structured JSON to stderr.
	jsonHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})
	handlers = append(handlers, jsonHandler)

	// Optional: BugBarn for Warn+ events.
	if cfg.SelfEndpoint != "" && cfg.SelfAPIKey != "" {
		bbHandler := bblog.NewHandler(jsonHandler)
		handlers = append(handlers, bbHandler)
	}

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
	slog.SetDefault(buildLogger(cfg, spanbarnLogHandler))
	if selfReporting {
		slog.Info("self-reporting enabled", "endpoint", cfg.SelfEndpoint)
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

	if cfg.SpanBarnEndpoint != "" {
		slog.Info("spanbarn telemetry enabled", "endpoint", cfg.SpanBarnEndpoint, "signals", "traces,metrics,logs")
	}

	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer store.Close()

	// Wire services.
	projectsSvc := service.NewProjectService(store)
	funnelsSvc := service.NewFunnelService(store)
	abtestsSvc := service.NewABTestService(store)
	flagsSvc := service.NewFlagService(store)
	eventsSvc := service.NewEventService(store)
	overviewSvc := service.NewOverviewService(store)
	sessionsSvc := service.NewSessionService(store)
	apikeysSvc := service.NewAPIKeyService(store)
	widgetsSvc := service.NewWidgetService(store)
	segmentsSvc := service.NewSegmentService(store)
	healthSvc := service.NewProjectHealthService(store)

	eventSpool, err := spool.NewWithLimit(cfg.SpoolDir, cfg.MaxSpoolBytes)
	if err != nil {
		return fmt.Errorf("open spool: %w", err)
	}
	defer eventSpool.Close()

	var recordingsSvc service.Recordings
	if cfg.R2Endpoint != "" && cfg.R2AccessKeyID != "" && cfg.R2SecretAccessKey != "" && cfg.R2Bucket != "" {
		r2, r2err := storage.NewR2(cfg.R2Endpoint, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2Bucket)
		if r2err != nil {
			slog.Warn("session recording disabled: failed to initialize R2 storage", "err", r2err)
		} else {
			recordingsSvc = service.NewRecordingService(store, store, store, r2)
			slog.Info("session recording enabled", "bucket", cfg.R2Bucket)
		}
	}

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

	go runBackgroundWorker(ctx, cfg, store, eventSpool, geoLookup, recordingsSvc)

	apiAuthorizer, err := newAPIAuthorizer(cfg, store)
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
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// buildOIDCClient returns an OIDC adapter when all four FUNNELBARN_OIDC_* vars
// are set, or nil otherwise (in which case the local single-user login is the
// auth path). Discovery is lazy so an unreachable issuer at startup does not
// crash the process.
func buildOIDCClient(cfg config.Config) *auth.OIDCClient {
	oc := auth.OIDCConfig{
		Issuer:                cfg.OIDCIssuer,
		ClientID:              cfg.OIDCClientID,
		ClientSecret:          cfg.OIDCClientSecret,
		RedirectURL:           cfg.OIDCRedirectURL,
		RequiredGroup:         cfg.OIDCRequiredGroup,
		PostLogoutRedirectURI: cfg.PostLogoutRedirectURI,
	}
	if !oc.Enabled() {
		return nil
	}
	slog.Info("oidc: enabled", "issuer", oc.Issuer, "client_id", oc.ClientID, "required_group", oc.RequiredGroup)
	return auth.NewOIDCClient(oc)
}

// validateFailClosed refuses to start a production deployment whose auth
// surfaces would silently fail open: the ingest API accepting any key because
// none is configured, or the dashboard API serving every route unauthenticated
// because no login mechanism exists. Non-production tiers keep the permissive
// behaviour for local development and throwaway environments.
func validateFailClosed(env string, apiKeyConfigured, authConfigured bool) error {
	if env != environment.Production {
		return nil
	}
	if !apiKeyConfigured {
		return errors.New("refusing to start in production without an API key: ingest would accept ANY key — set FUNNELBARN_API_KEY(_SHA256) or create a key with 'funnelbarn apikey create'")
	}
	if !authConfigured {
		return errors.New("refusing to start in production without an authentication mechanism: dashboard routes would be served unauthenticated — set FUNNELBARN_ADMIN_*, configure FUNNELBARN_OIDC_*, or run 'funnelbarn user create'")
	}
	return nil
}

func newAPIAuthorizer(cfg config.Config, store *repository.Store) (*auth.Authorizer, error) {
	var base *auth.Authorizer
	var err error
	if cfg.APIKeySHA256 != "" {
		base, err = auth.NewHashed(cfg.APIKeySHA256)
		if err != nil {
			return nil, err
		}
	} else {
		base = auth.New(cfg.APIKey)
	}
	return base.WithDBLookup(store.ValidAPIKeySHA256, store.TouchAPIKey), nil
}

const (
	workerMaxRetries      = 3
	workerRotateThreshold = 64 << 20 // 64 MiB
)

func runBackgroundWorker(ctx context.Context, cfg config.Config, store *repository.Store, eventSpool *spool.Spool, geoLookup *geoip.Lookup, recordings service.Recordings) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	purgeTicker := time.NewTicker(24 * time.Hour)
	defer purgeTicker.Stop()

	offset, err := spool.ReadCursor(cfg.SpoolDir)
	if err != nil {
		slog.Warn("worker: failed to read cursor, starting from 0", "err", err)
		offset = 0
	}

	// Surface silent failure modes (stalled consumer, geo resolving nothing) as
	// BugBarn issues via slog.Error.
	health := workerhealth.New(workerhealth.Options{})

	retryCounts := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return
		case <-purgeTicker.C:
			purgeCtx, purgeSpan := tracing.StartSpan(ctx, "maintenance.purge")
			if cfg.EventRetentionDays > 0 {
				cutoff := time.Now().AddDate(0, 0, -cfg.EventRetentionDays)
				n, err := store.PurgeOldEvents(purgeCtx, cutoff)
				if err != nil {
					tracing.RecordError(purgeSpan, err)
					slog.Error("purge old events", "err", err)
				} else if n > 0 {
					slog.Info("purged old events", "count", n, "before", cutoff.Format(time.DateOnly))
					metrics.EventsPurged.Add(float64(n))
				}
				ne, err := store.PurgeOldEvaluations(purgeCtx, cutoff)
				if err != nil {
					tracing.RecordError(purgeSpan, err)
					slog.Error("purge old evaluations", "err", err)
				} else if ne > 0 {
					slog.Info("purged old evaluations", "count", ne, "before", cutoff.Format(time.DateOnly))
				}
			}
			if cfg.AutoRegisterTTLDays > 0 {
				cutoff := time.Now().AddDate(0, 0, -cfg.AutoRegisterTTLDays)
				nf, err := store.PurgeStaleAutoFlags(purgeCtx, cutoff)
				if err != nil {
					tracing.RecordError(purgeSpan, err)
					slog.Error("purge stale auto flags", "err", err)
				} else if nf > 0 {
					slog.Info("purged stale auto flags", "count", nf, "before", cutoff.Format(time.DateOnly))
				}
			}
			if recordings != nil {
				retentionDays := 90 // default
				if v, ok, _ := store.GetInstanceSetting(purgeCtx, "recording_retention_days"); ok {
					if n, err := strconv.Atoi(v); err == nil && n > 0 {
						retentionDays = n
					}
				}
				if err := recordings.PurgeOldRecordings(purgeCtx, retentionDays); err != nil {
					tracing.RecordError(purgeSpan, err)
					slog.Error("purge old recordings", "err", err)
				} else {
					slog.Debug("recording retention purge complete", "retention_days", retentionDays)
				}
				if n, err := recordings.PurgeBrokenRecordings(purgeCtx); err != nil {
					tracing.RecordError(purgeSpan, err)
					slog.Error("purge broken recordings", "err", err)
				} else if n > 0 {
					slog.Info("purged unplayable recordings", "count", n)
				}
			}
			purgeSpan.End()
		case <-ticker.C:
			tickCtx, tickSpan := tracing.StartSpan(ctx, "worker.tick",
				attribute.Int64("spool.offset", offset),
			)

			entries, err := spool.ReadRecordsFrom(spool.Path(cfg.SpoolDir), offset)
			if err != nil {
				tracing.RecordError(tickSpan, err)
				tickSpan.End()
				slog.Error("worker read spool", "err", err)
				continue
			}
			tickSpan.SetAttributes(attribute.Int("spool.entries", len(entries)))
			metrics.SpoolQueueDepth.Set(float64(len(entries)))

			// Stall detection: if the spool has pending bytes the cursor isn't
			// draining, raise an issue. This is the blindspot that hid a ~5-day
			// ingestion outage.
			if size, serr := spool.ActiveSize(cfg.SpoolDir); serr == nil {
				pending := size - offset
				if pending < 0 {
					pending = 0
				}
				metrics.IngestPendingBytes.Set(float64(pending))
				if stalled, since := health.CheckProgress(offset, pending); stalled {
					slog.Error("ingest worker stalled: spool backlog not draining",
						"handled", false,
						"pending_bytes", pending,
						"offset", offset,
						"stalled_for", since.Round(time.Second).String())
				}
				stalledGauge := 0.0
				if health.Stalled() {
					stalledGauge = 1
				}
				metrics.IngestStalled.Set(stalledGauge)
			}

			// Check geo_enabled once per batch to avoid a DB round-trip per event.
			geoEnabled := geoLookup != nil
			if geoEnabled {
				val, _, _ := store.GetInstanceSetting(tickCtx, "geo_enabled")
				geoEnabled = val != "false"
			}

			for _, entry := range entries {
				record := entry.Record

				event, err := worker.SafeProcess(record)
				if err != nil {
					retryCounts[record.IngestID]++
					slog.Error("worker process record",
						"ingest_id", record.IngestID,
						"attempt", retryCounts[record.IngestID],
						"err", err,
					)
					if retryCounts[record.IngestID] >= workerMaxRetries {
						slog.Error("worker dead-lettering record",
							"ingest_id", record.IngestID,
							"attempts", retryCounts[record.IngestID],
						)
						if dlErr := spool.AppendDeadLetter(cfg.SpoolDir, record); dlErr != nil {
							slog.Error("worker dead-letter write", "ingest_id", record.IngestID, "err", dlErr)
						}
						metrics.EventErrors.WithLabelValues("dead_letter").Inc()
						delete(retryCounts, record.IngestID)
						offset = entry.EndOffset
						if err := spool.WriteCursor(cfg.SpoolDir, offset); err != nil {
							slog.Error("worker write cursor", "err", err)
						}
					} else {
						metrics.EventErrors.WithLabelValues("retry").Inc()
					}
					break
				}

				eventStart := time.Now()

				// Resolve project from the slug stored in the spool record.
				// Each DB operation gets a 30s timeout so a stuck write can't block the worker.
				opCtx, opCancel := context.WithTimeout(tickCtx, 30*time.Second)
				opCtx, span := tracing.StartSpan(opCtx, "worker.persist_event",
					attribute.String("ingest.id", record.IngestID),
					attribute.String("project.slug", record.ProjectSlug),
				)
				if record.ProjectSlug != "" {
					proj, err := store.EnsureProject(opCtx, record.ProjectSlug)
					if err == nil {
						event.ProjectID = proj.ID
					} else {
						slog.Warn("worker ensure project", "slug", record.ProjectSlug, "err", err)
					}
				}

				var geoResult *geoip.GeoResult
				if geoEnabled {
					geoResult = geoLookup.Lookup(event.ClientIP)
					// Geo is on but resolving nothing usually means the real
					// client IP isn't reaching us (proxy/forwarding config).
					hit := geoResult != nil && geoResult.CountryCode != ""
					metrics.GeoLookups.Inc()
					if hit {
						metrics.GeoHits.Inc()
					}
					if alert, n := health.RecordGeo(hit); alert {
						slog.Error("geo enrichment resolved 0 countries over recent lookups; check FUNNELBARN_TRUSTED_PROXIES and client-IP forwarding",
							"handled", false, "lookups", n)
					}
				}

				persistErr := worker.PersistEvent(opCtx, store, event, geoResult)
				if persistErr != nil {
					tracing.RecordError(span, persistErr)
				}
				span.End()
				opCancel()
				metrics.EventProcessingDuration.Observe(time.Since(eventStart).Seconds())
				if persistErr != nil {
					retryCounts[record.IngestID]++
					slog.Error("worker persist record",
						"ingest_id", record.IngestID,
						"attempt", retryCounts[record.IngestID],
						"err", persistErr,
					)
					if retryCounts[record.IngestID] >= workerMaxRetries {
						slog.Error("worker dead-lettering record after persist failures",
							"ingest_id", record.IngestID,
						)
						if dlErr := spool.AppendDeadLetter(cfg.SpoolDir, record); dlErr != nil {
							slog.Error("worker dead-letter write", "ingest_id", record.IngestID, "err", dlErr)
						}
						metrics.EventErrors.WithLabelValues("dead_letter").Inc()
						delete(retryCounts, record.IngestID)
						offset = entry.EndOffset
						if err := spool.WriteCursor(cfg.SpoolDir, offset); err != nil {
							slog.Error("worker write cursor", "err", err)
						}
					} else {
						metrics.EventErrors.WithLabelValues("retry").Inc()
					}
					break
				}

				metrics.EventsProcessed.Inc()
				delete(retryCounts, record.IngestID)
				offset = entry.EndOffset
				if err := spool.WriteCursor(cfg.SpoolDir, offset); err != nil {
					slog.Error("worker write cursor", "err", err)
				}
			}

			if err := eventSpool.RotateIfExceeds(workerRotateThreshold); err != nil {
				tracing.RecordError(tickSpan, err)
				slog.Error("worker rotate spool", "err", err)
			}
			tickSpan.End()
		}
	}
}

// --------------------------------------------------------------------------
// CLI subcommands
// --------------------------------------------------------------------------

// runWorkerOnce replays queued records into the persistent store.
func runWorkerOnce(cfg config.Config) error {
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	records, err := spool.ReadRecords(spool.Path(cfg.SpoolDir))
	if err != nil {
		return err
	}

	ctx, span := tracing.StartSpan(context.Background(), "worker.replay_spool",
		attribute.Int("records", len(records)),
	)
	defer span.End()

	processed := 0
	for _, record := range records {
		event, err := worker.SafeProcess(record)
		if err != nil {
			tracing.RecordError(span, err)
			slog.Error("worker-once: process record", "ingest_id", record.IngestID, "err", err)
			continue
		}
		if record.ProjectSlug != "" {
			proj, projErr := store.EnsureProject(ctx, record.ProjectSlug)
			if projErr == nil {
				event.ProjectID = proj.ID
			}
		}
		if err := worker.PersistEvent(ctx, store, event, nil); err != nil {
			tracing.RecordError(span, err)
			slog.Error("worker-once: persist event", "ingest_id", record.IngestID, "err", err)
			continue
		}
		processed++
	}
	span.SetAttributes(attribute.Int("processed", processed))

	fmt.Printf("{\"records\":%d,\"processed\":%d}\n", len(records), processed)
	return nil
}

// runUserCmd handles: funnelbarn user create --username=X --password=Y
func runUserCmd(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: funnelbarn user <create>")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("user create", flag.ContinueOnError)
		username := fs.String("username", os.Getenv("FUNNELBARN_ADMIN_USERNAME"), "username")
		password := fs.String("password", os.Getenv("FUNNELBARN_ADMIN_PASSWORD"), "plaintext password")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		*username = strings.TrimSpace(*username)
		*password = strings.TrimSpace(*password)
		if *username == "" {
			return fmt.Errorf("--username is required")
		}
		if *password == "" {
			return fmt.Errorf("--password is required")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		store, err := repository.Open(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.UpsertUser(context.Background(), *username, string(hash)); err != nil {
			return fmt.Errorf("upsert user: %w", err)
		}
		fmt.Printf("user %q created/updated\n", *username)
		return nil
	default:
		return fmt.Errorf("unknown user subcommand %q", args[0])
	}
}

// runProjectCmd handles: funnelbarn project create --name=X [--slug=Y]
func runProjectCmd(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: funnelbarn project <create>")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("project create", flag.ContinueOnError)
		name := fs.String("name", "", "project display name")
		slug := fs.String("slug", "", "project slug (defaults to slugified name)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		*name = strings.TrimSpace(*name)
		if *name == "" {
			return fmt.Errorf("--name is required")
		}
		if *slug == "" {
			*slug = toSlugLocal(*name)
		}
		if !slugPattern.MatchString(*slug) {
			return fmt.Errorf("invalid slug %q: must be lowercase alphanumeric with hyphens", *slug)
		}
		store, err := repository.Open(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()
		p, err := store.CreateProject(context.Background(), *name, *slug)
		if err != nil {
			return fmt.Errorf("create project: %w", err)
		}
		fmt.Printf("{\"id\":%q,\"name\":%q,\"slug\":%q}\n", p.ID, p.Name, p.Slug)
		return nil
	default:
		return fmt.Errorf("unknown project subcommand %q", args[0])
	}
}

// runAPIKeyCmd handles: funnelbarn apikey create --project=default --name=my-app
func runAPIKeyCmd(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: funnelbarn apikey <create>")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("apikey create", flag.ContinueOnError)
		projectSlug := fs.String("project", "default", "project slug")
		name := fs.String("name", "", "key name/label")
		scope := fs.String("scope", repository.APIKeyScopeFull, "key scope: full or ingest")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		*name = strings.TrimSpace(*name)
		if *name == "" {
			return fmt.Errorf("--name is required")
		}
		if *scope != repository.APIKeyScopeFull && *scope != repository.APIKeyScopeIngest {
			return fmt.Errorf("--scope must be %q or %q", repository.APIKeyScopeFull, repository.APIKeyScopeIngest)
		}
		store, err := repository.Open(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()
		ctx := context.Background()
		project, err := store.ProjectBySlug(ctx, *projectSlug)
		if err != nil {
			project, err = store.CreateProject(ctx, *projectSlug, *projectSlug)
			if err != nil {
				return fmt.Errorf("create project %q: %w", *projectSlug, err)
			}
			fmt.Printf("Project %q created automatically.\n", *projectSlug)
		}
		var raw [32]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return fmt.Errorf("generate key: %w", err)
		}
		plaintext := hex.EncodeToString(raw[:])
		sum := sha256.Sum256([]byte(plaintext))
		keySHA256 := hex.EncodeToString(sum[:])

		key, err := store.CreateAPIKey(ctx, *name, project.ID, keySHA256, *scope)
		if err != nil {
			return fmt.Errorf("create api key: %w", err)
		}
		fmt.Printf("API key created (id=%s, project=%s, name=%s, scope=%s)\n", key.ID, project.Slug, key.Name, key.Scope)
		fmt.Printf("Key (shown once, store securely): %s\n", plaintext)
		return nil
	default:
		return fmt.Errorf("unknown apikey subcommand %q", args[0])
	}
}

// toSlugLocal converts a display name to a URL-safe slug.
func toSlugLocal(name string) string {
	s := strings.ToLower(name)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
