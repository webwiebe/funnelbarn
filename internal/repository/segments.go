package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// CreateSegment inserts a new segment.
func (s *Store) CreateSegment(ctx context.Context, seg domain.Segment) (domain.Segment, error) {
	id, err := generateUUID()
	if err != nil {
		return domain.Segment{}, fmt.Errorf("generate uuid: %w", err)
	}
	seg.ID = id
	rulesJSON, _ := json.Marshal(seg.Rules)
	const q = `INSERT INTO segments (id, project_id, name, rules) VALUES (?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, seg.ID, seg.ProjectID, seg.Name, string(rulesJSON)); err != nil {
		return domain.Segment{}, err
	}
	return s.segmentByID(ctx, s.db, seg.ID)
}

// SegmentByID fetches a segment by ID.
func (s *ReadStore) SegmentByID(ctx context.Context, id string) (domain.Segment, error) {
	return s.segmentByID(ctx, s.rdb, id)
}

func (s *ReadStore) segmentByID(ctx context.Context, db querier, id string) (domain.Segment, error) {
	const q = `SELECT id, project_id, name, rules, created_at FROM segments WHERE id = ?`
	var seg domain.Segment
	var rulesJSON string
	err := db.QueryRowContext(ctx, q, id).Scan(&seg.ID, &seg.ProjectID, &seg.Name, &rulesJSON, &seg.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Segment{}, fmt.Errorf("segment not found: %w", err)
	}
	if err != nil {
		return domain.Segment{}, err
	}
	if err := json.Unmarshal([]byte(rulesJSON), &seg.Rules); err != nil {
		slog.WarnContext(ctx, "segment: malformed rules JSON", "segment_id", seg.ID, "raw_len", len(rulesJSON), "error", err)
	}
	return seg, nil
}

// ListSegments returns all segments for a project.
func (s *ReadStore) ListSegments(ctx context.Context, projectID string) ([]domain.Segment, error) {
	const q = `SELECT id, project_id, name, rules, created_at FROM segments WHERE project_id = ? ORDER BY created_at DESC`
	rows, err := s.rdb.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var segs []domain.Segment
	for rows.Next() {
		var seg domain.Segment
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
func (s *Store) UpdateSegment(ctx context.Context, seg domain.Segment) (domain.Segment, error) {
	rulesJSON, _ := json.Marshal(seg.Rules)
	const q = `UPDATE segments SET name = ?, rules = ? WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, seg.Name, string(rulesJSON), seg.ID); err != nil {
		return domain.Segment{}, err
	}
	return s.segmentByID(ctx, s.db, seg.ID)
}

// DeleteSegment removes a segment.
func (s *Store) DeleteSegment(ctx context.Context, id string) error {
	const q = `DELETE FROM segments WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

// UpsertSessionSignals sets device/browser signals on a session only if not
// already collected. Scoped by project: the same session ID can exist under
// several projects, and one project's screen size is not another's.
func (s *Store) UpsertSessionSignals(ctx context.Context, projectID, sessionID string, signals domain.SessionSignals) error {
	return updateSessionSignals(ctx, s.db, projectID, sessionID, signals)
}

func updateSessionSignals(ctx context.Context, ex execer, projectID, sessionID string, signals domain.SessionSignals) error {
	const q = `
		UPDATE sessions SET
			screen_width      = ?,
			screen_height     = ?,
			pixel_ratio       = ?,
			touch             = ?,
			dark_mode         = ?,
			reduced_motion    = ?,
			browser_timezone  = ?,
			cpu_cores         = ?,
			signals_collected = 1
		WHERE project_id = ? AND id = ? AND signals_collected = 0`
	_, err := ex.ExecContext(ctx, q,
		nullInt(signals.ScreenWidth), nullInt(signals.ScreenHeight),
		nullFloatPtr(signals.PixelRatio),
		nullBool(signals.Touch), nullBool(signals.DarkMode), nullBool(signals.ReducedMotion),
		nullStr(signals.BrowserTimezone),
		nullInt(signals.CPUCores),
		projectID, sessionID,
	)
	return err
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullFloatPtr(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullBool(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return 1
	}
	return 0
}
