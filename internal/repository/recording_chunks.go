package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ApplyChunk folds one recording chunk into the recording row and stores its
// trace links, once per (recording, chunk index). A chunk that was already
// applied (an SDK retry, or a queue message delivered twice) changes nothing
// and returns false.
//
// The check, the recording upsert, the chunk marker and the trace links run in
// one transaction, so a failure leaves nothing behind and the redelivered chunk
// is applied in full.
func (s *Store) ApplyChunk(ctx context.Context, r Recording, chunkIndex int, links []TraceLink) (applied bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("apply chunk: begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var one int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM recording_chunks WHERE recording_id = ? AND chunk_index = ?`,
		r.ID, chunkIndex).Scan(&one)
	switch {
	case err == nil:
		return false, tx.Commit()
	case !errors.Is(err, sql.ErrNoRows):
		return false, fmt.Errorf("apply chunk: check: %w", err)
	}

	if err = upsertRecording(ctx, tx, r); err != nil {
		return false, fmt.Errorf("apply chunk: upsert recording: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO recording_chunks (recording_id, chunk_index) VALUES (?, ?)`,
		r.ID, chunkIndex); err != nil {
		return false, fmt.Errorf("apply chunk: mark chunk: %w", err)
	}
	if err = insertTraceLinks(ctx, tx, r.ProjectID, r.SessionID, r.ID, links); err != nil {
		return false, fmt.Errorf("apply chunk: trace links: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("apply chunk: commit: %w", err)
	}
	return true, nil
}
