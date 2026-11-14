package domain

import (
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/environment"
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

// SegmentFilter filters funnel analysis by a field condition on the events (or joined sessions) table.
type SegmentFilter struct {
	// Field is the column to filter on: "device_type", "user_id_hash", "browser",
	// "country_code", or the special value "session_returning".
	Field string
	// Op is the comparison operator: "eq", "neq", "is_null", "is_not_null".
	Op string
	// Value is the right-hand side for "eq" / "neq" operators.
	Value string
}

// FunnelSegments holds distinct values available for dynamic segment filtering.
type FunnelSegments struct {
	DeviceTypes []string `json:"device_types"`
	Browsers    []string `json:"browsers"`
	Countries   []string `json:"countries"`
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

// EnvironmentRule narrows a funnel analysis to one environment. env goes through
// the same normalisation as ingest ("prod" is "production"); an empty env means
// all environments and returns no rules. Events are matched by equality, like
// the event-count filters.
func EnvironmentRule(env string) []SegmentRule {
	if strings.TrimSpace(env) == "" {
		return nil
	}
	return []SegmentRule{{Field: SegmentFieldEnvironment, Operator: "eq", Value: environment.Normalize(env)}}
}

// SegmentFieldEnvironment is the events column the funnel environment argument filters
// on. It is deliberately absent from AllowedSegmentFields so a stored segment
// cannot name it; only EnvironmentRule produces a rule for it.
const SegmentFieldEnvironment = "environment"
