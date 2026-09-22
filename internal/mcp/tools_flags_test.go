package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// createManualFlag inserts a flag straight through the store (bypassing
// FlagService's validation, which the create_flag tool tests separately) so
// setup can freely shape variants/split/kind for whatever a test needs.
func createManualFlag(t *testing.T, env *testEnv, projectID, flagKey string, opts func(*repository.FeatureFlag)) repository.FeatureFlag {
	t.Helper()
	f := repository.FeatureFlag{
		ProjectID:      projectID,
		FlagKey:        flagKey,
		Name:           flagKey,
		FlagType:       "boolean",
		Variants:       `{"on":true,"off":false}`,
		DefaultVariant: "off",
		Split:          `{"on":50,"off":50}`,
		TargetingRules: "[]",
		Status:         "active",
	}
	if opts != nil {
		opts(&f)
	}
	created, err := env.Store.CreateFlag(context.Background(), f)
	if err != nil {
		t.Fatalf("CreateFlag(%s): %v", flagKey, err)
	}
	return created
}

// flagEvalCount counts every row ever written to flag_evaluations, across all
// projects and flags.
func flagEvalCount(t *testing.T, env *testEnv) int {
	t.Helper()
	var n int
	if err := env.Store.DB().QueryRow(`SELECT COUNT(*) FROM flag_evaluations`).Scan(&n); err != nil {
		t.Fatalf("count flag_evaluations: %v", err)
	}
	return n
}

func unmarshalVariants(t *testing.T, raw string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal variants %q: %v", raw, err)
	}
	return v
}

// ---------------------------------------------------------------------------
// list_flags
// ---------------------------------------------------------------------------

func TestListFlagsTool(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "flag_a", nil)
	createManualFlag(t, env, p.ID, "flag_b", func(f *repository.FeatureFlag) {
		f.Kind = repository.FlagKindConfig
	})

	cs := env.connect(t, tokenRead, p.Slug)
	var out listFlagsOut
	callOK(t, cs, "list_flags", nil, &out)

	if len(out.Flags) != 2 {
		t.Fatalf("got %d flags, want 2: %+v", len(out.Flags), out.Flags)
	}
	byKey := map[string]flagDetail{}
	for _, f := range out.Flags {
		byKey[f.FlagKey] = f
	}
	a, ok := byKey["flag_a"]
	if !ok {
		t.Fatalf("flag_a missing from list: %+v", out.Flags)
	}
	if a.Origin != "manual" || a.Status != "active" || a.Kind != repository.FlagKindExperiment {
		t.Errorf("flag_a: got %+v", a)
	}
	b, ok := byKey["flag_b"]
	if !ok || b.Kind != repository.FlagKindConfig {
		t.Errorf("flag_b missing or wrong kind: %+v", b)
	}
}

func TestListFlagsTool_ScopedToProject(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	createManualFlag(t, env, a.ID, "in_a", nil)
	createManualFlag(t, env, b.ID, "in_b", nil)

	cs := env.connect(t, tokenReadWrite, a.Slug)
	var out listFlagsOut
	callOK(t, cs, "list_flags", nil, &out)
	if len(out.Flags) != 1 || out.Flags[0].FlagKey != "in_a" {
		t.Errorf("got %+v, want only in_a", out.Flags)
	}
}

// ---------------------------------------------------------------------------
// get_flag
// ---------------------------------------------------------------------------

func TestGetFlagTool_ByKey(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "get_me", nil)

	cs := env.connect(t, tokenRead, p.Slug)
	var out flagDetail
	callOK(t, cs, "get_flag", map[string]any{"flag_key": "get_me"}, &out)
	if out.FlagKey != "get_me" {
		t.Fatalf("got %+v", out)
	}
}

func TestGetFlagTool_ByID(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	f := createManualFlag(t, env, p.ID, "get_me2", nil)

	cs := env.connect(t, tokenRead, p.Slug)
	var out flagDetail
	callOK(t, cs, "get_flag", map[string]any{"flag_id": f.ID}, &out)
	if out.ID != f.ID || out.FlagKey != "get_me2" {
		t.Fatalf("got %+v", out)
	}
}

func TestGetFlagTool_MissingKeyAndID(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, p.Slug)

	msg := callErr(t, cs, "get_flag", nil)
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestGetFlagTool_CrossProjectNotFoundByKey(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	createManualFlag(t, env, b.ID, "in_b", nil)

	cs := env.connect(t, tokenRead, a.Slug)
	msg := callErr(t, cs, "get_flag", map[string]any{"flag_key": "in_b"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestGetFlagTool_CrossProjectNotFoundByID(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	f := createManualFlag(t, env, b.ID, "in_b", nil)

	cs := env.connect(t, tokenRead, a.Slug)
	msg := callErr(t, cs, "get_flag", map[string]any{"flag_id": f.ID})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// create_flag
// ---------------------------------------------------------------------------

func TestCreateFlagTool_RequiresWriteScope(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, p.Slug)

	msg := callErr(t, cs, "create_flag", map[string]any{"flag_key": "x"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestCreateFlagTool_Success(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, p.Slug)

	var out flagDetail
	callOK(t, cs, "create_flag", map[string]any{
		"flag_key":      "new_checkout",
		"description":   "New Checkout",
		"default_value": false,
	}, &out)

	if out.FlagKey != "new_checkout" || out.Name != "New Checkout" {
		t.Fatalf("got %+v", out)
	}
	if out.Origin != "manual" {
		t.Errorf("origin = %q, want manual", out.Origin)
	}
	if out.Status != "active" {
		t.Errorf("status = %q, want active", out.Status)
	}
	if out.Kind != repository.FlagKindExperiment {
		t.Errorf("flag_kind = %q, want experiment (default)", out.Kind)
	}

	// Acceptance criterion: SDK evaluation returns the created flag's value.
	res, err := env.Deps.Flags.EvaluateFlag(context.Background(), p.ID, "new_checkout", nil)
	if err != nil {
		t.Fatalf("EvaluateFlag: %v", err)
	}
	if res.Value != false {
		t.Errorf("evaluated value = %v, want false", res.Value)
	}
}

func TestCreateFlagTool_DefaultsNameToKey(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, p.Slug)

	var out flagDetail
	callOK(t, cs, "create_flag", map[string]any{"flag_key": "bare_key"}, &out)
	if out.Name != "bare_key" {
		t.Errorf("name = %q, want bare_key", out.Name)
	}
}

func TestCreateFlagTool_MissingFlagKey(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, p.Slug)

	msg := callErr(t, cs, "create_flag", map[string]any{"description": "no key"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestCreateFlagTool_InvalidKind(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, p.Slug)

	msg := callErr(t, cs, "create_flag", map[string]any{"flag_key": "k", "flag_kind": "bogus"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestCreateFlagTool_ConfigKind(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, p.Slug)

	var out flagDetail
	callOK(t, cs, "create_flag", map[string]any{
		"flag_key": "daily_cap", "flag_kind": "config", "default_value": float64(250),
	}, &out)
	if out.Kind != repository.FlagKindConfig {
		t.Errorf("flag_kind = %q, want config", out.Kind)
	}
	variants := unmarshalVariants(t, out.Variants)
	if variants["default"] != float64(250) {
		t.Errorf("variants = %v, want default=250", variants)
	}
}

func TestCreateFlagTool_InvalidTargetingRules(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, p.Slug)

	msg := callErr(t, cs, "create_flag", map[string]any{"flag_key": "k", "targeting_rules": "not json"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// update_flag
// ---------------------------------------------------------------------------

func TestUpdateFlagTool_RequiresWriteScope(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	f := createManualFlag(t, env, p.ID, "k", nil)
	cs := env.connect(t, tokenRead, p.Slug)

	msg := callErr(t, cs, "update_flag", map[string]any{"flag_id": f.ID, "status": "paused"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestUpdateFlagTool_ByFlagKey_UpdatesDefaultValue(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "cap", func(f *repository.FeatureFlag) {
		f.Variants = `{"default":10}`
		f.DefaultVariant = "default"
		f.Split = `{"default":100}`
		f.Kind = repository.FlagKindConfig
		f.FlagType = "number"
	})
	cs := env.connect(t, tokenReadWrite, p.Slug)

	var out flagDetail
	callOK(t, cs, "update_flag", map[string]any{
		"flag_key": "cap", "default_value": float64(25),
	}, &out)

	variants := unmarshalVariants(t, out.Variants)
	if variants["default"] != float64(25) {
		t.Errorf("variants = %v, want default=25", variants)
	}

	res, err := env.Deps.Flags.EvaluateFlag(context.Background(), p.ID, "cap", nil)
	if err != nil {
		t.Fatalf("EvaluateFlag: %v", err)
	}
	if res.Value != float64(25) {
		t.Errorf("evaluated value = %v, want 25", res.Value)
	}
}

func TestUpdateFlagTool_PreservesOtherVariants(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "multi", func(f *repository.FeatureFlag) {
		f.Variants = `{"on":true,"off":false,"canary":true}`
		f.DefaultVariant = "off"
		f.Split = `{"off":100}`
	})
	cs := env.connect(t, tokenReadWrite, p.Slug)

	var out flagDetail
	callOK(t, cs, "update_flag", map[string]any{"flag_key": "multi", "default_value": true}, &out)

	variants := unmarshalVariants(t, out.Variants)
	if variants["off"] != true {
		t.Errorf("off variant = %v, want true (updated)", variants["off"])
	}
	if variants["on"] != true || variants["canary"] != true {
		t.Errorf("other variants clobbered: %v", variants)
	}
}

func TestUpdateFlagTool_PreservesOmittedFields(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "gate", func(f *repository.FeatureFlag) {
		f.Kind = repository.FlagKindConfig
	})
	cs := env.connect(t, tokenReadWrite, p.Slug)

	var out flagDetail
	callOK(t, cs, "update_flag", map[string]any{"flag_key": "gate", "status": "paused"}, &out)
	if out.Status != "paused" {
		t.Errorf("status = %q, want paused", out.Status)
	}
	if out.Kind != repository.FlagKindConfig {
		t.Errorf("flag_kind = %q, a partial update must not reset it to experiment", out.Kind)
	}
}

func TestUpdateFlagTool_ByFlagID(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	f := createManualFlag(t, env, p.ID, "byid", nil)
	cs := env.connect(t, tokenReadWrite, p.Slug)

	var out flagDetail
	callOK(t, cs, "update_flag", map[string]any{"flag_id": f.ID, "name": "Renamed"}, &out)
	if out.Name != "Renamed" {
		t.Errorf("name = %q, want Renamed", out.Name)
	}
}

func TestUpdateFlagTool_MissingKeyAndID(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenReadWrite, p.Slug)

	msg := callErr(t, cs, "update_flag", map[string]any{"status": "paused"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestUpdateFlagTool_InvalidTargetingRules(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	f := createManualFlag(t, env, p.ID, "rules", nil)
	cs := env.connect(t, tokenReadWrite, p.Slug)

	msg := callErr(t, cs, "update_flag", map[string]any{"flag_id": f.ID, "targeting_rules": "not json"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestUpdateFlagTool_CrossProjectNotFoundByKey(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	createManualFlag(t, env, b.ID, "in_b", nil)
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "update_flag", map[string]any{"flag_key": "in_b", "status": "paused"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestUpdateFlagTool_CrossProjectNotFoundByID(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	f := createManualFlag(t, env, b.ID, "in_b", nil)
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "update_flag", map[string]any{"flag_id": f.ID, "status": "paused"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// delete_flag
// ---------------------------------------------------------------------------

func TestDeleteFlagTool_RequiresWriteScope(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	f := createManualFlag(t, env, p.ID, "k", nil)
	cs := env.connect(t, tokenRead, p.Slug)

	msg := callErr(t, cs, "delete_flag", map[string]any{"flag_id": f.ID})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestDeleteFlagTool_Success(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "kill_me", nil)
	cs := env.connect(t, tokenReadWrite, p.Slug)

	var out deleteFlagOut
	callOK(t, cs, "delete_flag", map[string]any{"flag_key": "kill_me"}, &out)
	if !out.Deleted || out.FlagKey != "kill_me" {
		t.Fatalf("got %+v", out)
	}

	msg := callErr(t, cs, "get_flag", map[string]any{"flag_key": "kill_me"})
	if msg == "" {
		t.Fatal("want get_flag to fail after delete")
	}
}

func TestDeleteFlagTool_CrossProjectNotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	f := createManualFlag(t, env, b.ID, "in_b", nil)
	cs := env.connect(t, tokenReadWrite, a.Slug)

	msg := callErr(t, cs, "delete_flag", map[string]any{"flag_id": f.ID})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
	if _, err := env.Deps.Flags.GetFlag(context.Background(), f.ID); err != nil {
		t.Errorf("flag should still exist in project b, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// evaluate_flag
// ---------------------------------------------------------------------------

func TestEvaluateFlagTool_UnknownKeyLeavesNoTrace(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, p.Slug)

	msg := callErr(t, cs, "evaluate_flag", map[string]any{"flag_key": "does_not_exist"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}

	flags, err := env.Deps.Flags.ListFlags(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("ListFlags: %v", err)
	}
	if len(flags) != 0 {
		t.Errorf("evaluate_flag on an unknown key must not create a flag, got %+v", flags)
	}
	if n := flagEvalCount(t, env); n != 0 {
		t.Errorf("evaluate_flag on an unknown key must not record an evaluation, got %d rows", n)
	}
}

func TestEvaluateFlagTool_ResolvesRealFlag(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "checkout", func(f *repository.FeatureFlag) {
		f.Variants = `{"on":true,"off":false}`
		f.DefaultVariant = "off"
		f.Split = `{"on":100,"off":0}`
	})
	cs := env.connect(t, tokenRead, p.Slug)

	var out evaluateFlagOut
	callOK(t, cs, "evaluate_flag", map[string]any{
		"flag_key": "checkout",
		"context":  map[string]any{"targetingKey": "user-1"},
	}, &out)
	if out.Variant != "on" || out.Value != true || out.Reason != "SPLIT" {
		t.Errorf("got %+v, want variant=on value=true reason=SPLIT", out)
	}
	if n := flagEvalCount(t, env); n != 0 {
		t.Errorf("evaluate_flag is a dry run and must not record an evaluation, got %d rows", n)
	}
}

func TestEvaluateFlagTool_TargetingMatchRecordsNothing(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "beta_banner", func(f *repository.FeatureFlag) {
		f.Variants = `{"on":true,"off":false}`
		f.DefaultVariant = "off"
		f.Split = `{"on":0,"off":100}`
		f.TargetingRules = `[{"name":"staff","conditions":[{"context_key":"plan","operator":"eq","value":"staff"}],"variant":"on"}]`
	})
	cs := env.connect(t, tokenRead, p.Slug)

	var out evaluateFlagOut
	callOK(t, cs, "evaluate_flag", map[string]any{
		"flag_key": "beta_banner",
		"context":  map[string]any{"targetingKey": "user-1", "plan": "staff"},
	}, &out)
	if out.Reason != "TARGETING_MATCH" || out.Variant != "on" {
		t.Errorf("got %+v, want reason=TARGETING_MATCH variant=on", out)
	}
	if n := flagEvalCount(t, env); n != 0 {
		t.Errorf("evaluate_flag is a dry run and must not record an evaluation, got %d rows", n)
	}
}

func TestEvaluateFlagTool_MissingFlagKey(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, p.Slug)

	msg := callErr(t, cs, "evaluate_flag", nil)
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestEvaluateFlagTool_CrossProjectNotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Alpha", "alpha")
	b := env.createProject(t, "Beta", "beta")
	createManualFlag(t, env, b.ID, "in_b", nil)
	cs := env.connect(t, tokenRead, a.Slug)

	msg := callErr(t, cs, "evaluate_flag", map[string]any{"flag_key": "in_b"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// analyze_flag
// ---------------------------------------------------------------------------

func TestAnalyzeFlagTool_ConfigKindUnavailable(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "cap", func(f *repository.FeatureFlag) {
		f.Kind = repository.FlagKindConfig
		f.Variants = `{"default":10}`
		f.DefaultVariant = "default"
		f.Split = `{"default":100}`
	})
	cs := env.connect(t, tokenRead, p.Slug)

	var out analyzeFlagOut
	callOK(t, cs, "analyze_flag", map[string]any{"flag_key": "cap"}, &out)
	if out.Unavailable != "config" {
		t.Errorf("unavailable = %q, want config", out.Unavailable)
	}
	if len(out.Results) != 0 {
		t.Errorf("results = %+v, want empty", out.Results)
	}
}

func TestAnalyzeFlagTool_ExperimentReportsSamples(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "exp", func(f *repository.FeatureFlag) {
		f.Variants = `{"on":true,"off":false}`
		f.DefaultVariant = "off"
		f.Split = `{"on":100,"off":0}`
	})
	for i := 0; i < 5; i++ {
		if _, err := env.Deps.Flags.EvaluateFlag(context.Background(), p.ID, "exp", map[string]any{
			"targetingKey": fmt.Sprintf("user-%d", i),
		}); err != nil {
			t.Fatalf("EvaluateFlag: %v", err)
		}
	}

	cs := env.connect(t, tokenRead, p.Slug)
	var out analyzeFlagOut
	callOK(t, cs, "analyze_flag", map[string]any{"flag_key": "exp"}, &out)
	if out.Unavailable != "" {
		t.Fatalf("unavailable = %q, want empty", out.Unavailable)
	}
	if len(out.Results) != 1 || out.Results[0].Variant != "on" || out.Results[0].Sample != 5 {
		t.Fatalf("results = %+v, want one entry on/sample=5", out.Results)
	}
}

func TestAnalyzeFlagTool_MissingFlagKey(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, p.Slug)

	msg := callErr(t, cs, "analyze_flag", nil)
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestAnalyzeFlagTool_NotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	cs := env.connect(t, tokenRead, p.Slug)

	msg := callErr(t, cs, "analyze_flag", map[string]any{"flag_key": "nope"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

func TestAnalyzeFlagTool_InvalidRange(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "exp", nil)
	cs := env.connect(t, tokenRead, p.Slug)

	msg := callErr(t, cs, "analyze_flag", map[string]any{"flag_key": "exp", "from": "not-a-date"})
	if msg == "" {
		t.Fatal("want a non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// list_flag_context_keys
// ---------------------------------------------------------------------------

func TestListFlagContextKeysTool(t *testing.T) {
	env := newTestEnv(t, nil)
	p := env.createProject(t, "Alpha", "alpha")
	createManualFlag(t, env, p.ID, "ctxflag", nil)
	for i := 0; i < 3; i++ {
		if _, err := env.Deps.Flags.EvaluateFlag(context.Background(), p.ID, "ctxflag", map[string]any{
			"targetingKey": fmt.Sprintf("user-%d", i),
			"plan":         "pro",
		}); err != nil {
			t.Fatalf("EvaluateFlag: %v", err)
		}
	}

	cs := env.connect(t, tokenRead, p.Slug)
	var out listFlagContextKeysOut
	callOK(t, cs, "list_flag_context_keys", nil, &out)

	found := false
	for _, k := range out.ContextKeys {
		if k.ContextKey == "plan" {
			found = true
		}
	}
	if !found {
		t.Errorf(`want "plan" among context keys, got %+v`, out.ContextKeys)
	}
}

// ---------------------------------------------------------------------------
// annotations
// ---------------------------------------------------------------------------

func TestFlagTools_Annotations(t *testing.T) {
	env := newTestEnv(t, nil)
	cs := env.connect(t, tokenRead, "")

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	readOnly := map[string]bool{
		"list_flags": false, "get_flag": false, "analyze_flag": false,
		"list_flag_context_keys": false, "evaluate_flag": false,
	}
	nonDestructiveWrites := map[string]bool{"create_flag": false, "update_flag": false}
	destructive := map[string]bool{"delete_flag": false}

	for _, tool := range res.Tools {
		switch {
		case has(readOnly, tool.Name):
			readOnly[tool.Name] = true
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Errorf("%s: ReadOnlyHint not set", tool.Name)
			}
		case has(nonDestructiveWrites, tool.Name):
			nonDestructiveWrites[tool.Name] = true
			if tool.Annotations != nil && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
				t.Errorf("%s: DestructiveHint should not be true", tool.Name)
			}
		case has(destructive, tool.Name):
			destructive[tool.Name] = true
			if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
				t.Errorf("%s: DestructiveHint not set to true", tool.Name)
			}
		}
	}
	for _, seen := range []map[string]bool{readOnly, nonDestructiveWrites, destructive} {
		for name, ok := range seen {
			if !ok {
				t.Errorf("%s not in tools/list", name)
			}
		}
	}
}

func has(m map[string]bool, k string) bool {
	_, ok := m[k]
	return ok
}
