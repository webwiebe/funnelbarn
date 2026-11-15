package widgets

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/idgen"
)

// querier is the read surface shared by *sql.DB handles. Methods that read
// back their own write take the write pool explicitly; plain reads take the
// read pool.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ReadStore is the query side of the widget store. It holds only the read
// pool, so a method with a *ReadStore receiver has no write handle to misuse.
type ReadStore struct {
	rdb *sql.DB
}

// Store embeds the ReadStore and adds the write pool for the command methods.
type Store struct {
	*ReadStore
	db    *sql.DB
	newID func() (string, error)
}

// NewStore returns a widget store over the write pool db and the read pool
// rdb. For an in-memory database both are the same pool.
func NewStore(db, rdb *sql.DB) *Store {
	return &Store{ReadStore: &ReadStore{rdb: rdb}, db: db, newID: idgen.UUID}
}

// clampSize keeps a widget between one and three columns wide.
func clampSize(size int) int {
	if size < 1 || size > 3 {
		return 1
	}
	return size
}

func (s *Store) CreateWidget(ctx context.Context, w Widget) (Widget, error) {
	var err error
	w.ID, err = s.newID()
	if err != nil {
		return Widget{}, fmt.Errorf("generate uuid: %w", err)
	}

	w.Size = clampSize(w.Size)
	const q = `INSERT INTO dashboard_widgets (id, project_id, event_name, property, title, position, size) VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, w.ID, w.ProjectID, w.EventName, w.Property, w.Title, w.Position, w.Size); err != nil {
		return Widget{}, fmt.Errorf("insert widget: %w", err)
	}

	return s.widgetByID(ctx, s.db, w.ID)
}

func (s *ReadStore) WidgetByID(ctx context.Context, id string) (Widget, error) {
	return s.widgetByID(ctx, s.rdb, id)
}

func (s *ReadStore) widgetByID(ctx context.Context, db querier, id string) (Widget, error) {
	const q = `SELECT id, project_id, event_name, property, COALESCE(title,''), position, size, created_at FROM dashboard_widgets WHERE id = ?`
	var w Widget
	if err := db.QueryRowContext(ctx, q, id).Scan(&w.ID, &w.ProjectID, &w.EventName, &w.Property, &w.Title, &w.Position, &w.Size, &w.CreatedAt); err != nil {
		return Widget{}, err
	}
	return w, nil
}

func (s *ReadStore) ListWidgets(ctx context.Context, projectID string) ([]Widget, error) {
	const q = `SELECT id, project_id, event_name, property, COALESCE(title,''), position, size, created_at FROM dashboard_widgets WHERE project_id = ? ORDER BY position, created_at`
	rows, err := s.rdb.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var widgets []Widget
	for rows.Next() {
		var w Widget
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.EventName, &w.Property, &w.Title, &w.Position, &w.Size, &w.CreatedAt); err != nil {
			return nil, err
		}
		widgets = append(widgets, w)
	}
	return widgets, rows.Err()
}

func (s *Store) UpdateWidget(ctx context.Context, w Widget) (Widget, error) {
	w.Size = clampSize(w.Size)
	const q = `UPDATE dashboard_widgets SET event_name=?, property=?, title=?, position=?, size=? WHERE id=?`
	if _, err := s.db.ExecContext(ctx, q, w.EventName, w.Property, w.Title, w.Position, w.Size, w.ID); err != nil {
		return Widget{}, fmt.Errorf("update widget: %w", err)
	}
	return s.widgetByID(ctx, s.db, w.ID)
}

func (s *Store) DeleteWidget(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dashboard_widgets WHERE id = ?`, id)
	return err
}

// WidgetBreakdown returns the top-N property value counts from the last `window` events
// of the given event type. When property is empty, returns only the total count.
// When property is a metadata column (url, browser, etc.), queries the column directly.
// Otherwise, extracts from the JSON properties field.
func (s *ReadStore) WidgetBreakdown(ctx context.Context, projectID, eventName, property string, window, limit int) ([]domain.PropertyBreakdown, error) {
	if property == "" {
		return s.widgetCount(ctx, projectID, eventName, window)
	}

	var q string
	if domain.IsMetadataColumn(property) {
		q = fmt.Sprintf(`
			SELECT val, COUNT(*) AS cnt
			FROM (
				SELECT %s AS val
				FROM (
					SELECT %s FROM events
					WHERE project_id = ? AND name = ?
					ORDER BY occurred_at DESC
					LIMIT ?
				)
			)
			WHERE val IS NOT NULL AND val != ''
			GROUP BY val
			ORDER BY cnt DESC
			LIMIT ?`, property, property)
	} else {
		if !domain.ValidBreakdownProperty(property) {
			return nil, fmt.Errorf("invalid property name: %s", property)
		}
		q = fmt.Sprintf(`
			SELECT val, COUNT(*) AS cnt
			FROM (
				SELECT json_extract(properties, '$.%s') AS val
				FROM (
					SELECT properties FROM events
					WHERE project_id = ? AND name = ?
					ORDER BY occurred_at DESC
					LIMIT ?
				)
			)
			WHERE val IS NOT NULL AND val != ''
			GROUP BY val
			ORDER BY cnt DESC
			LIMIT ?`, property)
	}

	rows, err := s.rdb.QueryContext(ctx, q, projectID, eventName, window, limit)
	if err != nil {
		return nil, fmt.Errorf("widget breakdown: %w", err)
	}
	defer rows.Close()

	var results []domain.PropertyBreakdown
	for rows.Next() {
		var r domain.PropertyBreakdown
		if err := rows.Scan(&r.Value, &r.Count); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *ReadStore) widgetCount(ctx context.Context, projectID, eventName string, window int) ([]domain.PropertyBreakdown, error) {
	const q = `SELECT COUNT(*) FROM (SELECT 1 FROM events WHERE project_id = ? AND name = ? ORDER BY occurred_at DESC LIMIT ?)`
	var count int64
	if err := s.rdb.QueryRowContext(ctx, q, projectID, eventName, window).Scan(&count); err != nil {
		return nil, fmt.Errorf("widget count: %w", err)
	}
	return []domain.PropertyBreakdown{{Value: "_total", Count: count}}, nil
}
