package command_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

// gatedBus is an inner Bus whose Submit blocks until gate is closed, the way
// RedisBus blocks on a slow LPUSH.
type gatedBus struct {
	gate chan struct{}

	mu      sync.Mutex
	got     []string
	ctxErrs []error
	flushes int
	closed  bool
}

func newGatedBus() *gatedBus { return &gatedBus{gate: make(chan struct{})} }

func (b *gatedBus) Start(context.Context) {}

func (b *gatedBus) Submit(ctx context.Context, c command.Command) time.Duration {
	<-b.gate
	b.mu.Lock()
	defer b.mu.Unlock()
	b.got = append(b.got, c.Kind())
	b.ctxErrs = append(b.ctxErrs, ctx.Err())
	return 0
}

func (b *gatedBus) Flush(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushes++
	return nil
}

func (b *gatedBus) Close(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return nil
}

func (b *gatedBus) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.got)
}

func noop(kind string) command.Command {
	return fn{kind, func(context.Context) error { return nil }}
}

func newAsync(t *testing.T, inner command.Bus, buf, workers int) *command.Async {
	t.Helper()
	a := command.NewAsync(inner, command.AsyncOptions{Buffer: buf, Workers: workers})
	a.Start(context.Background())
	return a
}

func TestAsyncSubmitDoesNotWaitForTheInnerBus(t *testing.T) {
	inner := newGatedBus()
	a := newAsync(t, inner, 8, 2)

	start := time.Now()
	waited := a.Submit(context.Background(), noop("eval"))
	require.Less(t, time.Since(start), 100*time.Millisecond, "Submit waited on the blocked inner bus")
	require.Less(t, waited, 100*time.Millisecond)
	require.Equal(t, 0, inner.count(), "the inner bus is still blocked")

	close(inner.gate)
	require.NoError(t, a.Flush(context.Background()))
	require.Equal(t, []string{"eval"}, inner.got)
	require.Equal(t, 1, inner.flushes, "Flush flushes the inner bus too")
}

func TestAsyncBackpressureBlocksAndNeverDrops(t *testing.T) {
	inner := newGatedBus()
	a := newAsync(t, inner, 2, 1)

	// One command on the worker, two in the buffer: the fourth must block.
	for i := 0; i < 3; i++ {
		a.Submit(context.Background(), noop("eval"))
	}
	fourth := make(chan time.Duration)
	go func() { fourth <- a.Submit(context.Background(), noop("eval")) }()
	select {
	case <-fourth:
		t.Fatal("Submit returned while the buffer was full")
	case <-time.After(50 * time.Millisecond):
	}

	close(inner.gate)
	require.GreaterOrEqual(t, <-fourth, 50*time.Millisecond, "the blocked wait is reported")
	require.NoError(t, a.Flush(context.Background()))
	require.Equal(t, 4, inner.count(), "every command reached the inner bus")
}

func TestAsyncFlushWaitsForEveryWorker(t *testing.T) {
	inner := newGatedBus()
	a := newAsync(t, inner, 16, 4)
	for i := 0; i < 10; i++ {
		a.Submit(context.Background(), noop("eval"))
	}

	flushed := make(chan error)
	go func() { flushed <- a.Flush(context.Background()) }()
	select {
	case <-flushed:
		t.Fatal("Flush returned while commands were still with the workers")
	case <-time.After(50 * time.Millisecond):
	}

	close(inner.gate)
	require.NoError(t, <-flushed)
	require.Equal(t, 10, inner.count())
}

func TestAsyncFlushHonoursContext(t *testing.T) {
	inner := newGatedBus()
	a := newAsync(t, inner, 4, 1)
	a.Submit(context.Background(), noop("eval"))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, a.Flush(ctx), context.DeadlineExceeded)

	// A cancelled flush leaves the workers free to go on.
	close(inner.gate)
	require.NoError(t, a.Flush(context.Background()))
	require.Equal(t, 1, inner.count())
}

func TestAsyncCloseDrainsAndLeavesTheInnerBusOpen(t *testing.T) {
	inner := newGatedBus()
	close(inner.gate)
	a := newAsync(t, inner, 8, 2)
	for i := 0; i < 5; i++ {
		a.Submit(context.Background(), noop("eval"))
	}

	require.NoError(t, a.Close(context.Background()))
	require.Equal(t, 5, inner.count(), "Close hands every queued command to the inner bus")
	require.False(t, inner.closed, "the inner bus belongs to its owner")

	a.Submit(context.Background(), noop("late"))
	require.Equal(t, 5, inner.count(), "a submit after Close is not applied")
	require.Error(t, a.Flush(context.Background()))
}

func TestAsyncCloseHonoursContext(t *testing.T) {
	inner := newGatedBus()
	a := newAsync(t, inner, 4, 1)
	a.Submit(context.Background(), noop("eval"))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, a.Close(ctx), context.DeadlineExceeded)
	close(inner.gate)
	require.NoError(t, a.Close(context.Background()))
	require.Equal(t, 1, inner.count())
}

func TestAsyncInnerSubmitOutlivesTheRequestContext(t *testing.T) {
	inner := newGatedBus()
	a := newAsync(t, inner, 4, 1)

	ctx, cancel := context.WithCancel(context.Background())
	a.Submit(ctx, noop("eval"))
	cancel()

	close(inner.gate)
	require.NoError(t, a.Flush(context.Background()))
	require.Equal(t, []error{nil}, inner.ctxErrs, "a client that disconnects does not cancel its row")
}
