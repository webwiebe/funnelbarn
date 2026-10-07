package command_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/queue"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

const itKeyHash = "it-key-hash"

// itEnv is a RedisBus transport over miniredis with a real SQLite store behind
// the consumer.
type itEnv struct {
	list    *queue.RedisList
	store   *repository.Store
	project repository.Project
	flag    repository.FeatureFlag
}

func newITEnv(t *testing.T) *itEnv {
	t.Helper()
	ctx := context.Background()

	mr := miniredis.RunT(t)
	client, err := queue.NewClient("redis://" + mr.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	store, err := repository.Open(filepath.Join(t.TempDir(), "bus.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	p, err := store.CreateProject(ctx, "Bus", "bus")
	require.NoError(t, err)
	_, err = store.CreateAPIKey(ctx, "k", p.ID, itKeyHash, "full")
	require.NoError(t, err)
	f, err := store.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID: p.ID, FlagKey: "it_flag", Name: "It", FlagType: "boolean",
		Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: `{"on":50,"off":50}`, Status: "active",
	})
	require.NoError(t, err)

	return &itEnv{list: queue.NewRedisList(client, "bookkeeping", queue.WithPollTimeout(time.Second)), store: store, project: p, flag: f}
}

// bus builds a RedisBus on the shared list and closes it when the test ends,
// before the store's own cleanup.
func (e *itEnv) bus(t *testing.T, consume bool) *command.RedisBus {
	t.Helper()
	deps := command.Deps{
		Store:              e.store,
		MarkFlagsEvaluated: service.NewProjectHealthService(e.store).MarkFlagsEvaluated,
	}
	// The fallback must run: Flush waits on it too.
	fallback := command.New(command.Options{Deps: deps})
	fallback.Start(context.Background())
	b := command.NewRedisBus(command.RedisOptions{
		Queue:    e.list,
		Deps:     deps,
		Fallback: fallback,
		Consume:  consume,
	})
	b.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, b.Close(ctx))
	})
	return b
}

func (e *itEnv) eval(id string) repository.FlagEvaluation {
	return repository.FlagEvaluation{
		ID: id, FlagID: e.flag.ID, ProjectID: e.project.ID, Variant: "on",
		ContextHash: "h-" + id, SessionID: "s-" + id, ContextKeys: []string{"plan"},
	}
}

func (e *itEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, e.store.DB().QueryRow(query, args...).Scan(&n))
	return n
}

func (e *itEnv) evalIDs(t *testing.T) []string {
	t.Helper()
	rows, err := e.store.DB().Query(`SELECT id FROM flag_evaluations ORDER BY rowid`)
	require.NoError(t, err)
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

func (e *itEnv) keyUsed(t *testing.T) bool {
	t.Helper()
	return e.count(t, `SELECT COUNT(*) FROM api_keys WHERE key_hash = ? AND last_used_at IS NOT NULL`, itKeyHash) == 1
}

func (e *itEnv) flagTouched(t *testing.T) bool {
	t.Helper()
	return e.count(t, `SELECT COUNT(*) FROM feature_flags WHERE id = ? AND last_evaluated_at IS NOT NULL`, e.flag.ID) == 1
}

func itCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestRedisBusIntegrationQueuedCommandsWaitForConsumer(t *testing.T) {
	e := newITEnv(t)
	ctx := itCtx(t)

	producer := e.bus(t, false)
	for i := 0; i < 20; i++ {
		producer.Submit(ctx, command.RecordEvaluation{Eval: e.eval(fmt.Sprintf("eval-%02d", i))})
	}
	producer.Submit(ctx, command.TouchAPIKey{KeyHash: itKeyHash})
	producer.Submit(ctx, command.TouchFlagEvaluated{ProjectID: e.project.ID, FlagKey: e.flag.FlagKey})

	n, err := e.list.Len(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 22, n)
	require.Equal(t, 0, e.count(t, `SELECT COUNT(*) FROM flag_evaluations`))
	require.False(t, e.keyUsed(t))
	require.False(t, e.flagTouched(t))

	consumer := e.bus(t, true)
	require.NoError(t, consumer.Flush(ctx))

	require.Equal(t, 20, e.count(t, `SELECT COUNT(*) FROM flag_evaluations`))
	require.True(t, e.keyUsed(t))
	require.True(t, e.flagTouched(t))
}

func TestRedisBusIntegrationRedeliveryAfterCrashAppliesOnce(t *testing.T) {
	e := newITEnv(t)
	ctx := itCtx(t)

	// Empty ID: Encode assigns one, so the redelivered payload keeps it.
	e.bus(t, false).Submit(ctx, command.RecordEvaluation{Eval: e.eval("")})

	// A consumer that receives the payload, applies it and dies before the ack.
	payload, _, err := e.list.Receive(ctx)
	require.NoError(t, err)
	require.NotNil(t, payload)
	c, _, err := command.Decode(payload)
	require.NoError(t, err)
	require.NoError(t, c.Apply(ctx, command.Deps{Store: e.store}))
	require.Equal(t, 1, e.count(t, `SELECT COUNT(*) FROM flag_evaluations`))

	moved, err := e.list.Recover(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, moved)

	consumer := e.bus(t, true)
	require.NoError(t, consumer.Flush(ctx))
	require.Equal(t, 1, e.count(t, `SELECT COUNT(*) FROM flag_evaluations`))
}

func TestRedisBusIntegrationStartRecoversUnackedPayload(t *testing.T) {
	e := newITEnv(t)
	ctx := itCtx(t)

	e.bus(t, false).Submit(ctx, command.RecordEvaluation{Eval: e.eval("lost-in-flight")})
	payload, _, err := e.list.Receive(ctx) // received, never acked
	require.NoError(t, err)
	require.NotNil(t, payload)

	// Start recovers the processing list itself.
	consumer := e.bus(t, true)
	require.NoError(t, consumer.Flush(ctx))
	require.Equal(t, []string{"lost-in-flight"}, e.evalIDs(t))
}

func TestRedisBusIntegrationAppliesInSubmissionOrder(t *testing.T) {
	e := newITEnv(t)
	ctx := itCtx(t)

	producer := e.bus(t, false)
	var want []string
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("ord-%02d", i)
		want = append(want, id)
		producer.Submit(ctx, command.RecordEvaluation{Eval: e.eval(id)})
	}

	consumer := e.bus(t, true)
	require.NoError(t, consumer.Flush(ctx))
	require.Equal(t, want, e.evalIDs(t))
}
