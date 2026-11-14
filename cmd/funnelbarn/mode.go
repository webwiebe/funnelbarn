package main

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest/pipeline"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// FUNNELBARN_MODE splits one process into readers and a writer (spec 012).
// A standalone process and a writer open the database read-write and consume
// every queue they use; a reader opens it read-only and publishes every write
// to Valkey, buffering on disk while Valkey is down.

// isReader reports whether this process runs as a reader.
func isReader(cfg config.Config) bool { return pipeline.IsReader(cfg) }

// isSplit reports whether this process is one half of a reader/writer pair.
// Both halves move ingest and recordings through Valkey, whatever the
// per-queue flags say.
func isSplit(cfg config.Config) bool { return pipeline.IsSplit(cfg) }

// forwardTarget is where this process forwards the requests that write the
// database: the writer for a reader, nowhere otherwise.
func forwardTarget(cfg config.Config) string {
	if isReader(cfg) {
		return cfg.WriterURL
	}
	return ""
}

// openStore opens the database for this process's role.
func openStore(cfg config.Config) (*repository.Store, error) {
	if isReader(cfg) {
		return repository.OpenReadOnly(cfg.DBPath)
	}
	return repository.Open(cfg.DBPath)
}

// schemaGate keeps a reader out of the Service until the writer has migrated
// the database to the version this binary was built for.
type schemaGate struct {
	store *repository.ReadStore
	want  int64
	ok    atomic.Bool
}

func newSchemaGate(store *repository.ReadStore) (*schemaGate, error) {
	want, err := repository.EmbeddedSchemaVersion()
	if err != nil {
		return nil, err
	}
	return &schemaGate{store: store, want: want}, nil
}

// Ready returns an error while the database schema is older than the
// binary's. Once it has passed it stays passed: a schema does not go back.
func (g *schemaGate) Ready(ctx context.Context) error {
	if g.ok.Load() {
		return nil
	}
	have, err := g.store.SchemaVersion(ctx)
	if err != nil {
		return err
	}
	if have < g.want {
		return fmt.Errorf("database schema version %d is older than this binary's %d; waiting for the writer to migrate", have, g.want)
	}
	g.ok.Store(true)
	return nil
}
