package command_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

// faultyQueue wraps fakeQueue and fails a set number of calls per method.
type faultyQueue struct {
	*fakeQueue
	mu          sync.Mutex
	receiveErrs int
	ackErrs     int
	recoverErr  bool
	lenErr      bool
}

func (q *faultyQueue) take(n *int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if *n > 0 {
		*n--
		return true
	}
	return false
}

func (q *faultyQueue) Receive(ctx context.Context) ([]byte, func(context.Context) error, error) {
	if q.take(&q.receiveErrs) {
		return nil, nil, errors.New("connection refused")
	}
	payload, ack, err := q.fakeQueue.Receive(ctx)
	if ack == nil {
		return payload, ack, err
	}
	return payload, func(ctx context.Context) error {
		if q.take(&q.ackErrs) {
			return errors.New("lrem timeout")
		}
		return ack(ctx)
	}, err
}

func (q *faultyQueue) Recover(ctx context.Context) (int, error) {
	if q.recoverErr {
		return 0, errors.New("lmove refused")
	}
	return q.fakeQueue.Recover(ctx)
}

func (q *faultyQueue) Len(ctx context.Context) (int64, error) {
	q.mu.Lock()
	failing := q.lenErr
	q.mu.Unlock()
	if failing {
		return 0, errors.New("llen refused")
	}
	return q.fakeQueue.Len(ctx)
}

func newFaultyBus(t *testing.T, q *faultyQueue, fallback command.Bus) (*command.RedisBus, *recordingStore, *logCapture) {
	t.Helper()
	store, logs := &recordingStore{}, &logCapture{}
	bus := command.NewRedisBus(command.RedisOptions{
		Queue:    q,
		Deps:     command.Deps{Store: store},
		Fallback: fallback,
		Consume:  true,
		Logger:   logs.logger(),
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = bus.Close(ctx)
	})
	return bus, store, logs
}

func TestRedisBusReceiveErrorsBackOffAndRecover(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue(), receiveErrs: 3}
	bus, store, logs := newFaultyBus(t, q, nil)
	ctx := testCtx(t)
	bus.Start(ctx)

	bus.Submit(ctx, command.TouchAPIKey{KeyHash: "k"})
	require.NoError(t, bus.Flush(ctx))

	require.Equal(t, []string{"k"}, store.appliedKeys())
	require.Equal(t, 1, logs.count("ERROR", "receive failed"))
	require.Equal(t, 2, logs.count("WARN", "receive failed"))
	require.Equal(t, 1, logs.count("INFO", "receive recovered"))
}

func TestRedisBusCloseDuringReceiveBackoff(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue(), receiveErrs: 1000}
	bus, _, logs := newFaultyBus(t, q, nil)
	ctx := testCtx(t)
	bus.Start(ctx)
	require.Eventually(t, func() bool { return logs.count("ERROR", "receive failed") == 1 },
		5*time.Second, 5*time.Millisecond)

	require.NoError(t, bus.Close(ctx))
}

func TestRedisBusAckFailureIsRetried(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue(), ackErrs: 2}
	bus, store, logs := newFaultyBus(t, q, nil)
	ctx := testCtx(t)
	bus.Start(ctx)

	bus.Submit(ctx, command.TouchAPIKey{KeyHash: "k"})
	require.NoError(t, bus.Flush(ctx))
	require.Equal(t, []string{"k"}, store.appliedKeys())
	require.Zero(t, logs.count("WARN", "ack failed"))
}

func TestRedisBusAckFailureLeavesItemForRecover(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue(), ackErrs: 3}
	bus, store, logs := newFaultyBus(t, q, nil)
	ctx := testCtx(t)
	bus.Start(ctx)

	bus.Submit(ctx, command.TouchAPIKey{KeyHash: "k"})
	require.Eventually(t, func() bool { return logs.count("WARN", "ack failed") == 1 },
		5*time.Second, 5*time.Millisecond)
	require.Equal(t, []string{"k"}, store.appliedKeys())

	n, err := q.Len(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "an unacked item stays in the processing list")

	// Recover redelivers it and the idempotent command applies again.
	_, err = q.Recover(ctx)
	require.NoError(t, err)
	require.NoError(t, bus.Flush(ctx))
	require.Equal(t, []string{"k", "k"}, store.appliedKeys())
}

func TestRedisBusRecoverErrorIsLoggedAndConsumes(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue(), recoverErr: true}
	bus, store, logs := newFaultyBus(t, q, nil)
	ctx := testCtx(t)
	bus.Start(ctx)
	bus.Start(ctx) // second Start is a no-op

	bus.Submit(ctx, command.TouchAPIKey{KeyHash: "k"})
	require.NoError(t, bus.Flush(ctx))
	require.Equal(t, []string{"k"}, store.appliedKeys())
	require.Equal(t, 1, logs.count("ERROR", "recover failed"))
}

func TestRedisBusFlushReportsLenError(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue(), lenErr: true}
	bus, _, _ := newFaultyBus(t, q, nil)
	ctx := testCtx(t)
	bus.Start(ctx)
	require.ErrorContains(t, bus.Flush(ctx), "llen refused")
	q.mu.Lock()
	q.lenErr = false
	q.mu.Unlock()
}

func TestRedisBusFlushTimesOut(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue()}
	q.leaveUnacked([]byte(`{}`)) // never drains: no Recover runs without Start
	bus := command.NewRedisBus(command.RedisOptions{Queue: q, Consume: true, Logger: (&logCapture{}).logger()})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, bus.Flush(ctx), context.DeadlineExceeded)
}

func TestRedisBusWithoutFallbackDropsOnPublishFailure(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue()}
	q.setFailPublish(true)
	bus, store, logs := newFaultyBus(t, q, nil)
	ctx := testCtx(t)
	bus.Start(ctx)

	bus.Submit(ctx, command.TouchAPIKey{KeyHash: "k"})
	require.Equal(t, 1, logs.count("ERROR", "no fallback bus"))
	require.Empty(t, store.appliedKeys())
}

// failingBus is a fallback whose Flush and Close fail.
type failingBus struct{ command.Bus }

func (failingBus) Flush(context.Context) error { return errors.New("fallback flush failed") }
func (failingBus) Close(context.Context) error { return errors.New("fallback close failed") }

func TestRedisBusFallbackErrorsPropagate(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue()}
	bus, _, _ := newFaultyBus(t, q, failingBus{})
	ctx := testCtx(t)
	bus.Start(ctx)
	require.ErrorContains(t, bus.Flush(ctx), "fallback flush failed")
	require.ErrorContains(t, bus.Close(ctx), "fallback close failed")
}

func TestRedisBusCloseWithoutStart(t *testing.T) {
	q := &faultyQueue{fakeQueue: newFakeQueue()}
	bus, _, _ := newFaultyBus(t, q, nil)
	require.NoError(t, bus.Close(testCtx(t)))
	bus.Start(testCtx(t)) // Start after Close does nothing
}
