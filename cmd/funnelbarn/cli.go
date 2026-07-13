package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
	"github.com/wiebe-xyz/funnelbarn/internal/worker"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

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
