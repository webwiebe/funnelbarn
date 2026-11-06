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

// stepClock moves only when the test steps it. Every read is announced on
// reads, so a test can wait until a Submit has taken its start time.
type stepClock struct {
	mu    sync.Mutex
	t     time.Time
	reads chan struct{}
}

func newStepClock() *stepClock {
	return &stepClock{t: time.Unix(0, 0), reads: make(chan struct{}, 64)}
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	t := c.t
	c.mu.Unlock()
	c.reads <- struct{}{}
	return t
}

func (c *stepClock) Step(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newClockedAsync(t *testing.T, inner command.Bus, buf, workers int, clock *stepClock) *command.Async {
	t.Helper()
	a := command.NewAsync(inner, command.AsyncOptions{Buffer: buf, Workers: workers, Now: clock.Now})
	a.Start(context.Background())
	return a
}

func TestAsyncSubmitDoesNotWaitForTheInnerBus(t *testing.T) {
	inner := newGatedBus()
	clock := newStepClock()
	a := newClockedAsync(t, inner, 8, 2, clock)

	// The inner bus stays blocked until close(inner.gate), so Submit returning
	// here at all shows it did not wait for it.
	waited := a.Submit(context.Background(), noop("eval"))
	require.Zero(t, waited, "a free buffer slot reports no wait")
	require.Equal(t, 0, inner.count(), "the inner bus is still blocked")

	close(inner.gate)
	require.NoError(t, a.Flush(context.Background()))
	require.Equal(t, []string{"eval"}, inner.got)
	require.Equal(t, 1, inner.flushes, "Flush flushes the inner bus too")
}

func TestAsyncBackpressureBlocksAndNeverDrops(t *testing.T) {
	inner := newGatedBus()
	clock := newStepClock()
	a := newClockedAsync(t, inner, 2, 1, clock)

	// Three Submits returned with a buffer of 2, so the one worker holds a
	// command, blocked on the gate, and the buffer is full: the fourth blocks.
	for i := 0; i < 3; i++ {
		a.Submit(context.Background(), noop("eval"))
	}
	for i := 0; i < 6; i++ { // a start and an end read per Submit
		<-clock.reads
	}
	fourth := make(chan time.Duration, 1)
	go func() { fourth <- a.Submit(context.Background(), noop("eval")) }()
	<-clock.reads // the fourth Submit has taken its start time

	// Its end read needs a free slot, which needs the gate open, so the step
	// lands inside the blocked wait every time.
	clock.Step(50 * time.Millisecond)
	select {
	case <-fourth:
		t.Fatal("Submit returned while the buffer was full")
	default:
	}

	close(inner.gate)
	require.Equal(t, 50*time.Millisecond, <-fourth, "the blocked wait is reported")
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
