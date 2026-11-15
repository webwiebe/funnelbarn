// Package widgets is the dashboard widget slice: the breakdown cards on the
// Insights page, from their HTTP handlers down to their SQL.
package widgets

import "time"

// Widget is a user-configured breakdown card on the Insights page.
type Widget struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	EventName string    `json:"event_name"`
	Property  string    `json:"property"`
	Title     string    `json:"title"`
	Position  int       `json:"position"`
	Size      int       `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}
