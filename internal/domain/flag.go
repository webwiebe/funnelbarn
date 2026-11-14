package domain

import "time"

// FeatureFlag represents an OpenFeature-compatible feature flag.
type FeatureFlag struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	FlagKey         string    `json:"flag_key"`
	Name            string    `json:"name"`
	FlagType        string    `json:"flag_type"`
	Variants        string    `json:"variants"`
	DefaultVariant  string    `json:"default_variant"`
	Split           string    `json:"split"`
	ConversionEvent string    `json:"conversion_event"`
	TargetingRules  string    `json:"targeting_rules"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	// Origin is "manual" for human-created flags and "auto" for flags created on
	// first evaluation. LastEvaluatedAt tracks the most recent evaluation of an
	// auto flag so stale, never-configured ones can be pruned.
	Origin          string     `json:"origin"`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at,omitempty"`
	// Kind is "experiment" (bucketed per user, every read is an analytics data
	// point) or "config" (a singleton value a server reads on a loop — no
	// bucketing, no evaluation rows, no variant/conversion report).
	Kind string `json:"flag_kind"`
}

// FlagKindConfig is a singleton value read by a server: no bucketing, no
// evaluation rows, and no variant/conversion report.
const FlagKindConfig = "config"

// FlagKindExperiment is the A/B-test shape: bucketed per targeting key, every
// evaluation recorded, reported on by variant and conversion.
const FlagKindExperiment = "experiment"

// FlagEvaluation records a single flag evaluation.
type FlagEvaluation struct {
	ID          string    `json:"id"`
	FlagID      string    `json:"flag_id"`
	ProjectID   string    `json:"project_id"`
	Variant     string    `json:"variant"`
	ContextHash string    `json:"context_hash"`
	SessionID   string    `json:"session_id"`
	ContextKeys []string  `json:"context_keys"` // key names present in the eval context
	CreatedAt   time.Time `json:"created_at"`
}

// ContextKeySuggestion is a context key seen in recent evaluations with its frequency.
type ContextKeySuggestion struct {
	ContextKey string `json:"context_key"`
	SeenCount  int64  `json:"seen_count"`
	Pct        int    `json:"pct"` // percentage of evaluations in the last 30 days that included this key
}

// FlagAnalysisResult holds per-variant analysis.
type FlagAnalysisResult struct {
	Variant     string  `json:"variant"`
	Sample      int64   `json:"sample"`
	Conversions int64   `json:"conversions"`
	Rate        float64 `json:"rate"`
}

// FlagUsageRow is the number of recorded evaluations of one flag that resolved
// to one variant on one UTC day.
type FlagUsageRow struct {
	FlagID  string
	Day     string // YYYY-MM-DD, UTC
	Variant string
	Count   int64
}
