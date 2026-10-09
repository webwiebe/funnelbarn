package command

import (
	"context"
	"errors"

	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// KindIngestRecord labels an ingest record carried on the ingest queue.
const KindIngestRecord = "ingest_record"

// ErrPermanent marks an apply failure that no retry can fix. RedisBus
// dead-letters a durable command that fails with it straight away.
var ErrPermanent = errors.New("permanent apply failure")

// Durable is implemented by commands that must not be lost when an apply
// fails. RedisBus retries them and then hands them to its DeadLetter hook,
// where a bookkeeping command is logged and dropped.
type Durable interface {
	Command
	durable()
}

// IngestRecord stores one ingest record from the spool. The record keeps its
// ingest_id, and the event insert skips a known ingest_id, so a redelivery
// stores nothing new.
type IngestRecord struct {
	Record spool.Record `json:"record"`
}

// Kind implements Command.
func (IngestRecord) Kind() string { return KindIngestRecord }

// Project implements Command. The record carries a project slug, which the
// consumer resolves; the envelope names no project id.
func (IngestRecord) Project() string { return "" }

// Apply implements Command.
func (c IngestRecord) Apply(ctx context.Context, d Deps) error {
	if d.ApplyIngest == nil {
		return errors.New("ingest record: no ingest applier configured")
	}
	return d.ApplyIngest(ctx, c.Record)
}

func (IngestRecord) durable() {}
