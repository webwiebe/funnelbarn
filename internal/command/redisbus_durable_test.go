package command_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// ingestEnv is a consuming RedisBus whose ApplyIngest fails a set number of
// times per ingest id, and which records what it dead-letters.
type ingestEnv struct {
	queue *fakeQueue
	bus   *command.RedisBus

	mu       sync.Mutex
	failures map[string]int // ingest id -> failures left
	failWith error
	attempts map[string]int
	stored   []string
	dead     []string
	deadErr  []error
}

func newIngestEnv(t *testing.T, retryDelay time.Duration) *ingestEnv {
	t.Helper()
	e := &ingestEnv{
		queue:    newFakeQueue(),
		failures: map[string]int{},
		failWith: errors.New("database is locked"),
		attempts: map[string]int{},
	}
	apply := func(_ context.Context, rec spool.Record) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.attempts[rec.IngestID]++
		if e.failures[rec.IngestID] != 0 {
			e.failures[rec.IngestID]--
			return fmt.Errorf("persist %s: %w", rec.IngestID, e.failWith)
		}
		e.stored = append(e.stored, rec.IngestID)
		return nil
	}
	e.bus = command.NewRedisBus(command.RedisOptions{
		Queue:      e.queue,
		Deps:       command.Deps{ApplyIngest: apply},
		Consume:    true,
		Logger:     (&logCapture{}).logger(),
		RetryDelay: retryDelay,
		DeadLetter: func(_ context.Context, c command.Command, err error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.dead = append(e.dead, c.(command.IngestRecord).Record.IngestID)
			e.deadErr = append(e.deadErr, err)
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = e.bus.Close(ctx)
	})
	return e
}

func (e *ingestEnv) publish(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		b, err := command.Encode(command.IngestRecord{Record: spool.Record{IngestID: id}}, time.Now())
		require.NoError(t, err)
		require.NoError(t, e.queue.Publish(context.Background(), b))
	}
}

func (e *ingestEnv) snapshot() (stored, dead []string, attempts map[string]int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	a := make(map[string]int, len(e.attempts))
	for k, v := range e.attempts {
		a[k] = v
	}
	return append([]string(nil), e.stored...), append([]string(nil), e.dead...), a
}

// A transient failure is retried in place, so the record is stored and the
// records behind it keep their order.
func TestRedisBusRetriesDurableCommand(t *testing.T) {
	e := newIngestEnv(t, time.Millisecond)
	e.failures["a"] = 2
	ctx := testCtx(t)
	e.bus.Start(ctx)
	e.publish(t, "a", "b")
	require.NoError(t, e.bus.Flush(ctx))

	stored, dead, attempts := e.snapshot()
	require.Equal(t, []string{"a", "b"}, stored)
	require.Empty(t, dead)
	require.Equal(t, 3, attempts["a"])
}

// A failure that outlasts the attempts is dead-lettered once, acked, and the
// queue moves on.
func TestRedisBusDeadLettersAfterMaxAttempts(t *testing.T) {
	e := newIngestEnv(t, time.Millisecond)
	e.failures["a"] = 100
	ctx := testCtx(t)
	e.bus.Start(ctx)
	e.publish(t, "a", "b")
	require.NoError(t, e.bus.Flush(ctx))

	stored, dead, attempts := e.snapshot()
	require.Equal(t, []string{"b"}, stored)
	require.Equal(t, []string{"a"}, dead)
	require.Equal(t, command.DefaultMaxAttempts, attempts["a"])
	n, err := e.queue.Len(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}

// ErrPermanent skips the retries: an apply that can never succeed is
// dead-lettered on the first failure.
func TestRedisBusDeadLettersPermanentFailureAtOnce(t *testing.T) {
	e := newIngestEnv(t, time.Hour)
	e.failures["a"] = 1
	e.failWith = command.ErrPermanent
	ctx := testCtx(t)
	e.bus.Start(ctx)
	e.publish(t, "a")
	require.NoError(t, e.bus.Flush(ctx))

	_, dead, attempts := e.snapshot()
	require.Equal(t, []string{"a"}, dead)
	require.Equal(t, 1, attempts["a"])
	require.ErrorIs(t, e.deadErr[0], command.ErrPermanent)
}

// Stopping the consumer mid-retry leaves the record unacked, so the next
// consumer's Recover hands it over again. It is neither stored nor
// dead-lettered.
func TestRedisBusCloseMidRetryLeavesRecordQueued(t *testing.T) {
	e := newIngestEnv(t, time.Hour)
	e.failures["a"] = 1
	ctx := testCtx(t)
	e.bus.Start(ctx)
	e.publish(t, "a")
	require.Eventually(t, func() bool {
		_, _, attempts := e.snapshot()
		return attempts["a"] == 1
	}, 5*time.Second, time.Millisecond)

	closeCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, e.bus.Close(closeCtx), context.DeadlineExceeded)

	stored, dead, _ := e.snapshot()
	require.Empty(t, stored)
	require.Empty(t, dead)
	n, err := e.queue.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}

// failingTouchStore fails every TouchAPIKey and counts the calls.
type failingTouchStore struct {
	recordingStore
	calls int
}

func (s *failingTouchStore) TouchAPIKey(context.Context, string) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return errors.New("database is locked")
}

// Bookkeeping commands keep their old behaviour: one attempt, then dropped.
func TestRedisBusDoesNotRetryBookkeeping(t *testing.T) {
	store := &failingTouchStore{}
	q := newFakeQueue()
	bus := command.NewRedisBus(command.RedisOptions{
		Queue:      q,
		Deps:       command.Deps{Store: store},
		Consume:    true,
		Logger:     (&logCapture{}).logger(),
		RetryDelay: time.Millisecond,
	})
	ctx := testCtx(t)
	bus.Start(ctx)
	defer func() { _ = bus.Close(ctx) }()

	b, err := command.Encode(command.TouchAPIKey{KeyHash: "k"}, time.Now())
	require.NoError(t, err)
	require.NoError(t, q.Publish(ctx, b))
	require.NoError(t, bus.Flush(ctx))

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Equal(t, 1, store.calls)
}
