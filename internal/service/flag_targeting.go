package service

// Targeting rules, variant bucketing and evaluation-context helpers used by
// FlagService's evaluation paths.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
