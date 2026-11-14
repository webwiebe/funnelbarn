package repository

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// InsertTraceLinks persists the trace links observed during one recording chunk.
// It is idempotent on (recording_id, trace_id, occurred_at): a retried chunk
// upload re-inserts the same rows without duplicating them.
func (s *Store) InsertTraceLinks(ctx context.Context, projectID, sessionID, recordingID string, links []domain.TraceLink) error {
	if len(links) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := insertTraceLinks(ctx, tx, projectID, sessionID, recordingID, links); err != nil {
		return err
	}
	return tx.Commit()
}

func insertTraceLinks(ctx context.Context, ex execer, projectID, sessionID, recordingID string, links []domain.TraceLink) error {
	const q = `
		INSERT INTO recording_traces
			(project_id, session_id, recording_id, trace_id, span_id, url, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(recording_id, trace_id, occurred_at) DO NOTHING`
	for _, l := range links {
		if l.TraceID == "" {
			continue
		}
		if _, err := ex.ExecContext(ctx, q, projectID, sessionID, recordingID, l.TraceID, l.SpanID, l.URL, l.OccurredAt); err != nil {
			return err
		}
	}
	return nil
}

// LookupTrace resolves a trace_id to its recording within a project. When a trace
// spans multiple chunks it returns the earliest observation (the most useful seek
// target). Returns sql.ErrNoRows-wrapped nil + found=false when the trace is unknown.
func (s *ReadStore) LookupTrace(ctx context.Context, projectID, traceID string) (domain.TraceLookup, bool, error) {
	const q = `
		SELECT rt.project_id, rt.session_id, rt.recording_id, rt.trace_id, rt.url, rt.occurred_at,
		       r.started_at, r.first_chunk_index, r.last_chunk_index, r.chunk_count, r.duration_ms, r.environment, r.page_url
		FROM recording_traces rt
		JOIN recordings r ON r.id = rt.recording_id
		WHERE rt.project_id = ? AND rt.trace_id = ?
		ORDER BY rt.occurred_at ASC
		LIMIT 1`
	var out domain.TraceLookup
	err := s.rdb.QueryRowContext(ctx, q, projectID, traceID).Scan(
		&out.ProjectID, &out.SessionID, &out.RecordingID, &out.TraceID,
		&out.URL, &out.OccurredAt, &out.RecordingStartedAt,
		&out.FirstChunkIndex, &out.LastChunkIndex, &out.ChunkCount, &out.DurationMs, &out.Environment, &out.PageURL,
	)
	if err != nil {
		if isNoRows(err) {
			return domain.TraceLookup{}, false, nil
		}
		return domain.TraceLookup{}, false, err
	}
	out.OffsetMs = out.OccurredAt.Sub(out.RecordingStartedAt).Milliseconds()
	if out.OffsetMs < 0 {
		out.OffsetMs = 0
	}
	return out, true, nil
}

// TracesForRecording returns the ordered trace timeline for a recording, for the
// replay UI/CLI to overlay trace markers on the scrubber.
func (s *ReadStore) TracesForRecording(ctx context.Context, recordingID string) ([]domain.TraceLink, error) {
	const q = `
		SELECT trace_id, span_id, url, occurred_at
		FROM recording_traces
		WHERE recording_id = ?
		ORDER BY occurred_at ASC`
	rows, err := s.rdb.QueryContext(ctx, q, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TraceLink
	for rows.Next() {
		var l domain.TraceLink
		if err := rows.Scan(&l.TraceID, &l.SpanID, &l.URL, &l.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
