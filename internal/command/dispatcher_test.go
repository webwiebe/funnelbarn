package command_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

type fn struct {
	kind string
	fn   func(ctx context.Context) error
}

func (f fn) Kind() string                                    { return f.kind }
func (f fn) Project() string                                 { return "" }
func (f fn) Apply(ctx context.Context, _ command.Deps) error { return f.fn(ctx) }

func newStarted(t *testing.T, buf int) *command.Dispatcher {
	t.Helper()
	d := command.New(command.Options{Buffer: buf})
	d.Start(context.Background())
	return d
}

func TestFIFOAndFlush(t *testing.T) {
	d := newStarted(t, 8)
	var mu sync.Mutex
	var got []int
	for i := 0; i < 5; i++ {
		i := i
		d.Submit(context.Background(), fn{"fifo", func(context.Context) error {
			mu.Lock()
			got = append(got, i)
			mu.Unlock()
			return nil
		}})
	}
	require.NoError(t, d.Flush(context.Background()))
	mu.Lock()
	require.Equal(t, []int{0, 1, 2, 3, 4}, got)
	mu.Unlock()
	require.NoError(t, d.Close(context.Background()))
}

func TestFlushHonoursContext(t *testing.T) {
	d := newStarted(t, 8)
	release := make(chan struct{})
	d.Submit(context.Background(), fn{"block", func(context.Context) error { <-release; return nil }})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, d.Flush(ctx), context.Canceled)

	close(release)
	require.NoError(t, d.Close(context.Background()))
}

func TestBackpressureBlocksAndNeverDrops(t *testing.T) {
	d := newStarted(t, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	applied := 0
	count := func(context.Context) error { mu.Lock(); applied++; mu.Unlock(); return nil }

	// First command occupies the consumer, second fills the buffer.
	d.Submit(context.Background(), fn{"gate", func(context.Context) error { close(started); <-release; return nil }})
	<-started
	d.Submit(context.Background(), fn{"c", count})

	// Third submit must block, even with an already cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	returned := make(chan time.Duration, 1)
	go func() { returned <- d.Submit(ctx, fn{"c", count}) }()

	select {
	case <-returned:
		t.Fatal("Submit returned while the buffer was full")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	waited := <-returned
	require.Greater(t, waited, time.Duration(0))

	require.NoError(t, d.Flush(context.Background()))
	mu.Lock()
	require.Equal(t, 2, applied)
	mu.Unlock()
	require.NoError(t, d.Close(context.Background()))
}

func TestCloseDrainsAndTimesOut(t *testing.T) {
	d := newStarted(t, 8)
	release := make(chan struct{})
	var mu sync.Mutex
	applied := 0
	d.Submit(context.Background(), fn{"gate", func(context.Context) error { <-release; return nil }})
	for i := 0; i < 3; i++ {
		d.Submit(context.Background(), fn{"c", func(context.Context) error { mu.Lock(); applied++; mu.Unlock(); return nil }})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, d.Close(ctx), context.DeadlineExceeded)

	close(release)
	require.NoError(t, d.Close(context.Background()))
	mu.Lock()
	require.Equal(t, 3, applied)
	mu.Unlock()

	// After close nothing is applied and nothing blocks.
	require.Equal(t, time.Duration(0), d.Submit(context.Background(), fn{"late", func(context.Context) error { t.Error("applied after close"); return nil }}))
	require.Error(t, d.Flush(context.Background()))
}

func TestApplyErrorAndPanicDoNotStopConsumer(t *testing.T) {
	d := newStarted(t, 8)
	before := command.AppliedCount("err_kind", "error")
	d.Submit(context.Background(), fn{"err_kind", func(context.Context) error { return errors.New("boom") }})
	d.Submit(context.Background(), fn{"err_kind", func(context.Context) error { panic("kaboom") }})
	ran := make(chan struct{})
	d.Submit(context.Background(), fn{"ok_kind", func(context.Context) error { close(ran); return nil }})
	require.NoError(t, d.Flush(context.Background()))
	<-ran
	require.Equal(t, before+2, command.AppliedCount("err_kind", "error"))
	require.NoError(t, d.Close(context.Background()))
}

func TestMetricsMove(t *testing.T) {
	d := newStarted(t, 8)
	before := command.AppliedCount("metric_kind", "ok")
	for i := 0; i < 3; i++ {
		d.Submit(context.Background(), fn{"metric_kind", func(context.Context) error { return nil }})
	}
	require.NoError(t, d.Flush(context.Background()))
	require.Equal(t, before+3, command.AppliedCount("metric_kind", "ok"))
	require.NoError(t, d.Close(context.Background()))
}

type fakeStore struct {
	mu        sync.Mutex
	calls     []string
	autoFlags int
}

func (f *fakeStore) rec(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }
func (f *fakeStore) RecordEvaluation(_ context.Context, e repository.FlagEvaluation) error {
	f.rec("eval:" + e.ID)
	return nil
}
func (f *fakeStore) TouchAPIKey(_ context.Context, h string) error { f.rec("key:" + h); return nil }
func (f *fakeStore) FlagByKey(_ context.Context, p, k string) (repository.FeatureFlag, error) {
	return repository.FeatureFlag{ID: "id-" + p + "-" + k}, nil
}
func (f *fakeStore) TouchFlagEvaluated(_ context.Context, id string) error {
	f.rec("touch:" + id)
	return nil
}
func (f *fakeStore) EnsureAutoFlag(_ context.Context, fl repository.FeatureFlag) (repository.FeatureFlag, error) {
	f.rec("auto:" + fl.FlagKey)
	f.mu.Lock()
	f.autoFlags++
	f.mu.Unlock()
	return fl, nil
}
func (f *fakeStore) CountAutoFlags(_ context.Context, _ string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.autoFlags, nil
}

// missingFlagStore reports every flag as missing, as FlagByKey does for a key
// EnsureAutoFlag skipped at the cap.
type missingFlagStore struct{ fakeStore }

func (*missingFlagStore) FlagByKey(context.Context, string, string) (repository.FeatureFlag, error) {
	return repository.FeatureFlag{}, sql.ErrNoRows
}

// The request checks the cap on the read pool, so several queued EnsureAutoFlag
// commands can all pass it; the command re-checks when it applies.
func TestEnsureAutoFlagRechecksCap(t *testing.T) {
	st := &fakeStore{}
	ctx := context.Background()
	for _, k := range []string{"a", "b", "c"} {
		require.NoError(t, command.EnsureAutoFlag{Flag: repository.FeatureFlag{FlagKey: k}, Max: 2}.Apply(ctx, command.Deps{Store: st}))
	}
	require.Equal(t, []string{"auto:a", "auto:b"}, st.calls)
}

func TestTouchFlagEvaluatedSkipsMissingFlag(t *testing.T) {
	st := &missingFlagStore{}
	require.NoError(t, command.TouchFlagEvaluated{ProjectID: "p", FlagKey: "gone"}.Apply(context.Background(), command.Deps{Store: st}))
	require.Empty(t, st.calls)
}

func TestTypedCommands(t *testing.T) {
	st := &fakeStore{}
	ctx := context.Background()
	cmds := []command.Command{
		command.RecordEvaluation{Eval: repository.FlagEvaluation{ID: "e1"}},
		command.TouchAPIKey{KeyHash: "h"},
		command.TouchFlagEvaluated{ProjectID: "p", FlagKey: "k"},
		command.MarkFlagsEvaluated{ProjectID: "p"},
		command.EnsureAutoFlag{Flag: repository.FeatureFlag{FlagKey: "new"}},
	}
	deps := command.Deps{Store: st, MarkFlagsEvaluated: func(_ context.Context, p string) error { st.rec("mark:" + p); return nil }}
	for _, c := range cmds {
		require.NoError(t, c.Apply(ctx, deps), c.Kind())
	}
	require.Equal(t, []string{"eval:e1", "key:h", "touch:id-p-k", "mark:p", "auto:new"}, st.calls)
}
