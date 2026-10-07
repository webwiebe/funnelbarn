// Package queue implements the durable command transport on a Redis (or
// Valkey) list. Producers LPUSH encoded commands; the single consumer moves
// each one onto a processing list with BLMOVE, applies it, and acks by removing
// it from there. A consumer that dies between receive and ack leaves the item
// in the processing list, and Recover puts it back on the queue.
package queue

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	keyPrefix = "funnelbarn:queue:"
	// defaultPollTimeout is how long Receive blocks before reporting an idle queue.
	defaultPollTimeout = 2 * time.Second
)

var tracer = otel.Tracer("funnelbarn/queue")

// NewClient builds a Redis client from redisURL without connecting, so startup
// never blocks on Redis. The connection is established on first use.
func NewClient(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		// url.Error quotes the whole URL, password included; keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("queue: parse redis url: %w", err)
	}
	return redis.NewClient(opts), nil
}

// Ping verifies that Redis is reachable.
func Ping(ctx context.Context, client *redis.Client) error {
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("queue: redis ping: %w", err)
	}
	return nil
}

// Option configures a RedisList.
type Option func(*RedisList)

// WithPollTimeout sets how long Receive blocks waiting for an item. Redis
// blocking commands take whole seconds; go-redis rounds shorter values up to 1s.
func WithPollTimeout(d time.Duration) Option {
	return func(l *RedisList) { l.pollTimeout = d }
}

// RedisList is a reliable FIFO queue on two Redis lists: the queue itself and
// a processing list holding items received but not yet acked.
type RedisList struct {
	client      *redis.Client
	name        string
	queueKey    string
	procKey     string
	pollTimeout time.Duration
}

// NewRedisList returns a queue named name, stored under
// "funnelbarn:queue:<name>" and "funnelbarn:queue:<name>:processing".
func NewRedisList(client *redis.Client, name string, opts ...Option) *RedisList {
	l := &RedisList{
		client:      client,
		name:        name,
		queueKey:    keyPrefix + name,
		procKey:     keyPrefix + name + ":processing",
		pollTimeout: defaultPollTimeout,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Publish appends one payload to the queue.
func (l *RedisList) Publish(ctx context.Context, payload []byte) error {
	ctx, span := tracer.Start(ctx, "queue.publish",
		trace.WithAttributes(attribute.String("queue.name", l.name)),
	)
	defer span.End()

	if err := l.client.LPush(ctx, l.queueKey, payload).Err(); err != nil {
		err = fmt.Errorf("queue: lpush: %w", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// Receive blocks for up to the poll timeout and returns the oldest payload,
// moved onto the processing list. The returned ack removes exactly that payload
// from the processing list. An idle queue returns a nil payload and nil error.
func (l *RedisList) Receive(ctx context.Context) ([]byte, func(context.Context) error, error) {
	ctx, span := tracer.Start(ctx, "queue.receive",
		trace.WithAttributes(attribute.String("queue.name", l.name)),
	)
	defer span.End()

	val, err := l.client.BLMove(ctx, l.queueKey, l.procKey, "RIGHT", "LEFT", l.pollTimeout).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil, nil
	}
	if err != nil {
		err = fmt.Errorf("queue: blmove: %w", err)
		span.SetStatus(codes.Error, err.Error())
		return nil, nil, err
	}

	ack := func(ctx context.Context) error {
		if err := l.client.LRem(ctx, l.procKey, 1, val).Err(); err != nil {
			return fmt.Errorf("queue: lrem: %w", err)
		}
		return nil
	}
	return []byte(val), ack, nil
}

// Recover moves everything left in the processing list back onto the queue and
// returns how many items it moved. Items go to the consuming end, so they are
// received next and keep their original order. Receive pushes onto the left of
// the processing list, so the newest item sits at the left: moving left to
// right puts the oldest one last, on the end Receive pops from.
func (l *RedisList) Recover(ctx context.Context) (int, error) {
	moved := 0
	for {
		_, err := l.client.LMove(ctx, l.procKey, l.queueKey, "LEFT", "RIGHT").Result()
		if errors.Is(err, redis.Nil) {
			return moved, nil
		}
		if err != nil {
			return moved, fmt.Errorf("queue: lmove: %w", err)
		}
		moved++
	}
}

// Len returns the number of payloads waiting, processing list included.
func (l *RedisList) Len(ctx context.Context) (int64, error) {
	pipe := l.client.Pipeline()
	queued := pipe.LLen(ctx, l.queueKey)
	processing := pipe.LLen(ctx, l.procKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("queue: llen: %w", err)
	}
	return queued.Val() + processing.Val(), nil
}
