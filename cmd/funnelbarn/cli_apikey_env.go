package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// runAPIKeySetEnvironment handles: funnelbarn apikey set-environment --id=<key id> --environment=production
//
// A key mapped to an environment files every event it carries under that
// environment, whatever the event says (#545). --environment= (empty) clears
// the mapping.
func runAPIKeySetEnvironment(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("apikey set-environment", flag.ContinueOnError)
	id := fs.String("id", "", "API key id")
	env := fs.String("environment", "", "environment to map the key to; empty clears it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	if !environment.IsKnown(*env) {
		return fmt.Errorf("--environment %q is not a known environment", *env)
	}
	store, err := repository.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer logShutdown(slog.LevelError, "store", store.Close)
	ctx := context.Background()
	canonical := environment.Normalize(*env)
	if err := store.SetAPIKeyEnvironment(ctx, *id, canonical); err != nil {
		return fmt.Errorf("set environment: %w", err)
	}
	fmt.Printf("{\"id\":%q,\"environment\":%q}\n", *id, canonical)
	return nil
}
