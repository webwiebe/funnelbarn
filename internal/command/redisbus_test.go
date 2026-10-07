package command_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// fakeQueue models the reliable list: Receive moves a payload to the
// processing list and ack removes it from there.
type fakeQueue struct {
	mu          sync.Mutex
	pending     [][]byte
	processing  map[int][]byte
	nextTag     int
	failPublish bool
	// storeThenFail keeps the payload but reports an error, like an LPUSH that
	// lands while its reply times out.
	storeThenFail bool
	publishes     int
	signal        chan struct{}
}

func newFakeQueue() *fakeQueue {
	return &fakeQueue{processing: map[int][]byte{}, signal: make(chan struct{}, 1)}
}

func (q *fakeQueue) notify() {
	select {
	case q.signal <- struct{}{}:
	default:
	}
}

func (q *fakeQueue) setFailPublish(v bool) {
	q.mu.Lock()
	q.failPublish = v
	q.mu.Unlock()
}

func (q *fakeQueue) Publish(_ context.Context, payload []byte) error {
	q.mu.Lock()
	q.publishes++
	if q.failPublish {
		q.mu.Unlock()
		return errors.New("redis down")
	}
	q.pending = append(q.pending, append([]byte(nil), payload...))
	ambiguous := q.storeThenFail
	q.mu.Unlock()
	q.notify()
	if ambiguous {
		return errors.New("i/o timeout")
	}
	return nil
}

func (q *fakeQueue) Receive(ctx context.Context) ([]byte, func(context.Context) error, error) {
	timer := time.NewTimer(20 * time.Millisecond)
	defer timer.Stop()
	for {
		q.mu.Lock()
		if len(q.pending) > 0 {
			p := q.pending[0]
			q.pending = q.pending[1:]
			tag := q.nextTag
			q.nextTag++
			q.processing[tag] = p
			q.mu.Unlock()
			ack := func(context.Context) error {
				q.mu.Lock()
				delete(q.processing, tag)
				q.mu.Unlock()
				return nil
			}
			return p, ack, nil
		}
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-q.signal:
		case <-timer.C:
			return nil, nil, nil
		}
	}
}

func (q *fakeQueue) Recover(context.Context) (int, error) {
	q.mu.Lock()
	n := len(q.processing)
	var back [][]byte
	for tag := 0; tag < q.nextTag; tag++ {
		if p, ok := q.processing[tag]; ok {
			back = append(back, p)
		}
	}
	q.processing = map[int][]byte{}
	q.pending = append(back, q.pending...)
	q.mu.Unlock()
	q.notify()
	return n, nil
}

func (q *fakeQueue) Len(context.Context) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return int64(len(q.pending) + len(q.processing)), nil
}

// leaveUnacked puts payload on the processing list as a consumer that died
// between Receive and ack would have.
func (q *fakeQueue) leaveUnacked(payload []byte) {
	q.mu.Lock()
	q.processing[q.nextTag] = payload
	q.nextTag++
	q.mu.Unlock()
}

func (q *fakeQueue) pendingPayloads() [][]byte {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([][]byte(nil), q.pending...)
}

// recordingStore is a Store that records what is applied to it.
type recordingStore struct {
	mu    sync.Mutex
	keys  []string
	evals []repository.FlagEvaluation
}

func (s *recordingStore) RecordEvaluation(_ context.Context, e repository.FlagEvaluation) error {
	s.mu.Lock()
	s.evals = append(s.evals, e)
	s.mu.Unlock()
	return nil
}

func (s *recordingStore) TouchAPIKey(_ context.Context, hash string) error {
	s.mu.Lock()
	s.keys = append(s.keys, hash)
	s.mu.Unlock()
	return nil
}

func (s *recordingStore) FlagByKey(context.Context, string, string) (repository.FeatureFlag, error) {
	return repository.FeatureFlag{}, nil
}
func (s *recordingStore) TouchFlagEvaluated(context.Context, string) error { return nil }
func (s *recordingStore) EnsureAutoFlag(_ context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error) {
	return f, nil
}
func (s *recordingStore) CountAutoFlags(context.Context, string) (int, error) { return 0, nil }

func (s *recordingStore) appliedKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.keys...)
}

func (s *recordingStore) appliedEvals() []repository.FlagEvaluation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]repository.FlagEvaluation(nil), s.evals...)
}

// logCapture collects JSON log lines written through a slog logger.
type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logCapture) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logCapture) logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(l, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// count returns the lines at level whose message contains msg.
func (l *logCapture) count(level, msg string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, line := range strings.Split(l.buf.String(), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		m, _ := rec["msg"].(string)
		if rec["level"] == level && strings.Contains(m, msg) {
			n++
		}
	}
	return n
}

type busEnv struct {
	queue    *fakeQueue
	store    *recordingStore
	fbStore  *recordingStore
	logs     *logCapture
	fallback *command.Dispatcher
	bus      *command.RedisBus
}

func newBusEnv(t *testing.T, consume bool) *busEnv {
	t.Helper()
	return newBusEnvCooldown(t, consume, 0)
}

func newBusEnvCooldown(t *testing.T, consume bool, cooldown time.Duration) *busEnv {
	t.Helper()
	e := &busEnv{queue: newFakeQueue(), store: &recordingStore{}, fbStore: &recordingStore{}, logs: &logCapture{}}
	e.fallback = command.New(command.Options{Deps: command.Deps{Store: e.fbStore}, Logger: e.logs.logger()})
	e.fallback.Start(context.Background())
	e.bus = command.NewRedisBus(command.RedisOptions{
		Queue:    e.queue,
		Deps:     command.Deps{Store: e.store},
		Fallback: e.fallback,
		Consume:  consume,
		Logger:   e.logs.logger(),

		PublishCooldown: cooldown,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = e.bus.Close(ctx)
	})
	return e
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestRedisBusRoundTripFIFO(t *testing.T) {
	e := newBusEnv(t, true)
	ctx := testCtx(t)
	e.bus.Start(ctx)
	for _, k := range []string{"k0", "k1", "k2", "k3", "k4"} {
		e.bus.Submit(ctx, command.TouchAPIKey{KeyHash: k})
	}
	require.NoError(t, e.bus.Flush(ctx))
	require.Equal(t, []string{"k0", "k1", "k2", "k3", "k4"}, e.store.appliedKeys())
	n, err := e.queue.Len(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestRedisBusPublishFailureUsesFallback(t *testing.T) {
	// A nanosecond cooldown makes every submit try Redis again.
	e := newBusEnvCooldown(t, true, time.Nanosecond)
	ctx := testCtx(t)
	e.bus.Start(ctx)

	e.queue.setFailPublish(true)
	for _, k := range []string{"f0", "f1", "f2"} {
		e.bus.Submit(ctx, command.TouchAPIKey{KeyHash: k})
	}
	require.NoError(t, e.fallback.Flush(ctx))
	require.Equal(t, []string{"f0", "f1", "f2"}, e.fbStore.appliedKeys())
	require.Equal(t, 1, e.logs.count("ERROR", "publish failed"))
	require.Equal(t, 2, e.logs.count("WARN", "publish failed"))
	require.Zero(t, e.logs.count("INFO", "publish recovered"))

	e.queue.setFailPublish(false)
	e.bus.Submit(ctx, command.TouchAPIKey{KeyHash: "ok"})
	require.NoError(t, e.bus.Flush(ctx))
	require.Equal(t, []string{"ok"}, e.store.appliedKeys())
	require.Equal(t, 1, e.logs.count("INFO", "publish recovered"))
	require.Equal(t, 1, e.logs.count("ERROR", "publish failed"))
}

// After a failed publish, commands skip Redis for the cooldown, so an outage
// that times out costs one PublishTimeout instead of one per request.
func TestRedisBusCooldownSkipsRedis(t *testing.T) {
	e := newBusEnv(t, true)
	ctx := testCtx(t)
	e.bus.Start(ctx)

	e.queue.setFailPublish(true)
	e.bus.Submit(ctx, command.TouchAPIKey{KeyHash: "first"})
	e.queue.setFailPublish(false)
	e.bus.Submit(ctx, command.TouchAPIKey{KeyHash: "second"})
	require.NoError(t, e.bus.Flush(ctx))

	require.Equal(t, []string{"first", "second"}, e.fbStore.appliedKeys())
	require.Empty(t, e.store.appliedKeys())
	e.queue.mu.Lock()
	require.Equal(t, 1, e.queue.publishes)
	e.queue.mu.Unlock()
	require.Equal(t, 1, e.logs.count("ERROR", "publish failed"))
}

// An LPUSH that lands while its reply times out puts the command on both
// paths. Both must carry the same evaluation id so INSERT OR IGNORE keeps one
// row.
func TestRedisBusAmbiguousPublishKeepsOneEvaluationID(t *testing.T) {
	e := newBusEnv(t, true)
	ctx := testCtx(t)
	e.bus.Start(ctx)

	e.queue.mu.Lock()
	e.queue.storeThenFail = true
	e.queue.mu.Unlock()
	e.bus.Submit(ctx, command.RecordEvaluation{Eval: repository.FlagEvaluation{FlagID: "f", ProjectID: "p"}})
	require.NoError(t, e.bus.Flush(ctx))

	queued, fallback := e.store.appliedEvals(), e.fbStore.appliedEvals()
	require.Len(t, queued, 1)
	require.Len(t, fallback, 1)
	require.NotEmpty(t, queued[0].ID)
	require.Equal(t, queued[0].ID, fallback[0].ID)
}

func TestRedisBusRecoverReappliesUnacked(t *testing.T) {
	e := newBusEnv(t, true)
	ctx := testCtx(t)
	payload, err := command.Encode(command.TouchAPIKey{KeyHash: "left-behind"}, time.Now())
	require.NoError(t, err)
	e.queue.leaveUnacked(payload)

	e.bus.Start(ctx)
	require.NoError(t, e.bus.Flush(ctx))
	require.Equal(t, []string{"left-behind"}, e.store.appliedKeys())
	require.Equal(t, 1, e.logs.count("INFO", "recovered unacknowledged"))
}

func TestRedisBusGarbagePayloadIsAckedAndSkipped(t *testing.T) {
	e := newBusEnv(t, true)
	ctx := testCtx(t)
	require.NoError(t, e.queue.Publish(ctx, []byte("not json")))
	e.bus.Start(ctx)
	e.bus.Submit(ctx, command.TouchAPIKey{KeyHash: "after-garbage"})

	require.NoError(t, e.bus.Flush(ctx))
	require.Equal(t, []string{"after-garbage"}, e.store.appliedKeys())
	require.Equal(t, 1, e.logs.count("ERROR", "decode failed"))
	n, err := e.queue.Len(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestRedisBusCloseDrains(t *testing.T) {
	e := newBusEnv(t, true)
	ctx := testCtx(t)
	e.bus.Start(ctx)
	for _, k := range []string{"d0", "d1", "d2", "d3"} {
		e.bus.Submit(ctx, command.TouchAPIKey{KeyHash: k})
	}
	require.NoError(t, e.bus.Close(ctx))
	require.Equal(t, []string{"d0", "d1", "d2", "d3"}, e.store.appliedKeys())
	require.NoError(t, e.bus.Close(ctx))
}

func TestRedisBusSubmitAfterCloseAppliesNothing(t *testing.T) {
	e := newBusEnv(t, true)
	ctx := testCtx(t)
	e.bus.Start(ctx)
	require.NoError(t, e.bus.Close(ctx))

	require.Zero(t, e.bus.Submit(ctx, command.TouchAPIKey{KeyHash: "late"}))
	require.Empty(t, e.queue.pendingPayloads())
	require.Empty(t, e.store.appliedKeys())
	require.Empty(t, e.fbStore.appliedKeys())
	require.Equal(t, 1, e.logs.count("ERROR", "submitted after close"))
}

func TestRedisBusFlushWithoutConsumeErrors(t *testing.T) {
	e := newBusEnv(t, false)
	ctx := testCtx(t)
	e.bus.Start(ctx)
	require.ErrorContains(t, e.bus.Flush(ctx), "does not consume")

	// Publishing still works; the payload waits for a consumer elsewhere.
	e.bus.Submit(ctx, command.TouchAPIKey{KeyHash: "x"})
	require.Len(t, e.queue.pendingPayloads(), 1)
	require.NoError(t, e.bus.Close(ctx))
}

func TestRedisBusRecordEvaluationKeepsIDAcrossRedelivery(t *testing.T) {
	e := newBusEnv(t, false)
	ctx := testCtx(t)
	eval := fullEvaluation()
	eval.ID = ""
	e.bus.Submit(ctx, command.RecordEvaluation{Eval: eval})

	payloads := e.queue.pendingPayloads()
	require.Len(t, payloads, 1)
	deps := command.Deps{Store: e.store}
	for i := 0; i < 2; i++ {
		c, _, err := command.Decode(payloads[0])
		require.NoError(t, err)
		require.NoError(t, c.Apply(ctx, deps))
	}
	got := e.store.appliedEvals()
	require.Len(t, got, 2)
	require.NotEmpty(t, got[0].ID)
	require.Equal(t, got[0].ID, got[1].ID)
}
