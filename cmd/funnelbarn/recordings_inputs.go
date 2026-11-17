package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/blobstore"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

const checkInputsUsage = "usage: funnelbarn recordings check-inputs --id <recording id> [--id ...] [--contains <text> ...]"

// idList collects a repeated --id flag.
type idList []string

func (l *idList) String() string     { return strings.Join(*l, ",") }
func (l *idList) Set(v string) error { *l = append(*l, v); return nil }

// checkInputsFlags are the options of recordings check-inputs.
type checkInputsFlags struct {
	ids     idList
	needles idList
}

func parseCheckInputsFlags(args []string) (checkInputsFlags, error) {
	fs := flag.NewFlagSet("recordings check-inputs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var f checkInputsFlags
	fs.Var(&f.ids, "id", "recording id to check (repeatable)")
	fs.Var(&f.needles, "contains", "text to look for in the stored chunks; only yes or no is reported (repeatable)")
	if err := fs.Parse(args); err != nil {
		return f, fmt.Errorf("%s: %w", checkInputsUsage, err)
	}
	if len(f.ids) == 0 || fs.NArg() > 0 {
		return f, errors.New(checkInputsUsage)
	}
	return f, nil
}

// inputChecker is the one service method the command needs.
type inputChecker interface {
	CheckInputs(ctx context.Context, recordingID string, needles ...string) (service.InputCheck, error)
}

// writeInputChecks prints one JSON line per recording: counts per input type,
// never a value.
func writeInputChecks(ctx context.Context, w io.Writer, checker inputChecker, ids, needles []string) error {
	enc := json.NewEncoder(w)
	for _, id := range ids {
		res, err := checker.CheckInputs(ctx, id, needles...)
		if err != nil {
			return fmt.Errorf("recordings check-inputs: %s: %w", id, err)
		}
		if err := enc.Encode(res); err != nil {
			return err
		}
	}
	return nil
}

// runCheckInputs handles: funnelbarn recordings check-inputs --id <id>. It
// opens the database read-only and only reads object storage.
func runCheckInputs(cfg config.Config, args []string) error {
	flags, err := parseCheckInputsFlags(args)
	if err != nil {
		return err
	}
	if cfg.R2Endpoint == "" || cfg.R2AccessKeyID == "" || cfg.R2SecretAccessKey == "" || cfg.R2Bucket == "" {
		return errors.New("recordings check-inputs: object storage is not configured")
	}
	store, err := repository.OpenReadOnly(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	r2, err := blobstore.NewR2(cfg.R2Endpoint, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2Bucket)
	if err != nil {
		return fmt.Errorf("recordings check-inputs: object storage: %w", err)
	}
	return writeInputChecks(context.Background(), os.Stdout, service.NewRecordingService(store, store, store, r2), flags.ids, flags.needles)
}
