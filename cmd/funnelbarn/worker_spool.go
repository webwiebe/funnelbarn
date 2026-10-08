package main

import (
	"errors"
	"log/slog"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/workerhealth"
)

// deadLetterRecord parks an unprocessable record and advances the cursor past
// it, so one bad record can never wedge the spool. reason labels the metric:
// "dead_letter" for a record that exhausted its retries, "unresolved_project"
// for one that could never have been persisted at all. Returns the new offset.
func deadLetterRecord(spoolDir string, record spool.Record, endOffset int64, reason string) int64 {
	if err := spool.AppendDeadLetter(spoolDir, record); err != nil {
		// A full dead-letter file is its own alert: something has been failing
		// in bulk and nothing has drained it. handled=false so it reaches
		// BugBarn as an issue rather than scrolling past in the log.
		if errors.Is(err, spool.ErrDeadLetterFull) {
			slog.Error("dead-letter file is full; records are being discarded — replay it with 'funnelbarn replay-dead-letter' and find what is failing",
				"err", err, "handled", false,
				"ingest_id", record.IngestID,
				"limit_bytes", spool.MaxDeadLetterBytes)
		} else {
			slog.Error("worker dead-letter write", "ingest_id", record.IngestID, "err", err)
		}
	}
	metrics.EventErrors.WithLabelValues(reason).Inc()
	if err := spool.WriteCursor(spoolDir, endOffset); err != nil {
		slog.Error("worker write cursor", "err", err)
	}
	return endOffset
}

// skipMalformed reports spool bytes that are not a record and moves the cursor
// past them. Only the offset and length are logged, since the bytes are user
// data. handled=false so the lost record reaches BugBarn as an issue. Returns
// the new offset.
func skipMalformed(spoolDir string, entry spool.RecordAtOffset, offset int64) int64 {
	slog.Error("worker skipping malformed spool line",
		"err", entry.Malformed, "handled", false,
		"offset", offset,
		"length", entry.EndOffset-offset)
	metrics.EventErrors.WithLabelValues("malformed").Inc()
	if err := spool.WriteCursor(spoolDir, entry.EndOffset); err != nil {
		slog.Error("worker write cursor", "err", err)
	}
	return entry.EndOffset
}

// checkSpoolProgress raises an issue when the spool has pending bytes the
// cursor isn't draining. This is the blindspot that hid a ~5-day ingestion
// outage. It runs on every tick, a failed spool read included, because a read
// that keeps failing is a stall too.
func checkSpoolProgress(health *workerhealth.Monitor, spoolDir string, offset int64) {
	size, err := spool.ActiveSize(spoolDir)
	if err != nil {
		return
	}
	pending := size - offset
	if pending < 0 {
		pending = 0
	}
	metrics.IngestPendingBytes.Set(float64(pending))
	if stalled, since := health.CheckProgress(offset, pending); stalled {
		slog.Error("ingest worker stalled: spool backlog not draining",
			"handled", false,
			"pending_bytes", pending,
			"offset", offset,
			"stalled_for", since.Round(time.Second).String())
	}
	stalledGauge := 0.0
	if health.Stalled() {
		stalledGauge = 1
	}
	metrics.IngestStalled.Set(stalledGauge)
}
