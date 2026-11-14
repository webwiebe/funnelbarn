package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

// batchPublisher is the part of queue.RedisList a fallback forwarder uses.
type batchPublisher interface {
	PublishBatch(ctx context.Context, payloads [][]byte) error
}

// FallbackForwarder publishes what a reader buffered in a SpoolFallback while
// Valkey could not be reached.
type FallbackForwarder struct {
	name     string
	fallback *command.SpoolFallback
	list     batchPublisher
	outage   command.Outage // failed forwards; picks the log level
}

// NewFallbackForwarder returns a forwarder that publishes fallback's buffered
// commands to list. name prefixes its log messages.
func NewFallbackForwarder(name string, fallback *command.SpoolFallback, list batchPublisher) *FallbackForwarder {
	return &FallbackForwarder{name: name, fallback: fallback, list: list}
}

// Submit buffers c on disk, for publishing on a later Forward.
func (f *FallbackForwarder) Submit(ctx context.Context, c command.Command) {
	f.fallback.Submit(ctx, c)
}

// Close closes the buffer file.
func (f *FallbackForwarder) Close(ctx context.Context) error {
	return f.fallback.Close(ctx)
}

// Failing reports whether the last forwards failed.
func (f *FallbackForwarder) Failing() bool { return f.outage.Failing() }

// Forward runs on the reader's worker tick.
func (f *FallbackForwarder) Forward(ctx context.Context) {
	pubCtx, cancel := context.WithTimeout(ctx, command.DefaultPublishTimeout)
	defer cancel()
	n, err := f.fallback.Forward(pubCtx, f.list.PublishBatch)
	if err != nil {
		level, outage := f.outage.Fail()
		slog.Log(ctx, level, f.name+": forwarding buffered commands failed, keeping them on disk",
			outage, "err", err, "handled", true)
		return
	}
	if lasted, ok := f.outage.Recover(); ok {
		slog.InfoContext(ctx, f.name+": forwarding buffered commands recovered", "lasted", lasted.Round(time.Millisecond).String())
	}
	if n > 0 {
		slog.InfoContext(ctx, f.name+": forwarded buffered commands", "count", n)
	}
}
