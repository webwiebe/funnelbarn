package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/mock"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func createTestFlag(t *testing.T, svc *service.FlagService, projectID string, flagKey string, variants map[string]any, split map[string]int, defaultVariant string) repository.FeatureFlag {
	t.Helper()
	variantsJSON, _ := json.Marshal(variants)
	splitJSON, _ := json.Marshal(split)
	f, err := svc.CreateFlag(context.Background(), repository.FeatureFlag{
		ProjectID:      projectID,
		FlagKey:        flagKey,
		Name:           flagKey,
		FlagType:       "boolean",
		Variants:       string(variantsJSON),
		DefaultVariant: defaultVariant,
		Split:          string(splitJSON),
		TargetingRules: "[]",
		Status:         "active",
	})
	require.NoError(t, err)
	return f
}

func TestFlagService_EvaluateFlag_Split(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	createTestFlag(t, svc, "proj-1", "my-flag",
		map[string]any{"on": true, "off": false},
		map[string]int{"on": 50, "off": 50},
		"off",
	)

	result, err := svc.EvaluateFlag(context.Background(), "proj-1", "my-flag", map[string]any{
		"targetingKey": "user-123",
	})
	require.NoError(t, err)
	require.Equal(t, "SPLIT", result.Reason)
	require.Equal(t, "my-flag", result.FlagKey)
	require.Contains(t, []string{"on", "off"}, result.Variant)
}

func TestFlagService_EvaluateFlag_Disabled(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	variantsJSON, _ := json.Marshal(map[string]any{"on": true, "off": false})
	splitJSON, _ := json.Marshal(map[string]int{"on": 50, "off": 50})
	_, err := svc.CreateFlag(context.Background(), repository.FeatureFlag{
		ProjectID:      "proj-1",
		FlagKey:        "disabled-flag",
		Name:           "disabled-flag",
		FlagType:       "boolean",
		Variants:       string(variantsJSON),
		DefaultVariant: "off",
		Split:          string(splitJSON),
		TargetingRules: "[]",
		Status:         "paused",
	})
	require.NoError(t, err)

	result, err := svc.EvaluateFlag(context.Background(), "proj-1", "disabled-flag", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, "DISABLED", result.Reason)
	require.Equal(t, "off", result.Variant)
	require.Equal(t, false, result.Value)
}

func TestFlagService_EvaluateFlag_NotFound(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)

	_, err := svc.EvaluateFlag(context.Background(), "proj-1", "nonexistent", map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "flag not found")
}

func TestFlagService_EvaluateFlag_TargetingMatch(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)

	rules := []service.TargetingRule{
		{
			Name:    "Developer Override",
			Variant: "off",
			Match:   "all",
			Conditions: []service.TargetingCondition{
				{ContextKey: "bypassLaunchGate", Operator: "eq", Value: "true"},
			},
		},
	}
	rulesJSON, _ := json.Marshal(rules)

	variantsJSON, _ := json.Marshal(map[string]any{"on": true, "off": false})
	splitJSON, _ := json.Marshal(map[string]int{"on": 100})
	_, err := svc.CreateFlag(context.Background(), repository.FeatureFlag{
		ProjectID:      "proj-1",
		FlagKey:        "launch-gate",
		Name:           "Launch Gate",
		FlagType:       "boolean",
		Variants:       string(variantsJSON),
		DefaultVariant: "on",
		Split:          string(splitJSON),
		TargetingRules: string(rulesJSON),
		Status:         "active",
	})
	require.NoError(t, err)

	result, err := svc.EvaluateFlag(context.Background(), "proj-1", "launch-gate", map[string]any{
		"targetingKey":      "user-456",
		"bypassLaunchGate": "true",
	})
	require.NoError(t, err)
	require.Equal(t, "TARGETING_MATCH", result.Reason)
	require.Equal(t, "off", result.Variant)
	require.Equal(t, false, result.Value)
	require.Equal(t, "Developer Override", result.FlagMetadata["evaluated_rule_name"])
}

func TestFlagService_EvaluateFlag_TargetingNoMatch_FallsThrough(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)

	rules := []service.TargetingRule{
		{
			Name:    "Developer Override",
			Variant: "off",
			Match:   "all",
			Conditions: []service.TargetingCondition{
				{ContextKey: "bypassLaunchGate", Operator: "eq", Value: "true"},
			},
		},
	}
	rulesJSON, _ := json.Marshal(rules)

	variantsJSON, _ := json.Marshal(map[string]any{"on": true, "off": false})
	splitJSON, _ := json.Marshal(map[string]int{"on": 100})
	_, err := svc.CreateFlag(context.Background(), repository.FeatureFlag{
		ProjectID:      "proj-1",
		FlagKey:        "launch-gate",
		Name:           "Launch Gate",
		FlagType:       "boolean",
		Variants:       string(variantsJSON),
		DefaultVariant: "on",
		Split:          string(splitJSON),
		TargetingRules: string(rulesJSON),
		Status:         "active",
	})
	require.NoError(t, err)

	result, err := svc.EvaluateFlag(context.Background(), "proj-1", "launch-gate", map[string]any{
		"targetingKey": "user-789",
	})
	require.NoError(t, err)
	require.Equal(t, "SPLIT", result.Reason)
	require.Equal(t, "on", result.Variant)
}

func TestFlagService_EvaluateFlag_TargetingAnyMatch(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)

	rules := []service.TargetingRule{
		{
			Name:    "Internal Users",
			Variant: "on",
			Match:   "any",
			Conditions: []service.TargetingCondition{
				{ContextKey: "email", Operator: "ends_with", Value: "@wiebe.xyz"},
				{ContextKey: "email", Operator: "ends_with", Value: "@funnelbarn.com"},
			},
		},
	}
	rulesJSON, _ := json.Marshal(rules)
	variantsJSON, _ := json.Marshal(map[string]any{"on": true, "off": false})
	splitJSON, _ := json.Marshal(map[string]int{"off": 100})
	_, err := svc.CreateFlag(context.Background(), repository.FeatureFlag{
		ProjectID:      "proj-1",
		FlagKey:        "beta-feature",
		Name:           "Beta Feature",
		FlagType:       "boolean",
		Variants:       string(variantsJSON),
		DefaultVariant: "off",
		Split:          string(splitJSON),
		TargetingRules: string(rulesJSON),
		Status:         "active",
	})
	require.NoError(t, err)

	result, err := svc.EvaluateFlag(context.Background(), "proj-1", "beta-feature", map[string]any{
		"targetingKey": "user-internal",
		"email":        "dev@funnelbarn.com",
	})
	require.NoError(t, err)
	require.Equal(t, "TARGETING_MATCH", result.Reason)
	require.Equal(t, "on", result.Variant)
}

func TestValidateTargetingRules(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty string", "", false},
		{"empty array", "[]", false},
		{"valid rule", `[{"name":"test","variant":"on","match":"all","conditions":[{"context_key":"x","operator":"eq","value":"1"}]}]`, false},
		{"missing name", `[{"name":"","variant":"on","match":"all","conditions":[{"context_key":"x","operator":"eq","value":"1"}]}]`, true},
		{"missing variant", `[{"name":"test","variant":"","match":"all","conditions":[{"context_key":"x","operator":"eq","value":"1"}]}]`, true},
		{"invalid match", `[{"name":"test","variant":"on","match":"none","conditions":[{"context_key":"x","operator":"eq","value":"1"}]}]`, true},
		{"no conditions", `[{"name":"test","variant":"on","match":"all","conditions":[]}]`, true},
		{"unknown operator", `[{"name":"test","variant":"on","match":"all","conditions":[{"context_key":"x","operator":"regex","value":".*"}]}]`, true},
		{"invalid json", `not json`, true},
		{"missing context_key", `[{"name":"test","variant":"on","match":"all","conditions":[{"context_key":"","operator":"eq","value":"1"}]}]`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.ValidateTargetingRules(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFlagService_EvaluateFlag_Operators(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		value    string
		ctx      map[string]any
		match    bool
	}{
		{"eq match", "eq", "hello", map[string]any{"x": "hello"}, true},
		{"eq no match", "eq", "hello", map[string]any{"x": "world"}, false},
		{"neq match", "neq", "hello", map[string]any{"x": "world"}, true},
		{"neq no match", "neq", "hello", map[string]any{"x": "hello"}, false},
		{"contains match", "contains", "ell", map[string]any{"x": "hello"}, true},
		{"contains no match", "contains", "xyz", map[string]any{"x": "hello"}, false},
		{"not_contains match", "not_contains", "xyz", map[string]any{"x": "hello"}, true},
		{"not_contains no match", "not_contains", "ell", map[string]any{"x": "hello"}, false},
		{"starts_with match", "starts_with", "hel", map[string]any{"x": "hello"}, true},
		{"starts_with no match", "starts_with", "wor", map[string]any{"x": "hello"}, false},
		{"ends_with match", "ends_with", "llo", map[string]any{"x": "hello"}, true},
		{"ends_with no match", "ends_with", "wor", map[string]any{"x": "hello"}, false},
		{"in match", "in", "a,hello,b", map[string]any{"x": "hello"}, true},
		{"in no match", "in", "a,b,c", map[string]any{"x": "hello"}, false},
		{"not_in match", "not_in", "a,b,c", map[string]any{"x": "hello"}, true},
		{"not_in no match", "not_in", "a,hello,b", map[string]any{"x": "hello"}, false},
		{"present match", "present", "", map[string]any{"x": "anything"}, true},
		{"present no match", "present", "", map[string]any{}, false},
		{"not_present match", "not_present", "", map[string]any{}, true},
		{"not_present no match", "not_present", "", map[string]any{"x": "anything"}, false},
		{"numeric coercion", "eq", "42", map[string]any{"x": 42}, true},
		{"missing key", "eq", "hello", map[string]any{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := mock.New()
			svc := service.NewFlagService(store)

			rules := []service.TargetingRule{
				{
					Name:    "test-rule",
					Variant: "on",
					Match:   "all",
					Conditions: []service.TargetingCondition{
						{ContextKey: "x", Operator: tt.operator, Value: tt.value},
					},
				},
			}
			rulesJSON, _ := json.Marshal(rules)
			variantsJSON, _ := json.Marshal(map[string]any{"on": true, "off": false})
			splitJSON, _ := json.Marshal(map[string]int{"off": 100})

			_, err := svc.CreateFlag(context.Background(), repository.FeatureFlag{
				ProjectID:      "proj-1",
				FlagKey:        "op-test-" + tt.name,
				Name:           "op-test",
				FlagType:       "boolean",
				Variants:       string(variantsJSON),
				DefaultVariant: "off",
				Split:          string(splitJSON),
				TargetingRules: string(rulesJSON),
				Status:         "active",
			})
			require.NoError(t, err)

			tt.ctx["targetingKey"] = "user-1"
			result, err := svc.EvaluateFlag(context.Background(), "proj-1", "op-test-"+tt.name, tt.ctx)
			require.NoError(t, err)

			if tt.match {
				require.Equal(t, "TARGETING_MATCH", result.Reason)
				require.Equal(t, "on", result.Variant)
			} else {
				require.Equal(t, "SPLIT", result.Reason)
				require.Equal(t, "off", result.Variant)
			}
		})
	}
}

func TestFlagService_CRUD(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	f := createTestFlag(t, svc, "proj-1", "test-crud",
		map[string]any{"on": true, "off": false},
		map[string]int{"on": 50, "off": 50},
		"off",
	)

	got, err := svc.GetFlag(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, f.FlagKey, got.FlagKey)

	gotByKey, err := svc.GetFlagByKey(ctx, "proj-1", "test-crud")
	require.NoError(t, err)
	require.Equal(t, f.ID, gotByKey.ID)

	list, err := svc.ListFlags(ctx, "proj-1")
	require.NoError(t, err)
	require.Len(t, list, 1)

	updated, err := svc.UpdateFlag(ctx, repository.FeatureFlag{
		ID:             f.ID,
		Name:           "Updated Name",
		Variants:       f.Variants,
		DefaultVariant: f.DefaultVariant,
		Split:          f.Split,
		TargetingRules: f.TargetingRules,
		Status:         "active",
	})
	require.NoError(t, err)
	require.Equal(t, "Updated Name", updated.Name)

	err = svc.DeleteFlag(ctx, f.ID)
	require.NoError(t, err)

	list, err = svc.ListFlags(ctx, "proj-1")
	require.NoError(t, err)
	require.Len(t, list, 0)
}

func TestFlagService_DeterministicSplit(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	createTestFlag(t, svc, "proj-1", "deterministic-flag",
		map[string]any{"on": true, "off": false},
		map[string]int{"on": 50, "off": 50},
		"off",
	)

	r1, err := svc.EvaluateFlag(context.Background(), "proj-1", "deterministic-flag", map[string]any{"targetingKey": "same-user"})
	require.NoError(t, err)
	r2, err := svc.EvaluateFlag(context.Background(), "proj-1", "deterministic-flag", map[string]any{"targetingKey": "same-user"})
	require.NoError(t, err)
	require.Equal(t, r1.Variant, r2.Variant)
}
