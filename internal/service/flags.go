package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// FlagEvalResult follows the OpenFeature resolution details structure.
type FlagEvalResult struct {
	Value        any            `json:"value"`
	Variant      string         `json:"variant"`
	Reason       string         `json:"reason"`
	FlagKey      string         `json:"flag_key"`
	ErrorCode    string         `json:"error_code,omitempty"`
	FlagMetadata map[string]any `json:"flag_metadata,omitempty"`
}

type TargetingCondition struct {
	ContextKey string `json:"context_key"`
	Operator   string `json:"operator"`
	Value      string `json:"value"`
}

type TargetingRule struct {
	Name       string               `json:"name"`
	Variant    string               `json:"variant"`
	Match      string               `json:"match"`
	Conditions []TargetingCondition `json:"conditions"`
}

var validOperators = map[string]bool{
	"eq": true, "neq": true,
	"contains": true, "not_contains": true,
	"starts_with": true, "ends_with": true,
	"in": true, "not_in": true,
	"present": true, "not_present": true,
}

func ValidateTargetingRules(rulesJSON string) error {
	if rulesJSON == "" || rulesJSON == "[]" {
		return nil
	}
	var rules []TargetingRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return fmt.Errorf("invalid targeting rules JSON: %w", err)
	}
	for i, r := range rules {
		if r.Name == "" {
			return fmt.Errorf("rule %d: name is required", i)
		}
		if r.Variant == "" {
			return fmt.Errorf("rule %d: variant is required", i)
		}
		if r.Match != "all" && r.Match != "any" {
			return fmt.Errorf("rule %d: match must be \"all\" or \"any\"", i)
		}
		if len(r.Conditions) == 0 {
			return fmt.Errorf("rule %d: at least one condition is required", i)
		}
		for j, c := range r.Conditions {
			if c.ContextKey == "" {
				return fmt.Errorf("rule %d, condition %d: context_key is required", i, j)
			}
			if !validOperators[c.Operator] {
				return fmt.Errorf("rule %d, condition %d: unknown operator %q", i, j, c.Operator)
			}
		}
	}
	return nil
}

type FlagService struct {
	store ports.FlagRepo
}

func NewFlagService(store ports.FlagRepo) *FlagService {
	return &FlagService{store: store}
}

func (svc *FlagService) CreateFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error) {
	return svc.store.CreateFlag(ctx, f)
}

func (svc *FlagService) GetFlag(ctx context.Context, id string) (repository.FeatureFlag, error) {
	f, err := svc.store.FlagByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.FeatureFlag{}, fmt.Errorf("%w: flag %s", domain.ErrNotFound, id)
		}
		return repository.FeatureFlag{}, err
	}
	return f, nil
}

func (svc *FlagService) GetFlagByKey(ctx context.Context, projectID, flagKey string) (repository.FeatureFlag, error) {
	f, err := svc.store.FlagByKey(ctx, projectID, flagKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.FeatureFlag{}, fmt.Errorf("%w: flag %s", domain.ErrNotFound, flagKey)
		}
		return repository.FeatureFlag{}, err
	}
	return f, nil
}

func (svc *FlagService) ListFlags(ctx context.Context, projectID string) ([]repository.FeatureFlag, error) {
	return svc.store.ListFlags(ctx, projectID)
}

func (svc *FlagService) UpdateFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error) {
	return svc.store.UpdateFlag(ctx, f)
}

func (svc *FlagService) DeleteFlag(ctx context.Context, id string) error {
	return svc.store.DeleteFlag(ctx, id)
}

func (svc *FlagService) EvaluateFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]any) (FlagEvalResult, error) {
	ctx, span := tracing.StartSpan(ctx, "flags.evaluate",
		attribute.String("flag.key", flagKey),
		attribute.String("project.id", projectID),
	)
	defer span.End()

	flag, err := svc.store.FlagByKey(ctx, projectID, flagKey)
	if err != nil {
		tracing.RecordError(span, err)
		return FlagEvalResult{}, fmt.Errorf("flag not found: %w", err)
	}

	if flag.Status != "active" {
		span.SetAttributes(attribute.String("flag.reason", "DISABLED"))
		val, _ := variantValue(flag.Variants, flag.DefaultVariant)
		return FlagEvalResult{
			Value:   val,
			Variant: flag.DefaultVariant,
			Reason:  "DISABLED",
			FlagKey: flag.FlagKey,
		}, nil
	}

	targetingKey := contextString(evalContext, "targetingKey")
	if targetingKey == "" {
		targetingKey = contextString(evalContext, "targeting_key")
	}
	if targetingKey == "" {
		targetingKey = contextString(evalContext, "session_id")
	}
	sessionID := contextString(evalContext, "session_id")

	ctxKeys := contextKeyNames(evalContext)

	if variant, ruleName, matched := evaluateTargetingRules(flag.TargetingRules, evalContext); matched {
		val, _ := variantValue(flag.Variants, variant)
		span.SetAttributes(
			attribute.String("flag.variant", variant),
			attribute.String("flag.reason", "TARGETING_MATCH"),
			attribute.String("flag.rule_name", ruleName),
		)
		if recErr := svc.store.RecordEvaluation(ctx, repository.FlagEvaluation{
			FlagID:      flag.ID,
			ProjectID:   flag.ProjectID,
			Variant:     variant,
			ContextHash: hashContext(targetingKey),
			SessionID:   sessionID,
			ContextKeys: ctxKeys,
		}); recErr != nil {
			// Best-effort: an evaluation already happened, we just lost the
			// analytics row. Warn so silent storage failures surface.
			slog.WarnContext(ctx, "flag: record evaluation (targeting)",
				"err", recErr, "handled", true,
				"flag_id", flag.ID, "project_id", flag.ProjectID)
		}
		return FlagEvalResult{
			Value:        val,
			Variant:      variant,
			Reason:       "TARGETING_MATCH",
			FlagKey:      flag.FlagKey,
			FlagMetadata: map[string]any{"evaluated_rule_name": ruleName},
		}, nil
	}

	variant := resolveVariant(flag.Split, flag.FlagKey, targetingKey, flag.DefaultVariant)
	val, _ := variantValue(flag.Variants, variant)

	span.SetAttributes(
		attribute.String("flag.variant", variant),
		attribute.String("flag.reason", "SPLIT"),
	)

	if recErr := svc.store.RecordEvaluation(ctx, repository.FlagEvaluation{
		FlagID:      flag.ID,
		ProjectID:   flag.ProjectID,
		Variant:     variant,
		ContextHash: hashContext(targetingKey),
		SessionID:   sessionID,
		ContextKeys: ctxKeys,
	}); recErr != nil {
		slog.WarnContext(ctx, "flag: record evaluation (split)",
			"err", recErr, "handled", true,
			"flag_id", flag.ID, "project_id", flag.ProjectID)
	}

	return FlagEvalResult{
		Value:   val,
		Variant: variant,
		Reason:  "SPLIT",
		FlagKey: flag.FlagKey,
	}, nil
}

// flagKeyRe bounds auto-registration to sane keys so a spam caller can't create
// rows with arbitrary/oversized garbage keys.
var flagKeyRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

func validFlagKey(key string) bool { return flagKeyRe.MatchString(key) }

// inferFlagType maps a JSON-decoded default value to a flag_type for the dashboard.
func inferFlagType(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64, int, int64:
		return "number"
	case string, nil:
		return "string"
	default:
		return "json"
	}
}

// buildAutoFlag describes an inert, auto-created flag: a single "default" variant
// holding the caller's default value, status "inactive" so evaluation returns that
// default (reason DISABLED) until a human configures it.
func buildAutoFlag(projectID, flagKey string, defaultValue any) repository.FeatureFlag {
	variants := map[string]any{"default": defaultValue}
	vb, err := json.Marshal(variants)
	if err != nil {
		vb = []byte(`{"default":null}`)
	}
	return repository.FeatureFlag{
		ProjectID:      projectID,
		FlagKey:        flagKey,
		Name:           flagKey,
		FlagType:       inferFlagType(defaultValue),
		Variants:       string(vb),
		DefaultVariant: "default",
		Split:          "{}",
		TargetingRules: "[]",
		Status:         "inactive",
		Origin:         "auto",
	}
}

// EvaluateOrRegisterFlag evaluates a flag and, when it doesn't exist yet,
// auto-registers an inert flag carrying the caller's default so it surfaces in
// the dashboard ready to configure. The caller always gets its default back in
// that case (reason DISABLED), so SDK behaviour is unchanged.
//
// maxAuto caps auto-created flags per project (0 disables auto-registration).
// Invalid keys, a missing project, or hitting the cap fall back to the original
// not-found behaviour (the cap case via domain.ErrAutoRegisterLimit).
func (svc *FlagService) EvaluateOrRegisterFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]any, defaultValue any, maxAuto int) (FlagEvalResult, error) {
	res, err := svc.EvaluateFlag(ctx, projectID, flagKey, evalContext)
	if err == nil {
		// Keep auto, still-unconfigured flags alive in the retention sweep while
		// they're actively evaluated. DISABLED covers inactive (auto) and paused
		// (manual, never pruned) flags; touchIfAuto filters to origin='auto'.
		if res.Reason == "DISABLED" {
			svc.touchIfAuto(projectID, flagKey)
		}
		return res, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return FlagEvalResult{}, err // a real lookup failure, not a missing flag
	}
	if maxAuto <= 0 || projectID == "" || !validFlagKey(flagKey) {
		return FlagEvalResult{}, err // fall through to FLAG_NOT_FOUND + default
	}
	if n, cerr := svc.store.CountAutoFlags(ctx, projectID); cerr == nil && n >= maxAuto {
		return FlagEvalResult{}, fmt.Errorf("project %s: %w", projectID, domain.ErrAutoRegisterLimit)
	}
	if _, cerr := svc.store.EnsureAutoFlag(ctx, buildAutoFlag(projectID, flagKey, defaultValue)); cerr != nil {
		slog.WarnContext(ctx, "flag: auto-register failed", "err", cerr, "handled", true,
			"project_id", projectID, "flag_key", flagKey)
		// Still hand the caller its default so the SDK is unaffected.
		return FlagEvalResult{Value: defaultValue, Variant: "default", Reason: "DISABLED", FlagKey: flagKey}, nil
	}
	// Re-evaluate now that the inert flag exists (returns default, reason DISABLED).
	return svc.EvaluateFlag(ctx, projectID, flagKey, evalContext)
}

// touchIfAuto best-effort bumps last_evaluated_at for an auto flag, off the
// request path so it never adds latency or fails the evaluation.
func (svc *FlagService) touchIfAuto(projectID, flagKey string) {
	go func() {
		ctx := context.Background()
		f, err := svc.store.FlagByKey(ctx, projectID, flagKey)
		if err != nil || f.Origin != "auto" {
			return
		}
		if err := svc.store.TouchFlagEvaluated(ctx, f.ID); err != nil {
			slog.WarnContext(ctx, "flag: touch last_evaluated_at", "err", err, "handled", true,
				"flag_id", f.ID, "project_id", projectID)
		}
	}()
}

func (svc *FlagService) AnalyzeFlag(ctx context.Context, flag repository.FeatureFlag, from, to time.Time) ([]repository.FlagAnalysisResult, error) {
	ctx, span := tracing.StartSpan(ctx, "flags.analyze",
		attribute.String("flag.id", flag.ID),
		attribute.String("flag.key", flag.FlagKey),
	)
	defer span.End()

	evals, err := svc.store.CountEvaluationsByVariant(ctx, flag.ID, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		return nil, fmt.Errorf("count evaluations: %w", err)
	}

	conversions := make(map[string]int64)
	if flag.ConversionEvent != "" {
		conversions, err = svc.store.CountConversionsByVariant(ctx, flag.ID, flag.ConversionEvent, flag.ProjectID, from, to)
		if err != nil {
			tracing.RecordError(span, err)
			return nil, fmt.Errorf("count conversions: %w", err)
		}
	}

	var results []repository.FlagAnalysisResult
	for variant, sample := range evals {
		conv := conversions[variant]
		rate := 0.0
		if sample > 0 {
			rate = float64(conv) / float64(sample)
		}
		results = append(results, repository.FlagAnalysisResult{
			Variant:     variant,
			Sample:      sample,
			Conversions: conv,
			Rate:        rate,
		})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Variant < results[j].Variant })
	return results, nil
}

// resolveVariant deterministically assigns a variant based on split percentages.
func resolveVariant(splitJSON, flagKey, targetingKey, defaultVariant string) string {
	var split map[string]int
	if err := json.Unmarshal([]byte(splitJSON), &split); err != nil || len(split) == 0 {
		return defaultVariant
	}

	h := sha256.Sum256([]byte(targetingKey + ":" + flagKey))
	bucket := binary.BigEndian.Uint64(h[:8]) % 10000

	keys := make([]string, 0, len(split))
	for k := range split {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var cumulative uint64
	for _, k := range keys {
		cumulative += uint64(split[k]) * 100 // percent → basis points
		if bucket < cumulative {
			return k
		}
	}
	return defaultVariant
}

func variantValue(variantsJSON, variant string) (any, error) {
	var variants map[string]any
	if err := json.Unmarshal([]byte(variantsJSON), &variants); err != nil {
		return nil, err
	}
	v, ok := variants[variant]
	if !ok {
		return nil, fmt.Errorf("variant %q not found", variant)
	}
	return v, nil
}

func hashContext(targetingKey string) string {
	h := sha256.Sum256([]byte(targetingKey))
	return fmt.Sprintf("%x", h[:16])
}

func contextString(ctx map[string]any, key string) string {
	v, ok := ctx[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// contextKeyNames returns a sorted slice of key names from the eval context.
func contextKeyNames(ctx map[string]any) []string {
	if len(ctx) == 0 {
		return nil
	}
	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}
	return keys
}

func (svc *FlagService) ContextKeySuggestions(ctx context.Context, projectID string) ([]repository.ContextKeySuggestion, error) {
	return svc.store.FlagContextKeySuggestions(ctx, projectID)
}

func evaluateTargetingRules(rulesJSON string, ctx map[string]any) (variant, ruleName string, matched bool) {
	if rulesJSON == "" || rulesJSON == "[]" {
		return "", "", false
	}
	var rules []TargetingRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return "", "", false
	}
	for _, rule := range rules {
		if len(rule.Conditions) == 0 {
			continue
		}
		if matchesRule(rule, ctx) {
			return rule.Variant, rule.Name, true
		}
	}
	return "", "", false
}

func matchesRule(rule TargetingRule, ctx map[string]any) bool {
	if rule.Match == "any" {
		for _, c := range rule.Conditions {
			if evaluateCondition(c, ctx) {
				return true
			}
		}
		return false
	}
	for _, c := range rule.Conditions {
		if !evaluateCondition(c, ctx) {
			return false
		}
	}
	return true
}

func evaluateCondition(c TargetingCondition, ctx map[string]any) bool {
	raw, exists := ctx[c.ContextKey]

	if c.Operator == "present" {
		return exists
	}
	if c.Operator == "not_present" {
		return !exists
	}

	if !exists {
		return false
	}

	var actual string
	if s, ok := raw.(string); ok {
		actual = s
	} else {
		actual = fmt.Sprintf("%v", raw)
	}

	switch c.Operator {
	case "eq":
		return actual == c.Value
	case "neq":
		return actual != c.Value
	case "contains":
		return strings.Contains(actual, c.Value)
	case "not_contains":
		return !strings.Contains(actual, c.Value)
	case "starts_with":
		return strings.HasPrefix(actual, c.Value)
	case "ends_with":
		return strings.HasSuffix(actual, c.Value)
	case "in":
		for _, v := range strings.Split(c.Value, ",") {
			if actual == strings.TrimSpace(v) {
				return true
			}
		}
		return false
	case "not_in":
		for _, v := range strings.Split(c.Value, ",") {
			if actual == strings.TrimSpace(v) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
