package domain

import (
	"regexp"
	"time"
)

// Funnel is a multi-step conversion funnel.
type Funnel struct {
	ID          string       `json:"id"`
	ProjectID   string       `json:"project_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Scope       string       `json:"scope"` // "session" (default), "page_view" or "user"
	Steps       []FunnelStep `json:"steps"`
	CreatedAt   time.Time    `json:"created_at"`

	// ConversionWindowSeconds bounds a "user" funnel: every later step must
	// happen within this many seconds of step 1. Nil means
	// DefaultConversionWindow. The other scopes ignore it.
	ConversionWindowSeconds *int64 `json:"conversion_window_seconds,omitempty"`

	// UnmatchedSteps lists the step event names this project has never emitted.
	// A funnel whose steps reference names the project does not send can never
	// report a conversion, and presents as a legitimate zero-conversion funnel
	// rather than as misconfiguration — 20 of 37 funnels in production are in
	// that state. Read-only: computed on load, never persisted, ignored on write.
	UnmatchedSteps []string `json:"unmatched_steps,omitempty"`
}

// FunnelStep is one step in a funnel.
type FunnelStep struct {
	ID        string         `json:"id"`
	FunnelID  string         `json:"funnel_id"`
	StepOrder int            `json:"step_order"`
	EventName string         `json:"event_name"`
	Filters   []FunnelFilter `json:"filters"`
}

// FunnelFilter filters events at a funnel step by property value.
type FunnelFilter struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

// FunnelStepResult holds analysis results for one funnel step.
type FunnelStepResult struct {
	StepOrder  int     `json:"step_order"`
	EventName  string  `json:"event_name"`
	Count      int64   `json:"count"`
	Conversion float64 `json:"conversion"`
	DropOff    float64 `json:"drop_off"`
}

// Funnel scopes: what one funnel entry is keyed on.
const (
	// FunnelScopeSession counts distinct sessions per step (the default).
	FunnelScopeSession = "session"
	// FunnelScopePageView counts distinct page views per step.
	FunnelScopePageView = "page_view"
	// FunnelScopeUser counts distinct users (user_id_hash) through ordered
	// steps: a user reaches step N only by firing it at or after their step
	// N-1, and within the funnel's conversion window of their step 1.
	FunnelScopeUser = "user"
)

// DefaultConversionWindow is the window of a user-scoped funnel whose
// ConversionWindowSeconds is unset.
const DefaultConversionWindow = 30 * 24 * time.Hour

// FunnelBreakdownGroup is one value of the breakdown property and the funnel
// numbers of the entries that carry it.
type FunnelBreakdownGroup struct {
	// Value is the property value on the entry event. Empty for entries without
	// the property.
	Value string `json:"value"`
	// Label is what a UI shows: the value, "(none)" or "(other)".
	Label string `json:"label"`
	// Other marks the group that collects every value beyond the cap.
	Other bool `json:"other,omitempty"`
	// Entries is the number of funnel entries in the group (step 1 count).
	Entries int64              `json:"entries"`
	Results []FunnelStepResult `json:"results"`
}

// MaxBreakdownGroups caps the groups of one breakdown; the rest collapse into
// one "other" group so the response stays bounded.
const MaxBreakdownGroups = 20

// ValidBreakdownProperty reports whether name can be used as a breakdown
// property. Names go into a JSON path, so only a safe subset is accepted.
func ValidBreakdownProperty(name string) bool { return validPropertyName.MatchString(name) }

// ConversionWindow returns the window of a user-scoped funnel.
func (f Funnel) ConversionWindow() time.Duration {
	if f.ConversionWindowSeconds == nil {
		return DefaultConversionWindow
	}
	return time.Duration(*f.ConversionWindowSeconds) * time.Second
}

// validPropertyName matches only alphanumeric and underscore characters.
var validPropertyName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
