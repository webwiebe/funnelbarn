package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
)

// MaxPageURLBatch caps how many recording rows StripRecordingPageURLs holds in
// memory at once. The cap is in code so that no flag can make a run read a
// whole project into memory (a scan once restarted the production writer).
const MaxPageURLBatch = 500

// pageURLBatchPause is the wait between two batches, so a run leaves the
// writer room for ingest.
const pageURLBatchPause = 50 * time.Millisecond

// PageURLStrip counts one StripRecordingPageURLs run. Scanned is the number of
// rows whose page_url holds a query or a fragment. Changed is the number of
// those rows whose stored value differs from enrich.RecordingPageURL (a row
// that keeps only utm_* parameters is scanned and unchanged).
type PageURLStrip struct {
	Scanned int `json:"scanned"`
	Changed int `json:"changed"`
	Batches int `json:"batches"`
}

type pageURLRow struct{ id, url string }

// StripRecordingPageURLs reduces recordings.page_url of one project to the
// form ingest stores since v0.6.226 (enrich.RecordingPageURL). Rows are read
// in id order, at most batch at a time (capped at MaxPageURLBatch). With
// apply false nothing is written and the counts show what apply would do.
// The run is idempotent and each batch is one transaction.
func (s *Store) StripRecordingPageURLs(ctx context.Context, projectID string, batch int, apply bool) (PageURLStrip, error) {
	if batch <= 0 || batch > MaxPageURLBatch {
		batch = MaxPageURLBatch
	}
	var out PageURLStrip
	after := ""
	for {
		rows, err := s.pageURLBatch(ctx, projectID, after, batch)
		if err != nil {
			return out, err
		}
		if len(rows) == 0 {
			return out, nil
		}
		after = rows[len(rows)-1].id
		changed, err := s.stripPageURLBatch(ctx, rows, apply)
		if err != nil {
			return out, err
		}
		out.Scanned += len(rows)
		out.Changed += changed
		out.Batches++
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

func (s *Store) pageURLBatch(ctx context.Context, projectID, after string, limit int) ([]pageURLRow, error) {
	const q = `
		SELECT id, page_url FROM recordings
		WHERE project_id = ? AND id > ? AND (instr(page_url, '?') > 0 OR instr(page_url, '#') > 0)
		ORDER BY id LIMIT ?`
	rs, err := s.db.QueryContext(ctx, q, projectID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("page urls: read batch: %w", err)
	}
	defer func() { _ = rs.Close() }()
	var rows []pageURLRow
	for rs.Next() {
		var r pageURLRow
		if err := rs.Scan(&r.id, &r.url); err != nil {
			return nil, fmt.Errorf("page urls: scan: %w", err)
		}
		rows = append(rows, r)
	}
	return rows, rs.Err()
}

// stripPageURLBatch counts the rows enrich.RecordingPageURL changes and, with
// apply, updates them in one transaction. The update matches the value it read,
// so a row changed in between is left alone.
func (s *Store) stripPageURLBatch(ctx context.Context, rows []pageURLRow, apply bool) (int, error) {
	type change struct{ id, from, to string }
	var changes []change
	for _, r := range rows {
		if to := enrich.RecordingPageURL(r.url); to != r.url {
			changes = append(changes, change{r.id, r.url, to})
		}
	}
	if !apply || len(changes) == 0 {
		return len(changes), nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("page urls: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, c := range changes {
		if _, err := tx.ExecContext(ctx, `UPDATE recordings SET page_url = ? WHERE id = ? AND page_url = ?`, c.to, c.id, c.from); err != nil {
			return 0, fmt.Errorf("page urls: update: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("page urls: commit: %w", err)
	}
	return len(changes), nil
}
