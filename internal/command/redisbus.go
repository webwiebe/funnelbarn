package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// DefaultPublishTimeout bounds one Queue.Publish when RedisOptions.PublishTimeout is zero.
	DefaultPublishTimeout = 2 * time.Second

	receiveBackoffMin = 100 * time.Millisecond
	receiveBackoffMax = 5 * time.Second
	drainPollInterval = 5 * time.Millisecond
	ackTimeout        = 5 * time.Second
	ackAttempts       = 3
	ackRetryDelay     = 100 * time.Millisecond
	// DefaultPublishCooldown is how long Submit skips Redis after a failed
	// publish when RedisOptions.PublishCooldown is zero, so an outage that times
	// out instead of refusing connections does not add PublishTimeout to every
	// request.
	DefaultPublishCooldown = 5 * time.Second
)

// RedisOptions configures a RedisBus.
type RedisOptions struct {
	// Queue is the durable transport.
	Queue Queue
	// Deps is what the consumer applies commands against. Unused when Consume is false.
	Deps Deps
	// Fallback takes a command whose publish failed, so nothing is dropped
	// while Redis is down. Normally an in-process Dispatcher.
	Fallback Bus
	// Consume starts the consumer in Start. Only the writer process sets it.
	Consume bool
	// Logger receives failures. Nil means slog.Default().
	Logger *slog.Logger
	// PublishTimeout bounds one publish. Zero means DefaultPublishTimeout.
	PublishTimeout time.Duration
	// PublishCooldown is how long commands go straight to the fallback after a
	// failed publish. Zero means DefaultPublishCooldown.
	PublishCooldown time.Duration
}

// RedisBus publishes commands to a Queue and, when Consume is set, applies
// what it receives one at a time on a single goroutine. Delivery is
// at-least-once: a command applied but not yet acked when the consumer dies is
// applied again after Recover, which is safe because every command is
// idempotent.
type RedisBus struct {
	queue    Queue
	deps     Deps
	fallback Bus
	consume  bool
	logger   *slog.Logger
	tracer   trace.Tracer
	timeout  time.Duration
	cooldown time.Duration

	publishFailing atomic.Bool
	lastFailure    atomic.Int64 // unix nanos of the last failed publish
	inflight       atomic.Int64

	mu      sync.Mutex
	closed  bool
	started bool
	senders sync.WaitGroup
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewRedisBus builds a RedisBus. Call Start before Submit.
func NewRedisBus(opts RedisOptions) *RedisBus {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := opts.PublishTimeout
	if timeout <= 0 {
		timeout = DefaultPublishTimeout
	}
	cooldown := opts.PublishCooldown
	if cooldown <= 0 {
		cooldown = DefaultPublishCooldown
	}
	return &RedisBus{
		queue:    opts.Queue,
		deps:     opts.Deps,
		fallback: opts.Fallback,
		consume:  opts.Consume,
		logger:   logger,
		tracer:   otel.Tracer("funnelbarn/command"),
		timeout:  timeout,
		cooldown: cooldown,
		done:     make(chan struct{}),
	}
}

var _ Bus = (*RedisBus)(nil)

// Start recovers anything a dead consumer left in the processing list, then
// launches the consumer goroutine. Without Consume it does nothing. The
// context does not stop the consumer: Close does, after draining. Calling
// Start twice is a no-op.
func (b *RedisBus) Start(ctx context.Context) {
	if !b.consume {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started || b.closed {
		return
	}
	b.started = true

	n, err := b.queue.Recover(ctx)
	switch {
	case err != nil:
		b.logger.ErrorContext(ctx, "command queue recover failed", "error", err, "handled", true)
	case n > 0:
		b.logger.InfoContext(ctx, "recovered unacknowledged commands", "count", n)
	}

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	b.cancel = cancel
	go b.run(runCtx)
}

// Submit publishes c and returns how long that waited. The publish ignores
// ctx cancellation so a client that disconnects does not lose its row. When
// the publish fails, c goes to the fallback, so a command is never dropped.
// After Close, Submit applies nothing and logs at Error, like Dispatcher.
func (b *RedisBus) Submit(ctx context.Context, c Command) time.Duration {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		b.logger.ErrorContext(ctx, "command submitted after close, not applied", "kind", c.Kind(), "handled", true)
		return 0
	}
	b.senders.Add(1)
	b.mu.Unlock()
	defer b.senders.Done()

	start := time.Now()
	b.publish(ctx, c)
	waited := time.Since(start)

	submitWait.Observe(waited.Seconds())
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Float64("command.submit_wait_ms", float64(waited)/float64(time.Millisecond)))
	span.AddEvent("command.submitted", trace.WithAttributes(
		attribute.String("command.kind", c.Kind()),
		attribute.Float64("command.submit_wait_ms", float64(waited)/float64(time.Millisecond)),
	))
	return waited
}

func (b *RedisBus) publish(ctx context.Context, c Command) {
	// Fix the evaluation id before the publish: when Redis takes the LPUSH but
	// the reply times out, the fallback applies the same row as the consumer.
	if prepared, err := withID(c); err == nil {
		c = prepared
	}
	if b.publishFailing.Load() && time.Since(time.Unix(0, b.lastFailure.Load())) < b.cooldown {
		b.toFallback(ctx, c)
		return
	}
	payload, err := Encode(c, time.Now())
	if err != nil {
		// Commands are plain data, so this is a programming bug.
		b.logger.ErrorContext(ctx, "command encode failed, using fallback", "kind", c.Kind(), "error", err, "handled", true)
		b.toFallback(ctx, c)
		return
	}

	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), b.timeout)
	defer cancel()
	if err := b.queue.Publish(pubCtx, payload); err != nil {
		b.lastFailure.Store(time.Now().UnixNano())
		if b.publishFailing.CompareAndSwap(false, true) {
			b.logger.ErrorContext(ctx, "command publish failed, using fallback", "kind", c.Kind(), "error", err, "handled", true)
		} else {
			b.logger.WarnContext(ctx, "command publish failed, using fallback", "kind", c.Kind(), "error", err, "handled", true)
		}
		b.toFallback(ctx, c)
		return
	}
	if b.publishFailing.CompareAndSwap(true, false) {
		b.logger.InfoContext(ctx, "command publish recovered")
	}
}

func (b *RedisBus) toFallback(ctx context.Context, c Command) {
	if b.fallback == nil {
		b.logger.ErrorContext(ctx, "no fallback bus, command not applied", "kind", c.Kind(), "handled", true)
		return
	}
	b.fallback.Submit(ctx, c)
}

// run is the consumer loop.
func (b *RedisBus) run(ctx context.Context) {
	defer close(b.done)
	var (
		backoff time.Duration
		failing bool
	)
	for ctx.Err() == nil {
		payload, ack, err := b.queue.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if backoff == 0 {
				backoff = receiveBackoffMin
			} else if backoff *= 2; backoff > receiveBackoffMax {
				backoff = receiveBackoffMax
			}
			if !failing {
				b.logger.ErrorContext(ctx, "command queue receive failed", "error", err, "backoff", backoff, "handled", true)
			} else {
				b.logger.WarnContext(ctx, "command queue receive failed", "error", err, "backoff", backoff, "handled", true)
			}
			failing = true
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		if failing {
			failing = false
			b.logger.InfoContext(ctx, "command queue receive recovered")
		}
		backoff = 0
		if payload == nil {
			continue
		}

		b.inflight.Add(1)
		b.handle(ctx, payload, ack)
		b.inflight.Add(-1)
	}
}

// handle decodes, applies and acks one payload.
func (b *RedisBus) handle(ctx context.Context, payload []byte, ack func(context.Context) error) {
	c, _, decodeErr := Decode(payload)
	if decodeErr != nil {
		// It can never apply, and redelivering it would block the queue.
		b.logger.ErrorContext(ctx, "command decode failed, dropping payload", "error", decodeErr, "handled", true)
	} else {
		apply(b.tracer, b.logger, b.deps, c)
	}

	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ackTimeout)
	defer cancel()
	if err := ackWithRetry(ackCtx, ack); err != nil {
		// The command stays in the processing list, comes back after the next
		// Recover and applies again, which is safe because it is idempotent.
		b.logger.WarnContext(ctx, "command ack failed", "error", err, "handled", true)
	}

	if decodeErr == nil {
		if n, lenErr := b.queue.Len(ackCtx); lenErr == nil {
			queueDepth.Set(float64(n))
		}
	}
}

func ackWithRetry(ctx context.Context, ack func(context.Context) error) error {
	var err error
	for i := 0; i < ackAttempts; i++ {
		if err = ack(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(ackRetryDelay):
		}
	}
	return err
}

// Flush blocks until the queue is empty and nothing is in flight, after
// flushing the fallback. ctx bounds the wait. A bus that does not consume has
// nothing to wait for and returns an error.
func (b *RedisBus) Flush(ctx context.Context) error {
	if !b.consume {
		return errors.New("command bus does not consume, nothing to flush")
	}
	if b.fallback != nil {
		if err := b.fallback.Flush(ctx); err != nil {
			return err
		}
	}
	return b.waitDrained(ctx)
}

// waitDrained returns once Queue.Len is 0 and no item is being applied.
func (b *RedisBus) waitDrained(ctx context.Context) error {
	ticker := time.NewTicker(drainPollInterval)
	defer ticker.Stop()
	for {
		n, err := b.queue.Len(ctx)
		if err != nil {
			return fmt.Errorf("command queue length: %w", err)
		}
		// Len counts the processing list, so an item is in the queue until it is
		// acked; reading inflight second covers the gap after the ack.
		if n == 0 && b.inflight.Load() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Close stops accepting commands, keeps consuming until the queue is empty or
// ctx ends, stops the consumer, and closes the fallback. It returns ctx's
// error when the drain did not finish. Calling it again is safe.
func (b *RedisBus) Close(ctx context.Context) error {
	b.mu.Lock()
	b.closed = true
	started := b.started
	cancel := b.cancel
	b.mu.Unlock()

	// A publish in progress finishes before the drain starts.
	b.senders.Wait()

	var drainErr error
	if started {
		drainErr = b.waitDrained(ctx)
		cancel()
		if drainErr == nil {
			select {
			case <-b.done:
			case <-ctx.Done():
				drainErr = ctx.Err()
			}
		}
	}

	var fallbackErr error
	if b.fallback != nil {
		fallbackErr = b.fallback.Close(ctx)
	}
	if drainErr != nil {
		return drainErr
	}
	return fallbackErr
}
