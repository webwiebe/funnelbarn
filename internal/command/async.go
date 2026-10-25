package command

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// DefaultAsyncWorkers is the worker count used when AsyncOptions.Workers is zero.
const DefaultAsyncWorkers = 4

// AsyncOptions configures an Async.
type AsyncOptions struct {
	// Buffer is the channel capacity. Zero means DefaultBuffer.
	Buffer int
	// Workers is how many goroutines hand commands to the inner bus. Zero
	// means DefaultAsyncWorkers.
	Workers int
	// Logger receives submits after Close. Nil means slog.Default().
	Logger *slog.Logger
}

// asyncItem is one queue entry: a command, or one worker's share of a flush.
type asyncItem struct {
	ctx   context.Context
	cmd   Command
	flush *asyncFlush
}

// asyncFlush is a barrier: every worker takes one marker, and none goes on
// until all have, so each has finished the commands it took before its marker.
type asyncFlush struct {
	remaining atomic.Int32
	allTaken  chan struct{}
	release   chan struct{}
}

// Async hands commands to an inner Bus from a fixed pool of workers, so the
// submitter waits for a buffer slot and not for the inner Submit. RedisBus
// publishes synchronously, and an LPUSH to the shared Valkey takes 60 to 200+
// ms at times (#337, FUN-48); behind Async that wait leaves the request path.
//
// It never drops a command: a full buffer blocks the submitter, as the
// Dispatcher does. Commands reach the inner bus out of order when Workers is
// above 1, so only commands that do not depend on each other belong here.
// Close drains into the inner bus and leaves the inner bus open; its owner
// closes it afterwards.
type Async struct {
	inner   Bus
	ch      chan asyncItem
	workers int
	logger  *slog.Logger

	mu      sync.Mutex
	closed  bool
	started bool
	senders sync.WaitGroup
	running sync.WaitGroup
	done    chan struct{}
}

// NewAsync builds an Async in front of inner. Call Start before Submit.
func NewAsync(inner Bus, opts AsyncOptions) *Async {
	size := opts.Buffer
	if size <= 0 {
		size = DefaultBuffer
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = DefaultAsyncWorkers
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Async{
		inner:   inner,
		ch:      make(chan asyncItem, size),
		workers: workers,
		logger:  logger,
		done:    make(chan struct{}),
	}
}

// Start launches the workers. It does not start the inner bus. Calling Start
// twice is a no-op.
func (a *Async) Start(_ context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return
	}
	a.started = true
	a.running.Add(a.workers)
	for i := 0; i < a.workers; i++ {
		go a.run()
	}
	go func() {
		a.running.Wait()
		close(a.done)
	}()
}

// Submit enqueues c and returns how long the enqueue waited. The inner Submit
// runs later on a worker with ctx's values and without its cancellation or
// span, so a client that disconnects does not lose its row and the request
// span keeps the wait it actually had.
func (a *Async) Submit(ctx context.Context, c Command) time.Duration {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		a.logger.ErrorContext(ctx, "command submitted after close, not applied", "kind", c.Kind(), "handled", true)
		return 0
	}
	a.senders.Add(1)
	a.mu.Unlock()
	defer a.senders.Done()

	detached := trace.ContextWithSpan(context.WithoutCancel(ctx), trace.SpanFromContext(context.Background()))
	start := time.Now()
	a.ch <- asyncItem{ctx: detached, cmd: c}
	waited := time.Since(start)

	trace.SpanFromContext(ctx).SetAttributes(
		attribute.Float64("command.submit_wait_ms", float64(waited)/float64(time.Millisecond)))
	return waited
}

// Flush blocks until every command submitted before the call has been handed
// to the inner bus, then flushes the inner bus. ctx bounds the whole wait.
func (a *Async) Flush(ctx context.Context) error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return fmt.Errorf("async command bus closed")
	}
	a.senders.Add(1)
	a.mu.Unlock()

	f := &asyncFlush{allTaken: make(chan struct{}), release: make(chan struct{})}
	f.remaining.Store(int32(a.workers))
	defer close(f.release)
	for i := 0; i < a.workers; i++ {
		select {
		case a.ch <- asyncItem{flush: f}:
		case <-ctx.Done():
			a.senders.Done()
			return ctx.Err()
		}
	}
	a.senders.Done()
	select {
	case <-f.allTaken:
	case <-ctx.Done():
		return ctx.Err()
	}
	return a.inner.Flush(ctx)
}

// Close stops accepting commands and hands everything queued to the inner bus,
// returning ctx's error if that takes longer than ctx allows. It does not
// close the inner bus.
func (a *Async) Close(ctx context.Context) error {
	a.mu.Lock()
	if !a.closed {
		a.closed = true
		go func() {
			a.senders.Wait()
			close(a.ch)
		}()
	}
	started := a.started
	a.mu.Unlock()
	if !started {
		return nil
	}
	select {
	case <-a.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Async) run() {
	defer a.running.Done()
	for it := range a.ch {
		if it.flush != nil {
			if it.flush.remaining.Add(-1) == 0 {
				close(it.flush.allTaken)
			}
			<-it.flush.release
			continue
		}
		a.inner.Submit(it.ctx, it.cmd)
	}
}
