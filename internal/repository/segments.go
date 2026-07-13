package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Segment is a named, reusable filter applied to funnel analysis.
type Segment struct {
	ID        string        `json:"id"`
	ProjectID string        `json:"project_id"`
	Name      string        `json:"name"`
	Rules     []SegmentRule `json:"rules"`
	CreatedAt time.Time     `json:"created_at"`
}

// SegmentRule is one condition within a segment.
type SegmentRule struct {
	Field    string `json:"field"`    // see allowedSegmentFields
	Operator string `json:"operator"` // "eq", "neq", "contains", "not_contains", "is_null", "is_not_null"
	Value    string `json:"value"`
}

// AllowedSegmentFields maps field name → table alias for the funnel JOIN query.
// Fields with alias "s" require a sessions JOIN; "e" don't.
var AllowedSegmentFields = map[string]string{
	"device_type":      "e",
	"browser":          "e",
	"os":               "e",
	"country_code":     "s",
	"city":             "s",
	"connection_class": "s",
	"dark_mode":        "s",
	"browser_timezone": "s",
}

// CreateSegment inserts a new segment.
func (s *Store) CreateSegment(ctx context.Context, seg Segment) (Segment, error) {
	id, err := generateUUID()
	if err != nil {
		return Segment{}, fmt.Errorf("generate uuid: %w", err)
	}
	seg.ID = id
	rulesJSON, _ := json.Marshal(seg.Rules)
	const q = `INSERT INTO segments (id, project_id, name, rules) VALUES (?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, seg.ID, seg.ProjectID, seg.Name, string(rulesJSON)); err != nil {
		return Segment{}, err
	}
	return s.SegmentByID(ctx, seg.ID)
}

// SegmentByID fetches a segment by ID.
func (s *Store) SegmentByID(ctx context.Context, id string) (Segment, error) {
	const q = `SELECT id, project_id, name, rules, created_at FROM segments WHERE id = ?`
	var seg Segment
	var rulesJSON string
	err := s.db.QueryRowContext(ctx, q, id).Scan(&seg.ID, &seg.ProjectID, &seg.Name, &rulesJSON, &seg.CreatedAt)
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
func (s *Store) ListSegments(ctx context.Context, projectID string) ([]Segment, error) {
	const q = `SELECT id, project_id, name, rules, created_at FROM segments WHERE project_id = ? ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, projectID)
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
	return s.SegmentByID(ctx, seg.ID)
}

// DeleteSegment removes a segment.
func (s *Store) DeleteSegment(ctx context.Context, id string) error {
	const q = `DELETE FROM segments WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

// UpsertSessionSignals sets device/browser signals on a session only if not already collected.
func (s *Store) UpsertSessionSignals(ctx context.Context, sessionID string, signals SessionSignals) error {
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
		WHERE id = ? AND signals_collected = 0`
	_, err := s.db.ExecContext(ctx, q,
		nullInt(signals.ScreenWidth), nullInt(signals.ScreenHeight),
		nullFloatPtr(signals.PixelRatio),
		nullBool(signals.Touch), nullBool(signals.DarkMode), nullBool(signals.ReducedMotion),
		nullStr(signals.BrowserTimezone),
		nullInt(signals.CPUCores),
		sessionID,
	)
	return err
}

// SessionSignals holds the device/browser signals collected by the SDK.
type SessionSignals struct {
	ScreenWidth     *int
	ScreenHeight    *int
	PixelRatio      *float64
	Touch           *bool
	DarkMode        *bool
	ReducedMotion   *bool
	BrowserTimezone string
	CPUCores        *int
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
