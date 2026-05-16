package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

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
	return svc.store.FlagByID(ctx, id)
}

func (svc *FlagService) GetFlagByKey(ctx context.Context, projectID, flagKey string) (repository.FeatureFlag, error) {
	return svc.store.FlagByKey(ctx, projectID, flagKey)
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
		_ = svc.store.RecordEvaluation(ctx, repository.FlagEvaluation{
			FlagID:      flag.ID,
			ProjectID:   flag.ProjectID,
			Variant:     variant,
			ContextHash: hashContext(targetingKey),
			SessionID:   sessionID,
			ContextKeys: ctxKeys,
		})
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

	_ = svc.store.RecordEvaluation(ctx, repository.FlagEvaluation{
		FlagID:      flag.ID,
		ProjectID:   flag.ProjectID,
		Variant:     variant,
		ContextHash: hashContext(targetingKey),
		SessionID:   sessionID,
		ContextKeys: ctxKeys,
	})

	return FlagEvalResult{
		Value:   val,
		Variant: variant,
		Reason:  "SPLIT",
		FlagKey: flag.FlagKey,
	}, nil
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
