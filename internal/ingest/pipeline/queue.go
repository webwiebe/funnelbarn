package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/queue"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// IngestQueueName is the name of the Redis list that carries ingest records.
const IngestQueueName = "ingest"

// IngestQueue sends spool records through Valkey: the worker forwards them and
// a RedisBus consumer stores them with the same Applier the in-process
// path uses. Spec 012 has the design.
type IngestQueue struct {
	Forwarder *SpoolForwarder
	Bus       *command.RedisBus
	Client    *redis.Client
}

// NewIngestQueue returns nil when FUNNELBARN_INGEST_VIA_QUEUE is off in a
// standalone process, which keeps ingest in the worker loop and is the
// rollback path. A reader and a writer always use the queue; a reader only
// forwards to it. The consumer is started.
func NewIngestQueue(ctx context.Context, cfg config.Config, applier *Applier, logger *slog.Logger) (*IngestQueue, error) {
	if !cfg.IngestViaQueue && !IsSplit(cfg) {
		return nil, nil
	}
	if cfg.RedisQueueURL == "" {
		return nil, errors.New("FUNNELBARN_INGEST_VIA_QUEUE needs FUNNELBARN_REDIS_QUEUE_URL")
	}
	client, err := queue.NewClient(cfg.RedisQueueURL)
	if err != nil {
		return nil, err
	}
	list := queue.NewRedisList(client, IngestQueueName)
	// The URL may carry a password, so only the queue name is logged.
	logger.Info("ingest: redis queue", "queue", IngestQueueName, "max_len", cfg.IngestQueueMaxLen, "consume", !IsReader(cfg))
	forwarder := &SpoolForwarder{list: list, spoolDir: cfg.SpoolDir, maxLen: cfg.IngestQueueMaxLen}
	if IsReader(cfg) {
		return &IngestQueue{Forwarder: forwarder, Client: client}, nil
	}

	// The consumer runs on its own goroutine, and a Monitor is
	// not safe for concurrent use, so it gets its own.
	consumer := *applier
	consumer.Health = NewMonitor(MonitorOptions{})
	bus := command.NewRedisBus(command.RedisOptions{
		Queue:   list,
		Deps:    command.Deps{ApplyIngest: consumer.applyQueued},
		Consume: true,
		Logger:  logger,
		DeadLetter: func(ctx context.Context, c command.Command, err error) {
			rec := c.(command.IngestRecord).Record
			reason := "dead_letter"
			if errors.Is(err, domain.ErrProjectUnresolvable) {
				reason = "unresolved_project"
			}
			logger.ErrorContext(ctx, "ingest queue dead-lettering record",
				"err", err, "handled", false, "ingest_id", rec.IngestID, "reason", reason)
			parkRecord(cfg.SpoolDir, rec, reason)
		},
	})
	bus.Start(ctx)
	return &IngestQueue{Forwarder: forwarder, Bus: bus, Client: client}, nil
}

// Close drains the consumer, if any, then closes the client. Safe on a nil
// queue.
func (q *IngestQueue) Close(ctx context.Context) error {
	if q == nil {
		return nil
	}
	var busErr error
	if q.Bus != nil {
		busErr = q.Bus.Close(ctx)
	}
	return errors.Join(busErr, q.Client.Close())
}

// applyQueued stores one record taken from the ingest queue. Geo enrichment is
// read per record here; the queue carries a few thousand records a day.
// A record whose project can never resolve is marked ErrPermanent, so the bus
// dead-letters it without retrying.
func (a *Applier) applyQueued(ctx context.Context, rec spool.Record) error {
	geoEnabled := a.Geo != nil
	if geoEnabled {
		val, _, _ := a.Store.GetInstanceSetting(ctx, "geo_enabled")
		geoEnabled = val != "false"
	}
	err := a.apply(ctx, rec, geoEnabled)
	if errors.Is(err, domain.ErrProjectUnresolvable) {
		return fmt.Errorf("%w: %w", command.ErrPermanent, err)
	}
	return err
}
