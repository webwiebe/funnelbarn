package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// forwardBatchSize bounds one LPUSH.
const forwardBatchSize = 500

// forwardList is the part of queue.RedisList the forwarder uses.
type forwardList interface {
	PublishBatch(ctx context.Context, payloads [][]byte) error
	QueuedLen(ctx context.Context) (int64, error)
}

// spoolForwarder moves spool records onto the ingest queue. The spool stays
// the durable buffer: the cursor moves past a record only after the LPUSH that
// carries it succeeded, so a Valkey outage or a full queue leaves records in
// the spool, and the next tick sends them. A crash between the LPUSH and the
// cursor write sends a record twice, which the consumer stores once.
type spoolForwarder struct {
	list     forwardList
	spoolDir string
	maxLen   int64

	failing bool // last publish failed; logged at Error once per outage
	full    bool // queue at its cap; logged once per episode
}

// forward publishes entries from offset on, up to the queue's cap, and returns
// the new offset. It writes the cursor after each successful batch.
func (f *spoolForwarder) forward(ctx context.Context, entries []spool.RecordAtOffset, offset int64) int64 {
	room := f.room(ctx)
	if room <= 0 {
		return offset
	}
	b := &forwardBatch{f: f, ctx: ctx, offset: offset, end: offset, room: room}
	for _, entry := range entries {
		if !b.add(entry) {
			break
		}
	}
	b.flush()
	return b.offset
}

// room returns how many records the queue takes before it reaches its cap. It
// returns zero when the queue is full or Valkey cannot be reached.
func (f *spoolForwarder) room(ctx context.Context) int64 {
	queued, err := f.list.QueuedLen(ctx)
	if err != nil {
		f.publishFailed(ctx, err)
		return 0
	}
	room := f.maxLen - queued
	if room <= 0 {
		if !f.full {
			f.full = true
			slog.WarnContext(ctx, "ingest queue full, leaving records in the spool",
				"handled", true, "queued", queued, "max_len", f.maxLen)
		}
		return 0
	}
	if f.full {
		f.full = false
		slog.InfoContext(ctx, "ingest queue has room again", "queued", queued)
	}
	return room
}

// forwardBatch collects encoded records for one forward call. offset is the
// cursor as written; end is where the cursor moves once send is published.
type forwardBatch struct {
	f      *spoolForwarder
	ctx    context.Context
	send   [][]byte
	offset int64
	end    int64
	room   int64
	failed bool
}

// add queues one entry and reports whether forwarding goes on.
func (b *forwardBatch) add(entry spool.RecordAtOffset) bool {
	if b.room-int64(len(b.send)) <= 0 {
		return false
	}
	if entry.Malformed != nil {
		if !b.flush() {
			return false
		}
		b.offset = skipMalformed(b.f.spoolDir, entry, b.offset)
		b.end = b.offset
		return true
	}
	payload, err := command.Encode(command.IngestRecord{Record: entry.Record}, time.Now())
	if err != nil {
		// A record is plain data, so this cannot happen in practice. Park it
		// rather than wedge the spool on it.
		if !b.flush() {
			return false
		}
		slog.ErrorContext(b.ctx, "worker encode ingest record", "err", err, "ingest_id", entry.Record.IngestID)
		b.offset = deadLetterRecord(b.f.spoolDir, entry.Record, entry.EndOffset, "dead_letter")
		b.end = b.offset
		return true
	}
	b.send = append(b.send, payload)
	b.end = entry.EndOffset
	if len(b.send) == forwardBatchSize {
		return b.flush()
	}
	return true
}

// flush publishes what is collected and moves the cursor past it. After a
// failed publish it does nothing more, so one forward call tries Valkey once.
func (b *forwardBatch) flush() bool {
	if b.failed {
		return false
	}
	if len(b.send) == 0 {
		return true
	}
	if err := b.f.list.PublishBatch(b.ctx, b.send); err != nil {
		b.failed = true
		b.f.publishFailed(b.ctx, err)
		return false
	}
	b.f.publishRecovered(b.ctx)
	b.offset = b.end
	if err := spool.WriteCursor(b.f.spoolDir, b.offset); err != nil {
		slog.ErrorContext(b.ctx, "worker write cursor", "err", err)
	}
	b.room -= int64(len(b.send))
	b.send = b.send[:0]
	return true
}

func (f *spoolForwarder) publishFailed(ctx context.Context, err error) {
	if !f.failing {
		f.failing = true
		slog.ErrorContext(ctx, "ingest queue publish failed, leaving records in the spool",
			"err", err, "handled", true)
		return
	}
	// The worker ticks every second; the spool stall check reports an outage
	// that lasts.
	slog.DebugContext(ctx, "ingest queue publish failed, leaving records in the spool",
		"err", err, "handled", true)
}

func (f *spoolForwarder) publishRecovered(ctx context.Context) {
	if f.failing {
		f.failing = false
		slog.InfoContext(ctx, "ingest queue publish recovered")
	}
}
