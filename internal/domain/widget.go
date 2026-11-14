package domain

import "time"

// DashboardWidget is a user-configured breakdown card on the Insights page.
type DashboardWidget struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	EventName string    `json:"event_name"`
	Property  string    `json:"property"`
	Title     string    `json:"title"`
	Position  int       `json:"position"`
	Size      int       `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// PropertyBreakdown is one row of a top-N breakdown result.
type PropertyBreakdown struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}
