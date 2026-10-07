package api

// Golden cases for the flag evaluation endpoints. These are the hot SDK path
// and the first thing the CQRS split moves (evaluate bookkeeping becomes an
// async command), so they get the most precise coverage: every reason code the
// handler can return.

import "net/http"

// goldenPOSTCases returns the evaluate cases. They write (evaluation rows,
// auto-registered flags), so they run after every read-only case.
func goldenPOSTCases(s *goldenSeed) []goldenCase {
	var cs []goldenCase
	sdk := func(name string, key string, body map[string]any) {
		cs = append(cs, goldenCase{Name: name, Method: "POST", Path: "/api/v1/evaluate", Auth: authKey, Key: key, Body: body, Phase: phasePOST})
	}
	want := func(status int) { cs[len(cs)-1].WantStatus = status }
	play := func(name string, body map[string]any) {
		cs = append(cs, goldenCase{Name: name, Method: "POST", Path: "/api/v1/projects/" + s.ProjectA + "/flags/evaluate", Auth: authSession, Body: body, Phase: phasePOST})
	}
	ctx := func(i int, country string) map[string]any {
		return map[string]any{"targeting_key": "u-eval-" + string(rune('a'+i)), "session_id": "sess-eval-" + string(rune('a'+i)), "country": country}
	}

	// SDK evaluate, API-key authed.
	sdk("evaluate_experiment_split_nl_treatment", s.KeyAIngest, map[string]any{"flag_key": "checkout_redesign", "default_value": false, "context": ctx(0, "NL")})
	sdk("evaluate_experiment_split_de_control", s.KeyAIngest, map[string]any{"flag_key": "checkout_redesign", "default_value": false, "context": ctx(1, "DE")})
	sdk("evaluate_targeting_match", s.KeyAIngest, map[string]any{"flag_key": "pricing_banner", "default_value": "standard", "context": ctx(2, "NL")})
	sdk("evaluate_targeting_no_match", s.KeyAIngest, map[string]any{"flag_key": "pricing_banner", "default_value": "standard", "context": ctx(3, "DE")})
	sdk("evaluate_config_flag", s.KeyAIngest, map[string]any{"flag_key": "max_items", "default_value": 10, "kind": "config", "context": map[string]any{}})
	sdk("evaluate_inactive_flag", s.KeyAIngest, map[string]any{"flag_key": "legacy_search", "default_value": true, "context": ctx(4, "FR")})
	sdk("evaluate_auto_registered_inactive", s.KeyAIngest, map[string]any{"flag_key": "sdk_discovered", "default_value": false, "context": ctx(5, "US")})
	sdk("evaluate_unknown_key_auto_registers", s.KeyAIngest, map[string]any{"flag_key": "brand_new_flag", "default_value": true, "context": ctx(6, "GB")})
	sdk("evaluate_unknown_key_auto_registered_again", s.KeyAIngest, map[string]any{"flag_key": "brand_new_flag", "default_value": true, "context": ctx(6, "GB")})
	sdk("evaluate_unknown_config_key_auto_registers", s.KeyAIngest, map[string]any{"flag_key": "new_limit", "default_value": 50, "kind": "config", "context": map[string]any{}})
	sdk("evaluate_unknown_key_invalid", s.KeyAIngest, map[string]any{"flag_key": "not a valid key!", "default_value": "fallback", "context": map[string]any{}})
	sdk("evaluate_other_project_flag", s.KeyBFull, map[string]any{"flag_key": "checkout_redesign", "default_value": false, "context": ctx(7, "NL")})
	sdk("evaluate_project_b_flag", s.KeyBFull, map[string]any{"flag_key": "beta_banner", "default_value": false, "context": ctx(8, "NL")})
	sdk("evaluate_missing_flag_key", s.KeyAIngest, map[string]any{"default_value": false})
	want(http.StatusUnprocessableEntity)
	sdk("evaluate_bad_key", "gk-wrong", map[string]any{"flag_key": "checkout_redesign"})
	want(http.StatusUnauthorized)

	// Dashboard playground, session authed, never auto-registers.
	play("playground_experiment_treatment", map[string]any{"flag_key": "checkout_redesign", "context": ctx(0, "NL")})
	play("playground_targeting_match", map[string]any{"flag_key": "pricing_banner", "context": ctx(2, "NL")})
	play("playground_config_flag", map[string]any{"flag_key": "max_items", "context": map[string]any{}})
	play("playground_inactive_flag", map[string]any{"flag_key": "legacy_search", "default_value": true, "context": ctx(4, "FR")})
	play("playground_unknown_key_no_register", map[string]any{"flag_key": "never_registered", "default_value": "fallback", "context": map[string]any{}})
	// Each auto-registering case is followed by a wait for its flag row. This
	// is intentional: the registration cap counts auto flags, and the
	// registration may land off the request goroutine, so the next case's cap
	// check only sees deterministic state once the row exists. The wait attaches
	// to the next case because an Await runs before its case.
	awaitFlagAfter(cs, "evaluate_auto_registered_inactive", s.ProjectA, "sdk_discovered")
	awaitFlagAfter(cs, "evaluate_unknown_key_auto_registers", s.ProjectA, "brand_new_flag")
	awaitFlagAfter(cs, "evaluate_unknown_key_auto_registered_again", s.ProjectA, "brand_new_flag")
	awaitFlagAfter(cs, "evaluate_unknown_config_key_auto_registers", s.ProjectA, "new_limit")
	awaitFlagAfter(cs, "evaluate_other_project_flag", s.ProjectB, "checkout_redesign")
	return append(cs, goldenAfterEvaluateCases(s)...)
}

// awaitFlagAfter makes the case following name wait until the flag row exists.
func awaitFlagAfter(cs []goldenCase, name, project, key string) {
	for i := range cs {
		if cs[i].Name != name {
			continue
		}
		if i+1 >= len(cs) {
			panic("awaitFlagAfter: no case after " + name)
		}
		next := &cs[i+1]
		if next.Await != "" && next.Await != awaitAutoFlag {
			panic("awaitFlagAfter: " + next.Name + " already awaits " + next.Await)
		}
		next.Await = awaitAutoFlag
		next.AwaitFlags = append(next.AwaitFlags, goldenFlagRef{Project: project, Key: key})
		return
	}
	panic("awaitFlagAfter: no case named " + name)
}

// goldenAfterEvaluateCases is the read-after-write phase for evaluation: what
// the evaluations above left behind. The auto-registered flags appear in the
// lists, the experiment's analysis counts the new evaluations, and the health
// flag flips. The first case waits for the async bookkeeping (touch and
// MarkFlagsEvaluated run off the request goroutine).
func goldenAfterEvaluateCases(s *goldenSeed) []goldenCase {
	p := "/api/v1/projects/"
	get := func(name, path string) goldenCase {
		return goldenCase{Name: name, Method: "GET", Path: path, Auth: authSession, Phase: phaseAfterEvaluate}
	}
	first := get("flags_a_after_evaluate", p+s.ProjectA+"/flags")
	first.Await = awaitEvaluate
	// The setup guide request (phaseGETWrites) flips setup_called off the
	// request goroutine; this is the first health read after it.
	healthA := get("project_health_a_after_evaluate", p+s.ProjectA+"/health")
	healthA.Await = awaitSetup
	return []goldenCase{
		first,
		get("flags_b_after_evaluate", p+s.ProjectB+"/flags"),
		get("flag_analysis_experiment_after_evaluate", p+s.ProjectA+"/flags/"+s.FlagExperiment+"/analysis?range=30d"),
		healthA,
		get("project_health_b_after_evaluate", p+s.ProjectB+"/health"),
	}
}
