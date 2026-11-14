package domain

import "time"

// CanonicalEvent is an instance-level canonical event in the shared vocabulary.
type CanonicalEvent struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	SortOrder int    `json:"sort_order"`
}

// EventNameMapping maps one project's raw event name onto a canonical key.
type EventNameMapping struct {
	ProjectID    string `json:"project_id"`
	RawName      string `json:"raw_name"`
	CanonicalKey string `json:"canonical_key"`
}

// MappingSuggestion is an unmapped raw event name with a best-guess canonical key.
type MappingSuggestion struct {
	RawName      string `json:"raw_name"`
	SuggestedKey string `json:"suggested_key"` // "" when no confident guess
}

// CanonicalFunnel is an instance-level funnel defined over canonical event keys
// and evaluated across multiple projects.
type CanonicalFunnel struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Scope       string                `json:"scope"`       // "session" (default) or "page_view"
	ProjectIDs  []string              `json:"project_ids"` // default scope; empty = all projects
	Segment     string                `json:"segment"`     // default preset segment; "" = none
	Steps       []CanonicalFunnelStep `json:"steps"`
	CreatedAt   time.Time             `json:"created_at"`
}

// CanonicalFunnelStep is one step, referencing a canonical event key.
type CanonicalFunnelStep struct {
	StepOrder    int    `json:"step_order"`
	CanonicalKey string `json:"canonical_key"`
	Label        string `json:"label"`
}

// CanonicalFunnelResult is the aggregate analysis plus per-project detail.
type CanonicalFunnelResult struct {
	Steps            []FunnelStepResult       `json:"steps"` // aggregated across included projects
	ByProject        []ProjectFunnelBreakdown `json:"by_project"`
	ExcludedProjects []ExcludedProject        `json:"excluded_projects"`
}

// ProjectFunnelBreakdown holds one project's contribution to a canonical funnel.
type ProjectFunnelBreakdown struct {
	ProjectID   string             `json:"project_id"`
	ProjectName string             `json:"project_name"`
	Steps       []FunnelStepResult `json:"steps"`
}

// ExcludedProject records a project dropped from a canonical funnel and why.
type ExcludedProject struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Reason      string `json:"reason"`
}
