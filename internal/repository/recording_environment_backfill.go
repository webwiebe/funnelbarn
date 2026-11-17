package repository

import (
	"context"
	"fmt"
	"time"
)

// EnvironmentBackfill counts one BackfillRecordingEnvironments run.
//   - Scanned: recordings of the project with an empty environment that the run read.
//   - Filled: recordings whose session events all share one environment (written with apply).
//   - Mixed: sessions whose events disagree; the recording stays empty.
//   - NoEvents: recordings whose session has no events; they stay empty.
type EnvironmentBackfill struct {
	Scanned  int `json:"scanned"`
	Filled   int `json:"filled"`
	Mixed    int `json:"mixed"`
	NoEvents int `json:"no_events"`
	Batches  int `json:"batches"`
}

type envRecordingRow struct{ id, sessionID string }

// BackfillRecordingEnvironments gives the recordings of one project that were
// stored with an empty environment (#560) the environment of their session's
// events. A session whose events disagree, or that has none, is left empty and
// counted. Rows are read in id order, at most batch at a time (capped at
// MaxPageURLBatch), one transaction per batch. With apply false nothing is
// written. Events with an empty environment count as production, the way the
// dashboard reads them. onBatch, when set, receives the running totals after each batch.
func (s *Store) BackfillRecordingEnvironments(ctx context.Context, projectID string, batch int, apply bool, onBatch func(EnvironmentBackfill)) (EnvironmentBackfill, error) {
	if batch <= 0 || batch > MaxPageURLBatch {
		batch = MaxPageURLBatch
	}
	var out EnvironmentBackfill
	after := ""
	for {
		rows, err := s.emptyEnvironmentBatch(ctx, projectID, after, batch)
		if err != nil {
			return out, err
		}
		if len(rows) == 0 {
			return out, nil
		}
		after = rows[len(rows)-1].id
		if err := s.backfillEnvironmentBatch(ctx, projectID, rows, apply, &out); err != nil {
			return out, err
		}
		out.Batches++
		if onBatch != nil {
			onBatch(out)
		}
		if len(rows) < batch {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(pageURLBatchPause):
		}
	}
}

func (s *Store) emptyEnvironmentBatch(ctx context.Context, projectID, after string, limit int) ([]envRecordingRow, error) {
	rs, err := s.db.QueryContext(ctx,
		`SELECT id, session_id FROM recordings WHERE project_id = ? AND environment = '' AND id > ? ORDER BY id LIMIT ?`,
		projectID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("environment backfill: read batch: %w", err)
	}
	defer func() { _ = rs.Close() }()
	var rows []envRecordingRow
	for rs.Next() {
		var r envRecordingRow
		if err := rs.Scan(&r.id, &r.sessionID); err != nil {
			return nil, fmt.Errorf("environment backfill: scan: %w", err)
		}
		rows = append(rows, r)
	}
	return rows, rs.Err()
}

// sessionEnvironments returns up to two distinct environments of a session's
// events (two is enough to tell that they disagree).
func (s *Store) sessionEnvironments(ctx context.Context, projectID, sessionID string) ([]string, error) {
	rs, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT CASE WHEN environment = '' THEN 'production' ELSE environment END
		 FROM events WHERE session_id = ? AND project_id = ? LIMIT 2`, sessionID, projectID)
	if err != nil {
		return nil, fmt.Errorf("environment backfill: session events: %w", err)
	}
	defer func() { _ = rs.Close() }()
	var envs []string
	for rs.Next() {
		var e string
		if err := rs.Scan(&e); err != nil {
			return nil, fmt.Errorf("environment backfill: scan session: %w", err)
		}
		envs = append(envs, e)
	}
	return envs, rs.Err()
}

func (s *Store) backfillEnvironmentBatch(ctx context.Context, projectID string, rows []envRecordingRow, apply bool, out *EnvironmentBackfill) error {
	type fill struct{ id, env string }
	var fills []fill
	for _, r := range rows {
		out.Scanned++
		envs, err := s.sessionEnvironments(ctx, projectID, r.sessionID)
		if err != nil {
			return err
		}
		switch len(envs) {
		case 0:
			out.NoEvents++
		case 1:
			out.Filled++
			fills = append(fills, fill{r.id, envs[0]})
		default:
			out.Mixed++
		}
	}
	if !apply || len(fills) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("environment backfill: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, f := range fills {
		if _, err := tx.ExecContext(ctx, `UPDATE recordings SET environment = ? WHERE id = ? AND environment = ''`, f.env, f.id); err != nil {
			return fmt.Errorf("environment backfill: update: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("environment backfill: commit: %w", err)
	}
	return nil
}
