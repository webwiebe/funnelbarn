package mcp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// maxFlagsListed caps list_flags so a project that has accumulated hundreds of
// auto-registered flags doesn't blow out a single tool response. There is no
// paging beyond it: the dashboard's Flags page is the tool for that.
const maxFlagsListed = 200

// registerFlagTools registers list_flags, get_flag, analyze_flag,
// list_flag_context_keys, evaluate_flag, create_flag, update_flag and
// delete_flag on top of service.Flags.
func registerFlagTools(s *mcp.Server, d *Deps) {
	addTool(s, d, &mcp.Tool{
		Name: "list_flags",
		Description: "List every feature flag in a project: key, name, kind, status, origin, " +
			"variants, split and targeting rules. Returns up to 200 flags, newest first; a " +
			"project with more has to be pruned or browsed from the Flags page.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listFlags)

	addTool(s, d, &mcp.Tool{
		Name:        "get_flag",
		Description: "Get one feature flag by flag_key or flag_id.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, getFlag)

	addTool(s, d, &mcp.Tool{
		Name: "analyze_flag",
		Description: "Per-variant sample size, conversions and conversion rate for an " +
			"experiment flag over a time range, with a two-proportion z-test when there are " +
			"exactly two variants (|z| > 1.96 is significant at 95% confidence). A config flag " +
			"records no evaluations, so it reports unavailable=\"config\" and an empty result " +
			"set instead.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, analyzeFlag)

	addTool(s, d, &mcp.Tool{
		Name: "list_flag_context_keys",
		Description: "Context keys seen in this project's recent flag evaluations, with how " +
			"often each appeared. Use it to write targeting rules against keys your SDK " +
			"actually sends instead of guessing.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listFlagContextKeys)

	addTool(s, d, &mcp.Tool{
		Name: "evaluate_flag",
		Description: "Resolve a flag evaluation for a given context, exactly the way an SDK " +
			"would. Unlike an SDK's own evaluate call, this never auto-registers an unrecognised " +
			"key, so testing a typo can't create a flag, and it records no evaluation, so a dry " +
			"run never shows up in analyze_flag.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, evaluateFlag)

	addTool(s, d, &mcp.Tool{
		Name: "create_flag",
		Description: "Create a feature flag with a single default value returned to everyone " +
			"until you add targeting rules. It appears on the Flags page with origin " +
			"\"manual\" and is active immediately, so SDK evaluation returns its value right away.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(false)},
	}, scopeWrite, createFlag)

	addTool(s, d, &mcp.Tool{
		Name: "update_flag",
		Description: "Update a feature flag's name, default value, kind, targeting rules or " +
			"status. Fields you omit keep their current value.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(false)},
	}, scopeWrite, updateFlag)

	addTool(s, d, &mcp.Tool{
		Name: "delete_flag",
		Description: "Delete a feature flag. This cannot be undone; SDKs evaluating this key " +
			"afterwards get FLAG_NOT_FOUND and fall back to their own default.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(false)},
	}, scopeWrite, deleteFlag)
}

// flagDetail is a feature flag as returned by list_flags, get_flag,
// create_flag and update_flag.
type flagDetail struct {
	ID      string `json:"id" jsonschema:"Flag ID."`
	FlagKey string `json:"flag_key" jsonschema:"Key SDKs evaluate by."`
	Name    string `json:"name" jsonschema:"Human-readable name shown on the Flags page."`
	Kind    string `json:"flag_kind" jsonschema:"\"experiment\" (bucketed per user, every evaluation recorded) or \"config\" (one value read on a loop, no bucketing, no evaluation rows)."`
	Status  string `json:"status" jsonschema:"\"active\" or \"paused\" (paused always returns the default variant, reason DISABLED)."`
	Origin  string `json:"origin" jsonschema:"\"manual\" (created by a human, API token or this MCP server) or \"auto\" (created on the first SDK evaluation of a key nobody registered yet)."`
	// Variants, DefaultVariant, Split and TargetingRules are kept as the raw
	// JSON strings the storage layer uses, the same shape the dashboard and
	// REST API expose, rather than re-modelled types that could drift from
	// what evaluate_flag actually reads.
	Variants        string     `json:"variants" jsonschema:"JSON object mapping variant name to its value, e.g. {\"on\":true,\"off\":false}."`
	DefaultVariant  string     `json:"default_variant" jsonschema:"Variant name returned when no targeting rule matches and (for an experiment) no split bucket applies."`
	Split           string     `json:"split" jsonschema:"JSON object mapping variant name to its traffic percentage for an experiment, e.g. {\"on\":50,\"off\":50}. Ignored for a config flag."`
	ConversionEvent string     `json:"conversion_event,omitempty" jsonschema:"Event name analyze_flag counts conversions against; empty if unset."`
	TargetingRules  string     `json:"targeting_rules" jsonschema:"JSON array of targeting rules; [] means none configured."`
	CreatedAt       time.Time  `json:"created_at" jsonschema:"When the flag was created."`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at,omitempty" jsonschema:"When the flag was last evaluated; unset if never."`
}

func toFlagDetail(f repository.FeatureFlag) flagDetail {
	return flagDetail{
		ID:              f.ID,
		FlagKey:         f.FlagKey,
		Name:            f.Name,
		Kind:            f.Kind,
		Status:          f.Status,
		Origin:          f.Origin,
		Variants:        f.Variants,
		DefaultVariant:  f.DefaultVariant,
		Split:           f.Split,
		ConversionEvent: f.ConversionEvent,
		TargetingRules:  f.TargetingRules,
		CreatedAt:       f.CreatedAt,
		LastEvaluatedAt: f.LastEvaluatedAt,
	}
}

// resolveFlag loads the flag named by flagKey or flagID, scoped to the
// resolved project: flagKey goes through GetFlagByKey (already scoped, so a
// flag in another project is a plain not-found); flagID goes through GetFlag
// and then checks ProjectID itself, per the package's object-ID convention.
func resolveFlag(ctx context.Context, c *Call, project, flagKey, flagID string) (repository.Project, repository.FeatureFlag, error) {
	p, err := c.Project(ctx, project)
	if err != nil {
		return repository.Project{}, repository.FeatureFlag{}, err
	}
	switch {
	case flagKey != "":
		f, err := c.Deps.Flags.GetFlagByKey(ctx, p.ID, flagKey)
		if err != nil {
			return p, repository.FeatureFlag{}, err
		}
		return p, f, nil
	case flagID != "":
		f, err := c.Deps.Flags.GetFlag(ctx, flagID)
		if err != nil {
			return p, repository.FeatureFlag{}, err
		}
		if f.ProjectID != p.ID {
			return p, repository.FeatureFlag{}, fmt.Errorf("%w: flag %s", domain.ErrNotFound, flagID)
		}
		return p, f, nil
	default:
		return p, repository.FeatureFlag{}, invalidInput("flag_key or flag_id is required")
	}
}

// ---------------------------------------------------------------------------
// list_flags
// ---------------------------------------------------------------------------

type listFlagsIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
}

type listFlagsOut struct {
	Flags []flagDetail `json:"flags" jsonschema:"Up to 200 flags for the project, newest first."`
}

func listFlags(ctx context.Context, c *Call, in listFlagsIn) (listFlagsOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return listFlagsOut{}, err
	}
	flags, err := c.Deps.Flags.ListFlags(ctx, p.ID)
	if err != nil {
		return listFlagsOut{}, err
	}
	if len(flags) > maxFlagsListed {
		flags = flags[:maxFlagsListed]
	}
	out := listFlagsOut{Flags: make([]flagDetail, len(flags))}
	for i, f := range flags {
		out.Flags[i] = toFlagDetail(f)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// get_flag
// ---------------------------------------------------------------------------

type getFlagIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	FlagKey string `json:"flag_key,omitempty" jsonschema:"The flag's key. Either flag_key or flag_id is required."`
	FlagID  string `json:"flag_id,omitempty" jsonschema:"The flag's ID. Either flag_key or flag_id is required."`
}

func getFlag(ctx context.Context, c *Call, in getFlagIn) (flagDetail, error) {
	_, f, err := resolveFlag(ctx, c, in.Project, in.FlagKey, in.FlagID)
	if err != nil {
		return flagDetail{}, err
	}
	return toFlagDetail(f), nil
}

// ---------------------------------------------------------------------------
// analyze_flag
// ---------------------------------------------------------------------------

type analyzeFlagIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	FlagKey string `json:"flag_key" jsonschema:"The flag's key."`
	From    string `json:"from,omitempty" jsonschema:"Start of the analysis window, RFC 3339 or YYYY-MM-DD. Defaults to seven days before to."`
	To      string `json:"to,omitempty" jsonschema:"End of the analysis window, RFC 3339 or YYYY-MM-DD. Defaults to now."`
}

type flagAnalysisVariant struct {
	Variant     string  `json:"variant" jsonschema:"Variant name."`
	Sample      int64   `json:"sample" jsonschema:"Distinct targeting keys evaluated to this variant in the window."`
	Conversions int64   `json:"conversions" jsonschema:"How many of them then fired the flag's conversion_event, if one is configured."`
	Rate        float64 `json:"rate" jsonschema:"conversions / sample, 0 when sample is 0."`
}

type analyzeFlagOut struct {
	FlagKey     string                `json:"flag_key" jsonschema:"The flag's key."`
	Results     []flagAnalysisVariant `json:"results" jsonschema:"One entry per variant with at least one evaluation in the window."`
	Significant bool                  `json:"significant" jsonschema:"True when there are exactly two variants and the two-proportion z-test is significant at 95% confidence."`
	ZScore      float64               `json:"z_score" jsonschema:"The z-score behind significant; 0 unless there are exactly two variants."`
	From        time.Time             `json:"from" jsonschema:"Start of the analysis window actually used."`
	To          time.Time             `json:"to" jsonschema:"End of the analysis window actually used."`
	Unavailable string                `json:"unavailable,omitempty" jsonschema:"Set to \"config\" when this is a config flag: it records no evaluations, so there is nothing to analyze."`
}

func analyzeFlag(ctx context.Context, c *Call, in analyzeFlagIn) (analyzeFlagOut, error) {
	if in.FlagKey == "" {
		return analyzeFlagOut{}, invalidInput("flag_key is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return analyzeFlagOut{}, err
	}
	flag, err := c.Deps.Flags.GetFlagByKey(ctx, p.ID, in.FlagKey)
	if err != nil {
		return analyzeFlagOut{}, err
	}
	from, to, err := parseRange(in.From, in.To, time.Now())
	if err != nil {
		return analyzeFlagOut{}, err
	}

	// A config flag records no evaluations (see FlagService.EvaluateFlag), so
	// a variant/conversion report for it would be an empty table dressed up
	// as a result. Mirrors handleFlagAnalysis's config case.
	if flag.Kind == repository.FlagKindConfig {
		return analyzeFlagOut{
			FlagKey:     flag.FlagKey,
			Results:     []flagAnalysisVariant{},
			From:        from,
			To:          to,
			Unavailable: "config",
		}, nil
	}

	results, err := c.Deps.Flags.AnalyzeFlag(ctx, flag, from, to)
	if err != nil {
		return analyzeFlagOut{}, err
	}
	out := analyzeFlagOut{FlagKey: flag.FlagKey, From: from, To: to, Results: make([]flagAnalysisVariant, len(results))}
	for i, r := range results {
		out.Results[i] = flagAnalysisVariant{Variant: r.Variant, Sample: r.Sample, Conversions: r.Conversions, Rate: r.Rate}
	}
	if len(results) == 2 {
		out.ZScore, out.Significant = service.ZTestTwoProportions(
			results[0].Sample, results[0].Conversions,
			results[1].Sample, results[1].Conversions,
		)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// list_flag_context_keys
// ---------------------------------------------------------------------------

type listFlagContextKeysIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
}

type contextKeySuggestion struct {
	ContextKey string `json:"context_key" jsonschema:"Key name seen in the evaluation context."`
	SeenCount  int64  `json:"seen_count" jsonschema:"How many recent evaluations included this key."`
	Pct        int    `json:"pct" jsonschema:"Percentage of evaluations in the last 30 days that included this key."`
}

type listFlagContextKeysOut struct {
	ContextKeys []contextKeySuggestion `json:"context_keys" jsonschema:"Context keys seen in this project's recent flag evaluations, most frequent first."`
}

func listFlagContextKeys(ctx context.Context, c *Call, in listFlagContextKeysIn) (listFlagContextKeysOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return listFlagContextKeysOut{}, err
	}
	suggestions, err := c.Deps.Flags.ContextKeySuggestions(ctx, p.ID)
	if err != nil {
		return listFlagContextKeysOut{}, err
	}
	out := listFlagContextKeysOut{ContextKeys: make([]contextKeySuggestion, len(suggestions))}
	for i, s := range suggestions {
		out.ContextKeys[i] = contextKeySuggestion{ContextKey: s.ContextKey, SeenCount: s.SeenCount, Pct: s.Pct}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// evaluate_flag
// ---------------------------------------------------------------------------

type evaluateFlagIn struct {
	Project string         `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	FlagKey string         `json:"flag_key" jsonschema:"The flag's key."`
	Context map[string]any `json:"context,omitempty" jsonschema:"Evaluation context: targetingKey (or session_id) plus whatever attributes your targeting rules match on."`
}

type evaluateFlagOut struct {
	Value              any            `json:"value" jsonschema:"The resolved value."`
	Variant            string         `json:"variant" jsonschema:"The resolved variant name."`
	Reason             string         `json:"reason" jsonschema:"Why this variant was chosen: TARGETING_MATCH, SPLIT, STATIC (config flag) or DISABLED (paused flag)."`
	FlagKey            string         `json:"flag_key" jsonschema:"The flag's key."`
	FlagMetadata       map[string]any `json:"flag_metadata,omitempty" jsonschema:"Extra detail, e.g. evaluated_rule_name on a TARGETING_MATCH."`
	CacheMaxAgeSeconds int            `json:"cache_max_age_seconds" jsonschema:"How long an SDK may cache this result; 0 for an experiment, since every real read there is an analytics data point."`
}

func evaluateFlag(ctx context.Context, c *Call, in evaluateFlagIn) (evaluateFlagOut, error) {
	if in.FlagKey == "" {
		return evaluateFlagOut{}, invalidInput("flag_key is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return evaluateFlagOut{}, err
	}
	evalContext := in.Context
	if evalContext == nil {
		evalContext = map[string]any{}
	}
	// PreviewFlag runs the same evaluation as an SDK call but never
	// auto-registers an unknown key and never records an evaluation row, so a
	// dry run leaves the flag's analytics untouched.
	res, err := c.Deps.Flags.PreviewFlag(ctx, p.ID, in.FlagKey, evalContext)
	if err != nil {
		// PreviewFlag's not-found error wraps sql.ErrNoRows, not
		// domain.ErrNotFound (see FlagService.EvaluateFlag): translate it here
		// so toolError logs it at Warn and returns "not found" instead of
		// treating a routine unknown key as an unexpected server error.
		if errors.Is(err, sql.ErrNoRows) {
			return evaluateFlagOut{}, fmt.Errorf("%w: flag %s", domain.ErrNotFound, in.FlagKey)
		}
		return evaluateFlagOut{}, err
	}
	return evaluateFlagOut{
		Value:              res.Value,
		Variant:            res.Variant,
		Reason:             res.Reason,
		FlagKey:            res.FlagKey,
		FlagMetadata:       res.FlagMetadata,
		CacheMaxAgeSeconds: res.CacheMaxAgeSeconds,
	}, nil
}
