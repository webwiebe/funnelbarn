package service

// Targeting rules, variant bucketing and evaluation-context helpers used by
// FlagService's evaluation paths.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

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
		if err := validateTargetingRule(i, r); err != nil {
			return err
		}
	}
	return nil
}

func validateTargetingRule(i int, r TargetingRule) error {
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
	return nil
}

// ValidateRuleVariants reports the first rule naming a variant the flag does
// not have. Such a rule would match and then have no value to serve.
func ValidateRuleVariants(rulesJSON, variantsJSON string) error {
	if rulesJSON == "" || rulesJSON == "[]" {
		return nil
	}
	var rules []TargetingRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return fmt.Errorf("invalid targeting rules JSON: %w", err)
	}
	var variants map[string]any
	if variantsJSON != "" {
		if err := json.Unmarshal([]byte(variantsJSON), &variants); err != nil {
			return fmt.Errorf("invalid variants JSON: %w", err)
		}
	}
	for i, r := range rules {
		if _, ok := variants[r.Variant]; !ok {
			return fmt.Errorf("rule %d: variant %q does not exist (have: %s)", i, r.Variant, variantNames(variants))
		}
	}
	return nil
}

func variantNames(variants map[string]any) string {
	if len(variants) == 0 {
		return "none"
	}
	names := make([]string, 0, len(variants))
	for name := range variants {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// validateFlagRules checks the rules' syntax and that each names a variant the
// flag has.
func validateFlagRules(rulesJSON, variantsJSON string) error {
	if err := ValidateTargetingRules(rulesJSON); err != nil {
		return err
	}
	return ValidateRuleVariants(rulesJSON, variantsJSON)
}

// validateUpdatedRules applies validateFlagRules to an update. A flag stored
// with a rule naming a missing variant has to stay editable, so its owner can
// still pause it or change a value. The check passes when the rules are
// unchanged and the update reports the same missing variant against the same
// variant names; editing the rules or the set of names has to repair it.
func (svc *FlagService) validateUpdatedRules(ctx context.Context, f repository.FeatureFlag) error {
	err := validateFlagRules(f.TargetingRules, f.Variants)
	if err == nil {
		return nil
	}
	existing, lookupErr := svc.store.FlagByID(ctx, f.ID)
	if lookupErr != nil || existing.TargetingRules != f.TargetingRules {
		return err
	}
	if stored := validateFlagRules(existing.TargetingRules, existing.Variants); stored != nil && stored.Error() == err.Error() {
		return nil
	}
	return err
}

// missingVariantReportInterval limits the Error for a rule naming a missing
// variant to one per flag per hour on each instance. The rule stays broken until
// someone repairs it and a poller can hit it on every read, while BugBarn needs
// only enough events to keep the issue current.
const missingVariantReportInterval = time.Hour

// missingVariantReports is that limit's state. The zero value is ready.
type missingVariantReports struct {
	mu sync.Mutex
	at map[string]time.Time
}

// due reports whether this flag is due for an Error, recording the decision so
// the next evaluation within the interval skips it. The map is bounded the same
// way as the touch throttle.
func (r *missingVariantReports) due(flagID string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if last, ok := r.at[flagID]; ok && now.Sub(last) < missingVariantReportInterval {
		return false
	}
	if r.at == nil || len(r.at) >= maxTrackedTouches {
		r.at = make(map[string]time.Time)
	}
	r.at[flagID] = now
	return true
}

// ruleVariantMissing answers a matched rule whose variant does not exist. Only a
// rule stored before the write check can get here. It serves the flag's default
// value with reason ERROR, so an SDK falls back to its caller's default. It
// records no evaluation, and a zero cache hint keeps the error result out of
// every cache. Serving a default in silence is how this fault hid, so it logs at
// Error to reach BugBarn.
func (svc *FlagService) ruleVariantMissing(ctx context.Context, span trace.Span, flag repository.FeatureFlag, ruleName string, cause error) FlagEvalResult {
	span.SetAttributes(
		attribute.String("flag.reason", "ERROR"),
		attribute.String("flag.error_code", "GENERAL"),
		attribute.String("flag.rule_name", ruleName),
	)
	if svc.missingVariants.due(flag.ID, time.Now()) {
		slog.ErrorContext(ctx, "flag: targeting rule names a missing variant",
			"err", cause, "handled", true,
			"project_id", flag.ProjectID, "flag_key", flag.FlagKey, "rule", ruleName)
	}
	val, _ := variantValue(flag.Variants, flag.DefaultVariant)
	return FlagEvalResult{
		Value:        val,
		Variant:      flag.DefaultVariant,
		Reason:       "ERROR",
		FlagKey:      flag.FlagKey,
		ErrorCode:    "GENERAL",
		FlagMetadata: map[string]any{"evaluated_rule_name": ruleName},
	}
}

// InferFlagType maps a JSON-decoded default value to a flag_type for the
// dashboard. Exported so callers outside the service (e.g. the MCP tools'
// create_flag) can build a repository.FeatureFlag from a bare default value
// the same way EvaluateOrRegisterFlag's auto-registration does.
func InferFlagType(v any) string {
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
	sort.Strings(keys)
	return keys
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

	switch c.Operator {
	case "present":
		return exists
	case "not_present":
		return !exists
	}
	if !exists {
		return false
	}
	match, ok := conditionMatchers[c.Operator]
	if !ok {
		return false
	}
	actual, isString := raw.(string)
	if !isString {
		actual = fmt.Sprintf("%v", raw)
	}
	return match(actual, c.Value)
}

// conditionMatchers compares a context value against a condition value, one
// entry per value-based operator in validOperators.
var conditionMatchers = map[string]func(actual, want string) bool{
	"eq":           func(a, w string) bool { return a == w },
	"neq":          func(a, w string) bool { return a != w },
	"contains":     strings.Contains,
	"not_contains": func(a, w string) bool { return !strings.Contains(a, w) },
	"starts_with":  strings.HasPrefix,
	"ends_with":    strings.HasSuffix,
	"in":           inList,
	"not_in":       func(a, w string) bool { return !inList(a, w) },
}

// inList reports whether actual equals one of the comma-separated values.
func inList(actual, list string) bool {
	for _, v := range strings.Split(list, ",") {
		if actual == strings.TrimSpace(v) {
			return true
		}
	}
	return false
}
