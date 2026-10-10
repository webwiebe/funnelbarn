package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/queue"
)

// bookkeepingQueue is the name of the Redis list that carries bookkeeping commands.
const bookkeepingQueue = "bookkeeping"

// newCommandBus builds the bus the services submit bookkeeping commands to.
// It always builds the in-process Dispatcher. With FUNNELBARN_REDIS_QUEUE_URL
// set it returns a RedisBus that consumes the queue itself and hands commands
// to the dispatcher when a publish fails; otherwise it returns the dispatcher,
// which is the rollback path. The bus is started. Closing it closes the Redis
// client after the bus has drained. Spec 012 has the design.
func newCommandBus(ctx context.Context, cfg config.Config, deps command.Deps, logger *slog.Logger) (command.Bus, error) {
	dispatcher := command.New(command.Options{Logger: logger, Deps: deps})
	if cfg.RedisQueueURL == "" {
		dispatcher.Start(ctx)
		return dispatcher, nil
	}

	client, err := queue.NewClient(cfg.RedisQueueURL)
	if err != nil {
		return nil, err
	}
	// The URL may carry a password, so only the queue name is logged.
	logger.Info("command bus: redis", "queue", bookkeepingQueue, "consume", !isReader(cfg))
	if isReader(cfg) {
		return newReaderCommandBus(ctx, cfg, client, logger)
	}
	bus := command.NewRedisBus(command.RedisOptions{
		Queue:    queue.NewRedisList(client, bookkeepingQueue),
		Deps:     deps,
		Fallback: dispatcher,
		Consume:  true,
		Logger:   logger,
	})
	closing := &redisClosingBus{Bus: bus, fallback: dispatcher, client: client}
	closing.Start(ctx)
	return closing, nil
}

// redisClosingBus starts the fallback dispatcher with the bus (RedisBus.Start
// only launches the consumer) and closes the Redis client once the bus has
// drained.
type redisClosingBus struct {
	command.Bus
	fallback command.Bus
	client   *redis.Client
}

// Start launches the fallback, then the queue consumer.
func (b *redisClosingBus) Start(ctx context.Context) {
	b.fallback.Start(ctx)
	b.Bus.Start(ctx)
}

// Close drains the bus, then closes the client.
func (b *redisClosingBus) Close(ctx context.Context) error {
	return errors.Join(b.Bus.Close(ctx), b.client.Close())
}

// bookkeepingFallbackName names a reader's on-disk buffer of bookkeeping
// commands in its spool directory.
const bookkeepingFallbackName = "bookkeeping-fallback"

// readerCommandBus publishes bookkeeping commands and consumes none. A reader
// has no write connection, so a command whose publish fails goes to a file in
// the spool directory, and the reader's worker publishes it later.
type readerCommandBus struct {
	*command.RedisBus
	fallback *fallbackForwarder
	client   *redis.Client
}

func newReaderCommandBus(ctx context.Context, cfg config.Config, client *redis.Client, logger *slog.Logger) (*readerCommandBus, error) {
	fallback, err := command.NewSpoolFallback(cfg.SpoolDir, bookkeepingFallbackName, 0, logger)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	list := queue.NewRedisList(client, bookkeepingQueue)
	bus := command.NewRedisBus(command.RedisOptions{
		Queue:    list,
		Fallback: fallback,
		Logger:   logger,
	})
	bus.Start(ctx)
	return &readerCommandBus{
		RedisBus: bus,
		fallback: &fallbackForwarder{name: "bookkeeping", fallback: fallback, list: list},
		client:   client,
	}, nil
}

// Flush is a no-op: the writer applies what a reader publishes.
func (b *readerCommandBus) Flush(context.Context) error { return nil }

// Close stops publishing, closes the fallback file, then the client.
func (b *readerCommandBus) Close(ctx context.Context) error {
	return errors.Join(b.RedisBus.Close(ctx), b.client.Close())
}
