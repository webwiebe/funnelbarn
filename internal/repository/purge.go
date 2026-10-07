package repository

import (
	"context"
	"strconv"
	"time"
)

// purgeBatchSize bounds how many rows one retention DELETE touches. The write
// pool has a single connection, so one unbounded DELETE would hold it for the
// whole purge. Each batch releases it, letting queued writes run in between.
const purgeBatchSize = 500

// purgeOlderThan deletes rows of table whose col is strictly before cutoff, in
// batches of purgeBatchSize, and returns the total deleted. It checks ctx
// between batches and returns the count so far together with ctx.Err() when the
// context ends. table and col are package constants, never user input.
func (s *Store) purgeOlderThan(ctx context.Context, table, col string, cutoff time.Time) (int64, error) {
	q := `DELETE FROM ` + table + ` WHERE rowid IN (SELECT rowid FROM ` + table +
		` WHERE ` + col + ` < ? LIMIT ` + strconv.Itoa(purgeBatchSize) + `)`
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		res, err := s.db.ExecContext(ctx, q, cutoff)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
		if n < purgeBatchSize {
			return total, nil
		}
	}
}

// PurgeOldEvents deletes events older than the given cutoff time and returns the number deleted.
func (s *Store) PurgeOldEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	return s.purgeOlderThan(ctx, "events", "occurred_at", cutoff)
}

// PurgeOldEvaluations deletes flag evaluations older than cutoff and returns the number deleted.
func (s *Store) PurgeOldEvaluations(ctx context.Context, cutoff time.Time) (int64, error) {
	return s.purgeOlderThan(ctx, "flag_evaluations", "created_at", cutoff)
}
