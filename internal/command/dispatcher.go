// Package command holds the async command dispatcher and the typed
// bookkeeping commands that run on the write pool off the request path.
package command

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// DefaultBuffer is the queue size used when Options.Buffer is zero.
const DefaultBuffer = 1024

// Options configures a Dispatcher.
type Options struct {
	// Buffer is the channel capacity. Zero means DefaultBuffer.
	Buffer int
	// Logger receives Apply failures. Nil means slog.Default().
	Logger *slog.Logger
	// Deps is what the consumer applies commands against.
	Deps Deps
}

// item is one queue entry: a command, or a flush marker.
type item struct {
	cmd   Command
	flush chan struct{}
}

// Dispatcher applies commands one at a time, in submission order, on a single
// consumer goroutine. It never drops a command: a full buffer blocks the
// submitter.
type Dispatcher struct {
	ch     chan item
	logger *slog.Logger
	tracer trace.Tracer
	deps   Deps

	mu      sync.Mutex
	closed  bool
	started bool
	senders sync.WaitGroup
	done    chan struct{}
}

// New builds a Dispatcher. Call Start before Submit.
func New(opts Options) *Dispatcher {
	size := opts.Buffer
	if size <= 0 {
		size = DefaultBuffer
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		ch:     make(chan item, size),
		logger: logger,
		tracer: otel.Tracer("funnelbarn/command"),
		deps:   opts.Deps,
		done:   make(chan struct{}),
	}
}

// Start launches the consumer goroutine. The context does not stop it: Close
// does, after draining. Calling Start twice is a no-op.
func (d *Dispatcher) Start(_ context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return
	}
	d.started = true
	go d.run()
}

// Submit enqueues c and returns how long the enqueue waited. It blocks while
// the buffer is full, and the wait ignores ctx cancellation so a client that
// disconnects does not lose its row. ctx is only read for the span that gets
// the command.submit_wait_ms attribute. After Close, Submit applies nothing
// and logs at Error: the row is lost, which only happens when a request
// outlives the HTTP server's shutdown.
func (d *Dispatcher) Submit(ctx context.Context, c Command) time.Duration {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		d.logger.ErrorContext(ctx, "command submitted after close, not applied", "kind", c.Kind(), "handled", true)
		return 0
	}
	d.senders.Add(1)
	d.mu.Unlock()
	defer d.senders.Done()

	start := time.Now()
	d.ch <- item{cmd: c}
	waited := time.Since(start)

	queueDepth.Set(float64(len(d.ch)))
	submitWait.Observe(waited.Seconds())
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Float64("command.submit_wait_ms", float64(waited)/float64(time.Millisecond)))
	span.AddEvent("command.submitted", trace.WithAttributes(
		attribute.String("command.kind", c.Kind()),
		attribute.Float64("command.submit_wait_ms", float64(waited)/float64(time.Millisecond)),
	))
	return waited
}

// Flush blocks until every command submitted before the call has been applied.
// ctx bounds both the enqueue of the marker and the wait.
func (d *Dispatcher) Flush(ctx context.Context) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return fmt.Errorf("command dispatcher closed")
	}
	d.senders.Add(1)
	d.mu.Unlock()

	marker := item{flush: make(chan struct{})}
	select {
	case d.ch <- marker:
		d.senders.Done()
	case <-ctx.Done():
		d.senders.Done()
		return ctx.Err()
	}
	select {
	case <-marker.flush:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops accepting commands, applies everything already queued, and
// returns ctx's error if that takes longer than ctx allows.
func (d *Dispatcher) Close(ctx context.Context) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return d.waitDone(ctx)
	}
	d.closed = true
	started := d.started
	d.mu.Unlock()

	go func() {
		d.senders.Wait()
		close(d.ch)
	}()
	if !started {
		return nil
	}
	return d.waitDone(ctx)
}

func (d *Dispatcher) waitDone(ctx context.Context) error {
	select {
	case <-d.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Dispatcher) run() {
	defer close(d.done)
	for it := range d.ch {
		queueDepth.Set(float64(len(d.ch)))
		if it.flush != nil {
			close(it.flush)
			continue
		}
		_ = apply(d.tracer, d.logger, d.deps, it.cmd)
	}
	queueDepth.Set(0)
}

// apply runs one command under a command.apply span. A failure is logged,
// counted and returned. Bookkeeping commands are not retried, because each one
// is idempotent bookkeeping and a retry loop would hold up the commands behind
// it; RedisBus retries a durable command (see Durable).
func apply(tracer trace.Tracer, logger *slog.Logger, deps Deps, c Command) error {
	kind := c.Kind()
	ctx, span := tracer.Start(context.Background(), "command.apply",
		trace.WithAttributes(attribute.String("command.kind", kind)))
	defer span.End()

	err := safeApply(ctx, c, deps)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		applied.WithLabelValues(kind, "error").Inc()
		logger.Warn("command apply failed", "kind", kind, "error", err, "handled", true)
		return err
	}
	applied.WithLabelValues(kind, "ok").Inc()
	return nil
}

func safeApply(ctx context.Context, c Command, deps Deps) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic applying command: %v", r)
		}
	}()
	return c.Apply(ctx, deps)
}
