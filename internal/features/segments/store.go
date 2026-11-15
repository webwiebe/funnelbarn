package segments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/idgen"
)

// querier is the read surface shared by *sql.DB handles. Methods that read
// back their own write take the write pool explicitly; plain reads take the
// read pool.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ReadStore is the query side of the segment store. It holds only the read
// pool.
type ReadStore struct {
	rdb *sql.DB
}

// Store embeds the ReadStore and adds the write pool for the command methods.
type Store struct {
	*ReadStore
	db    *sql.DB
	newID func() (string, error)
}

// NewStore returns a segment store over the write pool db and the read pool
// rdb. For an in-memory database both are the same pool.
func NewStore(db, rdb *sql.DB) *Store {
	return &Store{ReadStore: &ReadStore{rdb: rdb}, db: db, newID: idgen.UUID}
}

// CreateSegment inserts a new segment.
func (s *Store) CreateSegment(ctx context.Context, seg Segment) (Segment, error) {
	id, err := s.newID()
	if err != nil {
		return Segment{}, fmt.Errorf("generate uuid: %w", err)
	}
	seg.ID = id
	rulesJSON, _ := json.Marshal(seg.Rules)
	const q = `INSERT INTO segments (id, project_id, name, rules) VALUES (?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, seg.ID, seg.ProjectID, seg.Name, string(rulesJSON)); err != nil {
		return Segment{}, err
	}
	return s.segmentByID(ctx, s.db, seg.ID)
}

// SegmentByID fetches a segment by ID.
func (s *ReadStore) SegmentByID(ctx context.Context, id string) (Segment, error) {
	return s.segmentByID(ctx, s.rdb, id)
}

func (s *ReadStore) segmentByID(ctx context.Context, db querier, id string) (Segment, error) {
	const q = `SELECT id, project_id, name, rules, created_at FROM segments WHERE id = ?`
	var seg Segment
	var rulesJSON string
	err := db.QueryRowContext(ctx, q, id).Scan(&seg.ID, &seg.ProjectID, &seg.Name, &rulesJSON, &seg.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Segment{}, fmt.Errorf("segment not found: %w", err)
	}
	if err != nil {
		return Segment{}, err
	}
	if err := json.Unmarshal([]byte(rulesJSON), &seg.Rules); err != nil {
		slog.WarnContext(ctx, "segment: malformed rules JSON", "segment_id", seg.ID, "raw_len", len(rulesJSON), "error", err)
	}
	return seg, nil
}

// ListSegments returns all segments for a project.
func (s *ReadStore) ListSegments(ctx context.Context, projectID string) ([]Segment, error) {
	const q = `SELECT id, project_id, name, rules, created_at FROM segments WHERE project_id = ? ORDER BY created_at DESC`
	rows, err := s.rdb.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var segs []Segment
	for rows.Next() {
		var seg Segment
		var rulesJSON string
		if err := rows.Scan(&seg.ID, &seg.ProjectID, &seg.Name, &rulesJSON, &seg.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(rulesJSON), &seg.Rules); err != nil {
			slog.WarnContext(ctx, "segment: malformed rules JSON", "segment_id", seg.ID, "raw_len", len(rulesJSON), "error", err)
		}
		segs = append(segs, seg)
	}
	return segs, rows.Err()
}

// UpdateSegment replaces a segment's name and rules.
func (s *Store) UpdateSegment(ctx context.Context, seg Segment) (Segment, error) {
	rulesJSON, _ := json.Marshal(seg.Rules)
	const q = `UPDATE segments SET name = ?, rules = ? WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, seg.Name, string(rulesJSON), seg.ID); err != nil {
		return Segment{}, err
	}
	return s.segmentByID(ctx, s.db, seg.ID)
}

// DeleteSegment removes a segment.
func (s *Store) DeleteSegment(ctx context.Context, id string) error {
	const q = `DELETE FROM segments WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}
