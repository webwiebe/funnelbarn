package domain

import (
	"errors"
	"time"
)

// Project represents a tracked website or application.
//
// Domain is a display label for the site the project measures (shown on the
// dashboard and in settings). Nothing resolves a project from it: ingest uses
// the API key plus x-funnelbarn-project, which is what lets any f.<domain>
// alias work without per-project wiring.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrProjectUnresolvable marks a slug that can never resolve to a project, no
// matter how often the caller retries: a UUID with no matching project, or a
// slug we refuse to auto-create from. Callers use it to fail fast instead of
// burning retries on a lookup that is deterministically hopeless.
var ErrProjectUnresolvable = errors.New("project cannot be resolved from slug")

// ProjectRecordingSettings holds per-project recording overrides.
// Nil pointer fields mean "inherit from instance settings".
type ProjectRecordingSettings struct {
	ProjectID  string
	Enabled    *bool    // nil = inherit from instance
	SampleRate *float64 // nil = inherit from instance
	Rules      []RecordingRule
	UpdatedAt  time.Time
}

// RecordingRule is a URL pattern rule used to decide whether to capture or ignore a page.
type RecordingRule struct {
	Pattern string `json:"pattern"`
	Action  string `json:"action"` // "capture" or "ignore"
}
