package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/queue"
)

var _ command.Queue = (*queue.RedisList)(nil)

func newList(t *testing.T) (*queue.RedisList, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client, err := queue.NewClient("redis://" + mr.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, queue.Ping(context.Background(), client))
	return queue.NewRedisList(client, "commands", queue.WithPollTimeout(time.Second)), mr
}

func receive(t *testing.T, l *queue.RedisList) (string, func(context.Context) error) {
	t.Helper()
	payload, ack, err := l.Receive(context.Background())
	require.NoError(t, err)
	require.NotNil(t, payload)
	return string(payload), ack
}

func TestNewClientRejectsBadURL(t *testing.T) {
	_, err := queue.NewClient("not a url")
	require.Error(t, err)
}

func TestNewClientErrorOmitsPassword(t *testing.T) {
	_, err := queue.NewClient("redis://:s3cret@host:badport/0")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "s3cret")
}

func TestFIFOOrder(t *testing.T) {
	ctx := context.Background()
	l, _ := newList(t)
	for _, p := range []string{"a", "b", "c"} {
		require.NoError(t, l.Publish(ctx, []byte(p)))
	}
	for _, want := range []string{"a", "b", "c"} {
		got, ack := receive(t, l)
		require.Equal(t, want, got)
		require.NoError(t, ack(ctx))
	}
}

// A batch keeps its order and joins the queue behind what was already there.
func TestPublishBatchKeepsOrder(t *testing.T) {
	ctx := context.Background()
	l, _ := newList(t)
	require.NoError(t, l.Publish(ctx, []byte("first")))
	require.NoError(t, l.PublishBatch(ctx, [][]byte{[]byte("a"), []byte("b"), []byte("c")}))
	require.NoError(t, l.PublishBatch(ctx, nil))

	n, err := l.QueuedLen(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(4), n)

	for _, want := range []string{"first", "a", "b", "c"} {
		got, ack := receive(t, l)
		require.Equal(t, want, got)
		require.NoError(t, ack(ctx))
	}
}

// QueuedLen leaves out what a consumer holds, so a producer's cap measures the
// backlog only.
func TestQueuedLenExcludesProcessing(t *testing.T) {
	ctx := context.Background()
	l, _ := newList(t)
	require.NoError(t, l.PublishBatch(ctx, [][]byte{[]byte("a"), []byte("b")}))
	_, _ = receive(t, l)

	queued, err := l.QueuedLen(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), queued)
	total, err := l.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
}

func TestReceiveIdleReturnsNil(t *testing.T) {
	l, _ := newList(t)
	payload, ack, err := l.Receive(context.Background())
	require.NoError(t, err)
	require.Nil(t, payload)
	require.Nil(t, ack)
}

func TestAckRemovesFromProcessing(t *testing.T) {
	ctx := context.Background()
	l, mr := newList(t)
	require.NoError(t, l.Publish(ctx, []byte("a")))

	_, ack := receive(t, l)
	n, err := l.Len(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	require.NoError(t, ack(ctx))
	n, err = l.Len(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)
	require.False(t, mr.Exists("funnelbarn:queue:commands:processing"))
}

func TestRecoverRestoresUnackedInOrder(t *testing.T) {
	ctx := context.Background()
	l, _ := newList(t)
	for _, p := range []string{"a", "b", "c"} {
		require.NoError(t, l.Publish(ctx, []byte(p)))
	}
	// Two items are received and never acked, as after a consumer crash.
	got, _ := receive(t, l)
	require.Equal(t, "a", got)
	got, _ = receive(t, l)
	require.Equal(t, "b", got)
	require.NoError(t, l.Publish(ctx, []byte("d")))

	moved, err := l.Recover(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, moved)

	for _, want := range []string{"a", "b", "c", "d"} {
		got, ack := receive(t, l)
		require.Equal(t, want, got)
		require.NoError(t, ack(ctx))
	}
}

func TestRecoverOnEmptyProcessingList(t *testing.T) {
	l, _ := newList(t)
	moved, err := l.Recover(context.Background())
	require.NoError(t, err)
	require.Zero(t, moved)
}

func TestLenCountsBothLists(t *testing.T) {
	ctx := context.Background()
	l, _ := newList(t)
	for _, p := range []string{"a", "b", "c"} {
		require.NoError(t, l.Publish(ctx, []byte(p)))
	}
	receive(t, l)
	n, err := l.Len(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 3, n)
}

func TestErrorsWhenRedisIsDown(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client, err := queue.NewClient("redis://" + mr.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	l := queue.NewRedisList(client, "commands", queue.WithPollTimeout(time.Second))
	mr.Close()

	require.ErrorContains(t, queue.Ping(ctx, client), "redis ping")
	require.ErrorContains(t, l.Publish(ctx, []byte("a")), "lpush")
	require.ErrorContains(t, l.PublishBatch(ctx, [][]byte{[]byte("a")}), "lpush")
	_, err = l.QueuedLen(ctx)
	require.ErrorContains(t, err, "llen")
	_, _, err = l.Receive(ctx)
	require.ErrorContains(t, err, "blmove")
	_, err = l.Recover(ctx)
	require.ErrorContains(t, err, "lmove")
	_, err = l.Len(ctx)
	require.ErrorContains(t, err, "llen")
}

func TestAckErrorsWhenRedisIsDown(t *testing.T) {
	ctx := context.Background()
	l, mr := newList(t)
	require.NoError(t, l.Publish(ctx, []byte("a")))
	_, ack := receive(t, l)
	mr.Close()
	require.ErrorContains(t, ack(ctx), "lrem")
}

func TestReceiveHonoursContextCancellation(t *testing.T) {
	l, _ := newList(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := l.Receive(ctx)
	require.Error(t, err)
}
