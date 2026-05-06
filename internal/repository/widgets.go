package repository

import (
	"context"
	"fmt"
	"time"
)

// DashboardWidget is a user-configured breakdown card on the Insights page.
type DashboardWidget struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	EventName string    `json:"event_name"`
	Property  string    `json:"property"`
	Title     string    `json:"title"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// PropertyBreakdown is one row of a top-N breakdown result.
type PropertyBreakdown struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

func (s *Store) CreateWidget(ctx context.Context, w DashboardWidget) (DashboardWidget, error) {
	var err error
	w.ID, err = generateUUID()
	if err != nil {
		return DashboardWidget{}, fmt.Errorf("generate uuid: %w", err)
	}

	const q = `INSERT INTO dashboard_widgets (id, project_id, event_name, property, title, position) VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, w.ID, w.ProjectID, w.EventName, w.Property, w.Title, w.Position); err != nil {
		return DashboardWidget{}, fmt.Errorf("insert widget: %w", err)
	}

	return s.WidgetByID(ctx, w.ID)
}

func (s *Store) WidgetByID(ctx context.Context, id string) (DashboardWidget, error) {
	const q = `SELECT id, project_id, event_name, property, COALESCE(title,''), position, created_at FROM dashboard_widgets WHERE id = ?`
	var w DashboardWidget
	if err := s.db.QueryRowContext(ctx, q, id).Scan(&w.ID, &w.ProjectID, &w.EventName, &w.Property, &w.Title, &w.Position, &w.CreatedAt); err != nil {
		return DashboardWidget{}, err
	}
	return w, nil
}

func (s *Store) ListWidgets(ctx context.Context, projectID string) ([]DashboardWidget, error) {
	const q = `SELECT id, project_id, event_name, property, COALESCE(title,''), position, created_at FROM dashboard_widgets WHERE project_id = ? ORDER BY position, created_at`
	rows, err := s.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var widgets []DashboardWidget
	for rows.Next() {
		var w DashboardWidget
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.EventName, &w.Property, &w.Title, &w.Position, &w.CreatedAt); err != nil {
			return nil, err
		}
		widgets = append(widgets, w)
	}
	return widgets, rows.Err()
}

func (s *Store) UpdateWidget(ctx context.Context, w DashboardWidget) (DashboardWidget, error) {
	const q = `UPDATE dashboard_widgets SET event_name=?, property=?, title=?, position=? WHERE id=?`
	if _, err := s.db.ExecContext(ctx, q, w.EventName, w.Property, w.Title, w.Position, w.ID); err != nil {
		return DashboardWidget{}, fmt.Errorf("update widget: %w", err)
	}
	return s.WidgetByID(ctx, w.ID)
}

func (s *Store) DeleteWidget(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dashboard_widgets WHERE id = ?`, id)
	return err
}

// WidgetBreakdown returns the top-N property value counts from the last `window` events
// of the given event type. This is the rolling window query.
func (s *Store) WidgetBreakdown(ctx context.Context, projectID, eventName, property string, window, limit int) ([]PropertyBreakdown, error) {
	if !validPropertyName.MatchString(property) {
		return nil, fmt.Errorf("invalid property name: %s", property)
	}

	q := fmt.Sprintf(`
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

	rows, err := s.db.QueryContext(ctx, q, projectID, eventName, window, limit)
	if err != nil {
		return nil, fmt.Errorf("widget breakdown: %w", err)
	}
	defer rows.Close()

	var results []PropertyBreakdown
	for rows.Next() {
		var r PropertyBreakdown
		if err := rows.Scan(&r.Value, &r.Count); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
