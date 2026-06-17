package repository

import (
	"context"
	"time"
)

// TraceLink is a single W3C trace observed in the browser during a recording.
// It is sent by the SDK alongside a recording chunk and stored so a trace_id from
// SpanBarn/BugBarn can be resolved back to a session + recording (and seek offset).
type TraceLink struct {
	TraceID    string    `json:"trace_id"`
	SpanID     string    `json:"span_id,omitempty"`
	URL        string    `json:"url,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// TraceLookup resolves a trace_id to the recording that captured it, including the
// seek offset (occurred_at - recording.started_at) so a replay can jump to the
// moment the trace fired.
type TraceLookup struct {
	TraceID            string    `json:"trace_id"`
	ProjectID          string    `json:"project_id"`
	SessionID          string    `json:"session_id"`
	RecordingID        string    `json:"recording_id"`
	OccurredAt         time.Time `json:"occurred_at"`
	OffsetMs           int64     `json:"offset_ms"`
	URL                string    `json:"url,omitempty"`
	RecordingStartedAt time.Time `json:"recording_started_at"`
}

// InsertTraceLinks persists the trace links observed during one recording chunk.
// It is idempotent on (recording_id, trace_id, occurred_at): a retried chunk
// upload re-inserts the same rows without duplicating them.
func (s *Store) InsertTraceLinks(ctx context.Context, projectID, sessionID, recordingID string, links []TraceLink) error {
	if len(links) == 0 {
		return nil
	}
	const q = `
		INSERT INTO recording_traces
			(project_id, session_id, recording_id, trace_id, span_id, url, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(recording_id, trace_id, occurred_at) DO NOTHING`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, l := range links {
		if l.TraceID == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, projectID, sessionID, recordingID, l.TraceID, l.SpanID, l.URL, l.OccurredAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LookupTrace resolves a trace_id to its recording within a project. When a trace
// spans multiple chunks it returns the earliest observation (the most useful seek
// target). Returns sql.ErrNoRows-wrapped nil + found=false when the trace is unknown.
func (s *Store) LookupTrace(ctx context.Context, projectID, traceID string) (TraceLookup, bool, error) {
	const q = `
		SELECT rt.project_id, rt.session_id, rt.recording_id, rt.trace_id, rt.url, rt.occurred_at, r.started_at
		FROM recording_traces rt
		JOIN recordings r ON r.id = rt.recording_id
		WHERE rt.project_id = ? AND rt.trace_id = ?
		ORDER BY rt.occurred_at ASC
		LIMIT 1`
	var out TraceLookup
	err := s.db.QueryRowContext(ctx, q, projectID, traceID).Scan(
		&out.ProjectID, &out.SessionID, &out.RecordingID, &out.TraceID,
		&out.URL, &out.OccurredAt, &out.RecordingStartedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return TraceLookup{}, false, nil
		}
		return TraceLookup{}, false, err
	}
	out.OffsetMs = out.OccurredAt.Sub(out.RecordingStartedAt).Milliseconds()
	if out.OffsetMs < 0 {
		out.OffsetMs = 0
	}
	return out, true, nil
}

// TracesForRecording returns the ordered trace timeline for a recording, for the
// replay UI/CLI to overlay trace markers on the scrubber.
func (s *Store) TracesForRecording(ctx context.Context, recordingID string) ([]TraceLink, error) {
	const q = `
		SELECT trace_id, span_id, url, occurred_at
		FROM recording_traces
		WHERE recording_id = ?
		ORDER BY occurred_at ASC`
	rows, err := s.db.QueryContext(ctx, q, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TraceLink
	for rows.Next() {
		var l TraceLink
		if err := rows.Scan(&l.TraceID, &l.SpanID, &l.URL, &l.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
