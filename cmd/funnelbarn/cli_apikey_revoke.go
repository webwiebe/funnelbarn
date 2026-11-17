package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// keyStore is the store methods revoke needs.
type keyStore interface {
	ListAllAPIKeys(ctx context.Context) ([]domain.APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error
}

// runAPIKeyRevoke handles: funnelbarn apikey revoke --id=<key id>
//
// It deletes one API key. It refuses a key that is the only one its project
// holds, so a rotation cannot leave a project without a key. The output names
// the key and says when it was last used; it never prints a key or its hash.
func runAPIKeyRevoke(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("apikey revoke", flag.ContinueOnError)
	id := fs.String("id", "", "API key id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("--id is required")
	}
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer logShutdown(slog.LevelError, "store", store.Close)
	return revokeAPIKey(context.Background(), store, *id)
}

func revokeAPIKey(ctx context.Context, store keyStore, id string) error {
	keys, err := store.ListAllAPIKeys(ctx)
	if err != nil {
		return fmt.Errorf("revoke: list keys: %w", err)
	}
	var target *domain.APIKey
	siblings := 0
	for i := range keys {
		if keys[i].ID == id {
			target = &keys[i]
		}
	}
	if target == nil {
		return fmt.Errorf("revoke: no API key with id %q", id)
	}
	for _, k := range keys {
		if k.ProjectID == target.ProjectID && k.ID != id {
			siblings++
		}
	}
	if siblings == 0 {
		return fmt.Errorf("revoke: key %q is the only key of its project; create its replacement first", id)
	}
	last := "never"
	if target.LastUsedAt != nil {
		last = target.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if err := store.DeleteAPIKey(ctx, id); err != nil {
		return fmt.Errorf("revoke: %w", err)
	}
	fmt.Printf("{\"revoked\":%q,\"name\":%q,\"scope\":%q,\"last_used_at\":%q,\"project_keys_left\":%d}\n", id, target.Name, target.Scope, last, siblings)
	return nil
}
