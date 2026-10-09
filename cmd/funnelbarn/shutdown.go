package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

// waitServer blocks until the server fails or ctx is cancelled, then shuts it
// down gracefully.
func waitServer(ctx context.Context, server *http.Server, errCh <-chan error) error {
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			// Handlers still running would submit commands after the
			// dispatcher closes and lose them; cut their connections off.
			_ = server.Close()
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// drainIngestQueue stores what the ingest queue holds before the store
// closes, giving up after 10s. What is left stays in Valkey for the next
// consumer.
func drainIngestQueue(q *ingestQueue) {
	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := q.Close(closeCtx); err != nil {
		slog.Warn("ingest queue: drain on shutdown", "err", err, "handled", true)
	}
}

// drainCommands applies every queued command before the store closes, giving
// up after 10s.
func drainCommands(commands command.Bus) {
	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := commands.Close(closeCtx); err != nil {
		slog.Warn("command dispatcher: drain on shutdown", "err", err, "handled", true)
	}
}
