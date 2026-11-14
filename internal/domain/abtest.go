package domain

import "time"

// ABTest represents an A/B test experiment.
type ABTest struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	Name            string    `json:"name"`
	Status          string    `json:"status"`
	ControlFilter   string    `json:"control_filter"`
	VariantFilter   string    `json:"variant_filter"`
	ConversionEvent string    `json:"conversion_event"`
	CreatedAt       time.Time `json:"created_at"`
}

// ABTestFilter is a JSON-serialisable event property filter.
type ABTestFilter struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

// ABTestResult holds per-variant analysis results.
type ABTestResult struct {
	Variant     string  `json:"variant"`
	Total       int64   `json:"total"`
	Conversions int64   `json:"conversions"`
	Rate        float64 `json:"rate"`
}
