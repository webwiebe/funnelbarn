package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/api"
	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// Version, Commit and BuildTime are injected at build time via -ldflags.
// Version is overridden at run time by FUNNELBARN_VERSION (see run); Commit and
// BuildTime are facts about the binary and are served as built.
var (
	Version   = "dev"
	Commit    = "unknown"
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
// Each stage registers its shutdown steps on cleanup, which runs them in reverse
// order when run returns, so the store closes after everything that writes to it.
func run() error {
	if handled, code := handleUsageArgs(os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	cfg := config.Load()
	version := runtimeVersion(cfg)

	if handled, err := runSubcommand(cfg, os.Args[1:]); handled {
		return err
	}
	if err := cfg.ValidateMode(); err != nil {
		return err
	}
	if err := checkSessionSecret(cfg.SessionSecret); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var cleanup cleanupStack
	defer cleanup.run()

	// Telemetry (logging sinks, tracer, and OTel MeterProvider) is wired up
	// front, before repository.Open: see startTelemetry.
	tele, err := startTelemetry(ctx, cfg, version, &cleanup)
	if err != nil {
		return err
	}
	data, err := openData(ctx, cfg, &cleanup)
	if err != nil {
		return err
	}
	serverCfg, keySnapshot := newServerConfig(ctx, cfg, version, data)
	serverCfg.SpanRelay = tele.spanRelay

	ing, err := startIngest(ctx, cfg, data, &cleanup)
	if err != nil {
		return err
	}
	serverCfg.Recordings = ing.recordings
	serverCfg.Ready = ing.ready

	authn, err := newAuthStack(ctx, cfg, data, keySnapshot)
	if err != nil {
		return err
	}
	handler := newIngestHandler(ctx, cfg, authn.authorizer, ing.spool, data.health, &cleanup)
	serverCfg.Ingest = handler
	serverCfg.UserAuth = authn.users
	serverCfg.SessionManager = authn.sessions

	oidcClient := buildOIDCClient(cfg)
	localUsers, err := checkAuthentication(ctx, cfg, data.store, authn, oidcClient)
	if err != nil {
		return err
	}
	serverCfg.LocalUsersExist = localUsers > 0
	serverCfg.OIDC = oidcClient

	return serve(ctx, cfg, version, api.NewServer(serverCfg), tele.selfReporting)
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
