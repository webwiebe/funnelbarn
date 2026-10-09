package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/queue"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/workerhealth"
)

// ingestQueueName is the name of the Redis list that carries ingest records.
const ingestQueueName = "ingest"

// ingestQueue sends spool records through Valkey: the worker forwards them and
// a RedisBus consumer stores them with the same ingestApplier the in-process
// path uses. Spec 012 has the design.
type ingestQueue struct {
	forwarder *spoolForwarder
	bus       *command.RedisBus
	client    *redis.Client
}

// newIngestQueue returns nil when FUNNELBARN_INGEST_VIA_QUEUE is off, which
// keeps ingest in the worker loop and is the rollback path. The consumer is
// started.
func newIngestQueue(ctx context.Context, cfg config.Config, applier *ingestApplier, logger *slog.Logger) (*ingestQueue, error) {
	if !cfg.IngestViaQueue {
		return nil, nil
	}
	if cfg.RedisQueueURL == "" {
		return nil, errors.New("FUNNELBARN_INGEST_VIA_QUEUE needs FUNNELBARN_REDIS_QUEUE_URL")
	}
	client, err := queue.NewClient(cfg.RedisQueueURL)
	if err != nil {
		return nil, err
	}
	list := queue.NewRedisList(client, ingestQueueName)
	// The URL may carry a password, so only the queue name is logged.
	logger.Info("ingest: redis queue", "queue", ingestQueueName, "max_len", cfg.IngestQueueMaxLen)

	// The consumer runs on its own goroutine, and a workerhealth.Monitor is
	// not safe for concurrent use, so it gets its own.
	consumer := *applier
	consumer.health = workerhealth.New(workerhealth.Options{})
	bus := command.NewRedisBus(command.RedisOptions{
		Queue:   list,
		Deps:    command.Deps{ApplyIngest: consumer.applyQueued},
		Consume: true,
		Logger:  logger,
		DeadLetter: func(ctx context.Context, c command.Command, err error) {
			rec := c.(command.IngestRecord).Record
			reason := "dead_letter"
			if errors.Is(err, repository.ErrProjectUnresolvable) {
				reason = "unresolved_project"
			}
			logger.ErrorContext(ctx, "ingest queue dead-lettering record",
				"err", err, "handled", false, "ingest_id", rec.IngestID, "reason", reason)
			parkRecord(cfg.SpoolDir, rec, reason)
		},
	})
	bus.Start(ctx)
	return &ingestQueue{
		forwarder: &spoolForwarder{list: list, spoolDir: cfg.SpoolDir, maxLen: cfg.IngestQueueMaxLen},
		bus:       bus,
		client:    client,
	}, nil
}

// Close drains the consumer, then closes the client. Safe on a nil queue.
func (q *ingestQueue) Close(ctx context.Context) error {
	if q == nil {
		return nil
	}
	return errors.Join(q.bus.Close(ctx), q.client.Close())
}

// applyQueued stores one record taken from the ingest queue. Geo enrichment is
// read per record here; the queue carries a few thousand records a day.
// A record whose project can never resolve is marked ErrPermanent, so the bus
// dead-letters it without retrying.
func (a *ingestApplier) applyQueued(ctx context.Context, rec spool.Record) error {
	geoEnabled := a.geo != nil
	if geoEnabled {
		val, _, _ := a.store.GetInstanceSetting(ctx, "geo_enabled")
		geoEnabled = val != "false"
	}
	err := a.apply(ctx, rec, geoEnabled)
	if errors.Is(err, repository.ErrProjectUnresolvable) {
		return fmt.Errorf("%w: %w", command.ErrPermanent, err)
	}
	return err
}
