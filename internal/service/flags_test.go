package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
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
		"targetingKey":     "user-456",
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

func TestEvaluateOrRegisterFlag_CreatesInactiveDefaultFlag(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)

	res, err := svc.EvaluateOrRegisterFlag(context.Background(), "proj-1", "anon_qr_limit",
		map[string]any{"targeting_key": "device-1"}, float64(3), 100, "")
	require.NoError(t, err)
	require.Equal(t, "DISABLED", res.Reason)
	require.Equal(t, float64(3), res.Value)
	require.Equal(t, "anon_qr_limit", res.FlagKey)

	flags, err := svc.ListFlags(context.Background(), "proj-1")
	require.NoError(t, err)
	require.Len(t, flags, 1)
	require.Equal(t, "auto", flags[0].Origin)
	require.Equal(t, "inactive", flags[0].Status)
	require.Equal(t, "number", flags[0].FlagType)
	require.JSONEq(t, `{"default":3}`, flags[0].Variants)
}

func TestEvaluateOrRegisterFlag_Idempotent(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", "pricing_default_billing",
			map[string]any{"targeting_key": "s1"}, "monthly", 100, "")
		require.NoError(t, err)
	}
	flags, err := svc.ListFlags(ctx, "proj-1")
	require.NoError(t, err)
	require.Len(t, flags, 1, "repeated evaluations must not create duplicate flags")
}

func TestEvaluateOrRegisterFlag_CapReached(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	_, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", "flag_a", nil, true, 1, "")
	require.NoError(t, err)

	_, err = svc.EvaluateOrRegisterFlag(ctx, "proj-1", "flag_b", nil, true, 1, "")
	require.ErrorIs(t, err, domain.ErrAutoRegisterLimit)

	flags, _ := svc.ListFlags(ctx, "proj-1")
	require.Len(t, flags, 1, "cap must stop new auto flags from being created")
}

func TestEvaluateOrRegisterFlag_ManualFlagsNotCounted(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	// Three manual flags exist; with a cap of 1, auto-registration should still
	// succeed because manual flags do not count toward the cap.
	for _, k := range []string{"m1", "m2", "m3"} {
		createTestFlag(t, svc, "proj-1", k, map[string]any{"on": true, "off": false}, map[string]int{"on": 100}, "off")
	}
	_, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", "auto_1", nil, true, 1, "")
	require.NoError(t, err)

	// A second auto key now hits the cap of 1 auto flag.
	_, err = svc.EvaluateOrRegisterFlag(ctx, "proj-1", "auto_2", nil, true, 1, "")
	require.ErrorIs(t, err, domain.ErrAutoRegisterLimit)
}

func TestEvaluateOrRegisterFlag_InvalidKeyNotCreated(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	bad := "has spaces & symbols!"
	_, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", bad, nil, true, 100, "")
	require.ErrorIs(t, err, sql.ErrNoRows, "invalid keys fall through to not-found, never created")

	flags, _ := svc.ListFlags(ctx, "proj-1")
	require.Empty(t, flags)
}

func TestEvaluateOrRegisterFlag_DisabledWhenMaxZero(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	_, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", "some_flag", nil, true, 0, "")
	require.ErrorIs(t, err, sql.ErrNoRows, "maxAuto=0 disables auto-registration")

	flags, _ := svc.ListFlags(ctx, "proj-1")
	require.Empty(t, flags)
}

func TestEvaluateOrRegisterFlag_NoProjectSkips(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)

	_, err := svc.EvaluateOrRegisterFlag(context.Background(), "", "some_flag", nil, true, 100, "")
	require.ErrorIs(t, err, sql.ErrNoRows, "an empty project (admin key) must not auto-register")
}

func TestUpdateFlag_ClaimsAutoFlagAsManual(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	_, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", "claim_me", nil, true, 100, "")
	require.NoError(t, err)
	flags, _ := svc.ListFlags(ctx, "proj-1")
	require.Len(t, flags, 1)
	require.Equal(t, "auto", flags[0].Origin)

	f := flags[0]
	f.Status = "active"
	updated, err := svc.UpdateFlag(ctx, f)
	require.NoError(t, err)
	require.Equal(t, "manual", updated.Origin, "a human edit claims the flag")
}

// ---------------------------------------------------------------------------
// Config-kind flags
// ---------------------------------------------------------------------------

func createConfigFlag(t *testing.T, svc *service.FlagService, projectID, flagKey string, value any) repository.FeatureFlag {
	t.Helper()
	variantsJSON, _ := json.Marshal(map[string]any{"default": value})
	f, err := svc.CreateFlag(context.Background(), repository.FeatureFlag{
		ProjectID:      projectID,
		FlagKey:        flagKey,
		Name:           flagKey,
		FlagType:       "number",
		Variants:       string(variantsJSON),
		DefaultVariant: "default",
		Split:          `{"default":100}`,
		TargetingRules: "[]",
		Status:         "active",
		Kind:           repository.FlagKindConfig,
	})
	require.NoError(t, err)
	return f
}

// A server polling a config value must not write an evaluation row per read —
// one value polled every 60s from 3 pods would otherwise write ~4.3k rows/day.
func TestEvaluateFlag_ConfigKindRecordsNothing(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	createConfigFlag(t, svc, "proj-1", "cold_email_daily_cap", float64(250))

	for i := 0; i < 25; i++ {
		res, err := svc.EvaluateFlag(context.Background(), "proj-1", "cold_email_daily_cap", nil)
		require.NoError(t, err)
		require.Equal(t, float64(250), res.Value)
		require.Equal(t, "STATIC", res.Reason)
	}

	require.Empty(t, store.RecordedEvaluations(),
		"a config flag must not write evaluation rows")
}

// An experiment is the opposite: every read is a data point.
func TestEvaluateFlag_ExperimentKindStillRecords(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	createTestFlag(t, svc, "proj-1", "my-flag",
		map[string]any{"on": true, "off": false},
		map[string]int{"on": 50, "off": 50}, "off")

	for i := 0; i < 3; i++ {
		_, err := svc.EvaluateFlag(context.Background(), "proj-1", "my-flag",
			map[string]any{"targetingKey": "user-1"})
		require.NoError(t, err)
	}
	require.Len(t, store.RecordedEvaluations(), 3)
}

// Config values are the same for everyone. Bucketing by targeting key would let
// separate pods read different values for one setting.
func TestEvaluateFlag_ConfigKindIgnoresBucketing(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	variantsJSON, _ := json.Marshal(map[string]any{"low": float64(10), "high": float64(999)})
	_, err := svc.CreateFlag(context.Background(), repository.FeatureFlag{
		ProjectID:      "proj-1",
		FlagKey:        "batch_size",
		Name:           "batch_size",
		FlagType:       "number",
		Variants:       string(variantsJSON),
		DefaultVariant: "low",
		Split:          `{"low":50,"high":50}`,
		TargetingRules: "[]",
		Status:         "active",
		Kind:           repository.FlagKindConfig,
	})
	require.NoError(t, err)

	for _, pod := range []string{"pod-a", "pod-b", "pod-c", "pod-d"} {
		res, err := svc.EvaluateFlag(context.Background(), "proj-1", "batch_size",
			map[string]any{"targetingKey": pod})
		require.NoError(t, err)
		require.Equal(t, float64(10), res.Value, "every caller must see the default variant")
		require.Equal(t, "low", res.Variant)
	}
}

// The cache hint is the one sanctioned polling interval. Experiments advertise
// 0 — caching one would mis-bucket and silently drop the analytics.
func TestEvaluateFlag_CacheHint(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store).WithConfigCacheTTL(90 * time.Second)
	createConfigFlag(t, svc, "proj-1", "cfg", float64(1))
	createTestFlag(t, svc, "proj-1", "exp",
		map[string]any{"on": true, "off": false},
		map[string]int{"on": 100}, "on")

	cfg, err := svc.EvaluateFlag(context.Background(), "proj-1", "cfg", nil)
	require.NoError(t, err)
	require.Equal(t, 90, cfg.CacheMaxAgeSeconds)

	exp, err := svc.EvaluateFlag(context.Background(), "proj-1", "exp", nil)
	require.NoError(t, err)
	require.Zero(t, exp.CacheMaxAgeSeconds)
}

// An inactive config flag still returns the flag's own stored default and still
// advertises its cache hint — that is how a value is configured from the
// dashboard before anyone activates it.
func TestEvaluateFlag_ConfigKindDisabledKeepsHint(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	f := createConfigFlag(t, svc, "proj-1", "cap", float64(7))
	f.Status = "inactive"
	_, err := svc.UpdateFlag(context.Background(), f)
	require.NoError(t, err)

	res, err := svc.EvaluateFlag(context.Background(), "proj-1", "cap", nil)
	require.NoError(t, err)
	require.Equal(t, "DISABLED", res.Reason)
	require.Equal(t, float64(7), res.Value)
	require.Equal(t, int(service.DefaultConfigCacheTTL.Seconds()), res.CacheMaxAgeSeconds)
	require.Empty(t, store.RecordedEvaluations())
}

// A caller can declare the kind on first evaluation so the auto-registered flag
// lands in the dashboard already shaped as a config value.
func TestEvaluateOrRegisterFlag_HonoursDeclaredKind(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)

	res, err := svc.EvaluateOrRegisterFlag(context.Background(), "proj-1", "daily_cap",
		nil, float64(500), 100, repository.FlagKindConfig)
	require.NoError(t, err)
	require.Equal(t, float64(500), res.Value)

	f, err := svc.GetFlagByKey(context.Background(), "proj-1", "daily_cap")
	require.NoError(t, err)
	require.Equal(t, repository.FlagKindConfig, f.Kind)
	require.Equal(t, "number", f.FlagType)

	// Unset (and unknown) kinds stay experiments — the pre-existing behaviour.
	_, err = svc.EvaluateOrRegisterFlag(context.Background(), "proj-1", "other", nil, true, 100, "")
	require.NoError(t, err)
	other, err := svc.GetFlagByKey(context.Background(), "proj-1", "other")
	require.NoError(t, err)
	require.Equal(t, repository.FlagKindExperiment, other.Kind)
}

// The two flags responsible for every evaluation in production are manual and
// active, so they returned a real reason (not DISABLED) from an origin the
// touch filtered out — and read as never-evaluated forever.
func TestEvaluateOrRegisterFlag_TouchesLiveManualFlag(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	f := createTestFlag(t, svc, "proj-1", "iambarn-enabled",
		map[string]any{"on": true, "off": false},
		map[string]int{"on": 100}, "off")
	require.Equal(t, "manual", f.Origin)
	require.Nil(t, f.LastEvaluatedAt)

	res, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", "iambarn-enabled",
		map[string]any{"targeting_key": "device-1"}, false, 100, "")
	require.NoError(t, err)
	require.NotEqual(t, "DISABLED", res.Reason, "an active flag returns a real reason")

	// The touch is deliberately off the request path, so wait for it.
	require.Eventually(t, func() bool {
		got, err := store.FlagByID(ctx, f.ID)
		return err == nil && got.LastEvaluatedAt != nil
	}, 2*time.Second, 5*time.Millisecond, "last_evaluated_at was never set for a live manual flag")
}

// Auto flags still get their touch — the retention sweep depends on it.
func TestEvaluateOrRegisterFlag_StillTouchesAutoFlag(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	_, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", "anon_qr_limit",
		map[string]any{"targeting_key": "device-1"}, float64(3), 100, "")
	require.NoError(t, err)

	flags, err := svc.ListFlags(ctx, "proj-1")
	require.NoError(t, err)
	require.Len(t, flags, 1)
	require.Equal(t, "auto", flags[0].Origin)

	require.Eventually(t, func() bool {
		got, err := store.FlagByID(ctx, flags[0].ID)
		return err == nil && got.LastEvaluatedAt != nil
	}, 2*time.Second, 5*time.Millisecond, "last_evaluated_at was never set for an auto flag")
}

// A flag evaluated repeatedly inside the throttle window produces one write,
// not one per evaluation.
func TestEvaluateOrRegisterFlag_TouchIsThrottled(t *testing.T) {
	store := mock.New()
	svc := service.NewFlagService(store)
	ctx := context.Background()

	f := createTestFlag(t, svc, "proj-1", "busy-flag",
		map[string]any{"on": true, "off": false},
		map[string]int{"on": 100}, "off")

	_, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", "busy-flag",
		map[string]any{"targeting_key": "device-1"}, false, 100, "")
	require.NoError(t, err)

	var first time.Time
	require.Eventually(t, func() bool {
		got, err := store.FlagByID(ctx, f.ID)
		if err != nil || got.LastEvaluatedAt == nil {
			return false
		}
		first = *got.LastEvaluatedAt
		return true
	}, 2*time.Second, 5*time.Millisecond)

	// Every further evaluation in this window must be skipped before the
	// goroutine is even started, so the stored timestamp cannot move.
	for range 50 {
		_, err := svc.EvaluateOrRegisterFlag(ctx, "proj-1", "busy-flag",
			map[string]any{"targeting_key": "device-1"}, false, 100, "")
		require.NoError(t, err)
	}
	got, err := store.FlagByID(ctx, f.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastEvaluatedAt)
	require.True(t, got.LastEvaluatedAt.Equal(first),
		"throttled evaluations still wrote: %v != %v", *got.LastEvaluatedAt, first)
}
