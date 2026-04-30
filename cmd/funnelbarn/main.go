package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	bb "github.com/wiebe-xyz/bugbarn-go"

	"github.com/wiebe-xyz/funnelbarn/internal/api"
	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/worker"
)

// Version and BuildTime are injected at build time via -ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

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
// It always writes JSON to stderr, and optionally fans out Warn+ to BugBarn.
func buildLogger(cfg config.Config) *slog.Logger {
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

	return slog.New(bblog.NewMultiHandler(handlers...))
}

// run owns process wiring: opens storage, starts the worker, and serves the API.
func run() error {
	cfg := config.Load()

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

	if cfg.SessionSecret == "" {
		slog.Warn("FUNNELBARN_SESSION_SECRET is not set; sessions will not persist across restarts")
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
	eventsSvc := service.NewEventService(store)
	sessionsSvc := service.NewSessionService(store)
	apikeysSvc := service.NewAPIKeyService(store)

	eventSpool, err := spool.NewWithLimit(cfg.SpoolDir, cfg.MaxSpoolBytes)
	if err != nil {
		return fmt.Errorf("open spool: %w", err)
	}
	defer eventSpool.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wire BugBarn self-reporting and set up multi-sink logger.
	selfReporting := cfg.SelfEndpoint != "" && cfg.SelfAPIKey != ""
	if selfReporting {
		bb.Init(bb.Options{
			APIKey:      cfg.SelfAPIKey,
			Endpoint:    cfg.SelfEndpoint,
			ProjectSlug: "funnelbarn",
			Environment: cfg.SelfEnvironment,
		})
		defer bb.Shutdown(2 * time.Second)
	}
	// Rewire the global logger with the appropriate sinks.
	slog.SetDefault(buildLogger(cfg))
	if selfReporting {
		slog.Info("self-reporting enabled", "endpoint", cfg.SelfEndpoint)
	}

	go runBackgroundWorker(ctx, cfg, store)

	apiAuthorizer, err := newAPIAuthorizer(cfg, store)
	if err != nil {
		return err
	}
	userAuth, err := auth.NewUserAuthenticator(cfg.AdminUsername, cfg.AdminPassword, cfg.AdminPasswordBcrypt)
	if err != nil {
		return err
	}
	sessionManager := auth.NewSessionManager(cfg.SessionSecret, cfg.SessionTTL)
	handler := ingest.NewHandler(apiAuthorizer, eventSpool, cfg.MaxBodyBytes)
	go handler.Start(ctx)

	apiServer := api.NewServer(
		handler,
		projectsSvc,
		funnelsSvc,
		abtestsSvc,
		eventsSvc,
		sessionsSvc,
		apikeysSvc,
		userAuth,
		sessionManager,
		cfg.AllowedOrigins,
		cfg.SessionSecret,
		cfg.PublicURL,
		cfg.LoginRatePerMinute,
		cfg.LoginRateBurst,
		cfg.APIRatePerMinute,
		cfg.APIRateBurst,
		cfg.IngestRatePerMinute,
		cfg.IngestRateBurst,
		store,
		Version,
	)
	if cfg.MetricsToken != "" {
		apiServer.SetMetricsToken(cfg.MetricsToken)
	}

	var httpHandler http.Handler = apiServer
	if selfReporting {
		httpHandler = bb.RecoverMiddleware(httpHandler)
	}

	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: httpHandler,
	}

	slog.Info("funnelbarn starting", "addr", cfg.Addr, "version", Version)

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

func runBackgroundWorker(ctx context.Context, cfg config.Config, store *repository.Store) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	purgeTicker := time.NewTicker(24 * time.Hour)
	defer purgeTicker.Stop()

	offset, err := spool.ReadCursor(cfg.SpoolDir)
	if err != nil {
		slog.Warn("worker: failed to read cursor, starting from 0", "err", err)
		offset = 0
	}

	retryCounts := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return
		case <-purgeTicker.C:
			if cfg.EventRetentionDays > 0 {
				cutoff := time.Now().AddDate(0, 0, -cfg.EventRetentionDays)
				n, err := store.PurgeOldEvents(ctx, cutoff)
				if err != nil {
					slog.Error("purge old events", "err", err)
				} else if n > 0 {
					slog.Info("purged old events", "count", n, "before", cutoff.Format(time.DateOnly))
					metrics.EventsPurged.Add(float64(n))
				}
			}
		case <-ticker.C:
			entries, err := spool.ReadRecordsFrom(spool.Path(cfg.SpoolDir), offset)
			if err != nil {
				slog.Error("worker read spool", "err", err)
				continue
			}
			metrics.SpoolQueueDepth.Set(float64(len(entries)))

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
						metrics.EventErrors.Inc()
						delete(retryCounts, record.IngestID)
						offset = entry.EndOffset
						if err := spool.WriteCursor(cfg.SpoolDir, offset); err != nil {
							slog.Error("worker write cursor", "err", err)
						}
					}
					break
				}

				// Resolve project from the slug stored in the spool record.
				// Each DB operation gets a 30s timeout so a stuck write can't block the worker.
				opCtx, opCancel := context.WithTimeout(ctx, 30*time.Second)
				if record.ProjectSlug != "" {
					proj, err := store.EnsureProject(opCtx, record.ProjectSlug)
					if err == nil {
						event.ProjectID = proj.ID
					} else {
						slog.Warn("worker ensure project", "slug", record.ProjectSlug, "err", err)
					}
				}

				persistErr := worker.PersistEvent(opCtx, store, event)
				opCancel()
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
						metrics.EventErrors.Inc()
						delete(retryCounts, record.IngestID)
						offset = entry.EndOffset
						if err := spool.WriteCursor(cfg.SpoolDir, offset); err != nil {
							slog.Error("worker write cursor", "err", err)
						}
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

			if err := spool.RotateIfExceedsPath(cfg.SpoolDir, workerRotateThreshold); err != nil {
				slog.Error("worker rotate spool", "err", err)
			}
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

	processed := 0
	ctx := context.Background()
	for _, record := range records {
		event, err := worker.SafeProcess(record)
		if err != nil {
			slog.Error("worker-once: process record", "ingest_id", record.IngestID, "err", err)
			continue
		}
		if record.ProjectSlug != "" {
			proj, projErr := store.EnsureProject(ctx, record.ProjectSlug)
			if projErr == nil {
				event.ProjectID = proj.ID
			}
		}
		if err := worker.PersistEvent(ctx, store, event); err != nil {
			slog.Error("worker-once: persist event", "ingest_id", record.IngestID, "err", err)
			continue
		}
		processed++
	}

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
