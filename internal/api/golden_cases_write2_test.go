package api

// Mutation cases for the project-scoped entities: funnels, flags, segments,
// widgets, A/B tests and the canonical vocabulary. See
// golden_cases_write_test.go for the conventions.

import "net/http"

func goldenFunnelMutations(b *mutBuilder, s *goldenSeed) {
	pa := "/api/v1/projects/" + s.ProjectA
	step := func(name string) map[string]any { return map[string]any{"event_name": name} }
	b.write("funnel_create", "POST", pa+"/funnels", map[string]any{"name": "Golden funnel", "description": "created by the golden suite", "scope": "session",
		"steps": []any{step("page_view"), map[string]any{"event_name": "purchase", "filters": []any{map[string]any{"property": "plan", "value": "pro"}}}}},
		http.StatusCreated, map[string]string{"newFunnel": "id"})
	b.write("funnel_create_no_steps", "POST", pa+"/funnels", map[string]any{"name": "Empty"}, http.StatusUnprocessableEntity, nil)
	b.read("funnels_a_after_create", pa+"/funnels")
	b.write("funnel_update", "PUT", pa+"/funnels/$newFunnel", map[string]any{"name": "Golden funnel v2", "scope": "session",
		"steps": []any{step("page_view"), step("add_to_cart"), step("purchase")}}, http.StatusOK, nil)
	b.read("funnels_a_after_update", pa+"/funnels")
	b.read("funnel_analysis_new_after_update", s.window(pa+"/funnels/$newFunnel/analysis"))
	b.write("funnel_delete", "DELETE", pa+"/funnels/$newFunnel", nil, http.StatusNoContent, nil)
	b.read("funnels_a_after_delete", pa+"/funnels")
}

func goldenFlagMutations(b *mutBuilder, s *goldenSeed) {
	pa := "/api/v1/projects/" + s.ProjectA
	b.write("flag_create", "POST", pa+"/flags", map[string]any{"flag_key": "golden_flag", "name": "Golden flag", "flag_type": "boolean",
		"variants": `{"on":true,"off":false}`, "default_variant": "off", "split": `{"on":50,"off":50}`},
		http.StatusCreated, map[string]string{"newFlag": "id"})
	b.write("flag_create_missing_key", "POST", pa+"/flags", map[string]any{"name": "No key"}, http.StatusUnprocessableEntity, nil)
	b.read("flag_after_create", pa+"/flags/$newFlag")
	b.write("flag_update", "PUT", pa+"/flags/$newFlag", map[string]any{"name": "Golden flag renamed", "status": "paused"}, http.StatusOK, nil)
	b.read("flag_after_update", pa+"/flags/$newFlag")
	b.write("flag_delete", "DELETE", pa+"/flags/$newFlag", nil, http.StatusNoContent, nil)
	b.readStatus("flag_after_delete", pa+"/flags/$newFlag", http.StatusNotFound)
	b.read("flags_a_after_mutations", pa+"/flags")
}

func goldenSegmentWidgetMutations(b *mutBuilder, s *goldenSeed) {
	pa := "/api/v1/projects/" + s.ProjectA
	rule := func(field, value string) []any {
		return []any{map[string]any{"field": field, "operator": "eq", "value": value}}
	}
	b.write("segment_create", "POST", pa+"/segments", map[string]any{"name": "Golden segment", "rules": rule("country_code", "FR")},
		http.StatusCreated, map[string]string{"newSegment": "id"})
	b.write("segment_create_no_name", "POST", pa+"/segments", map[string]any{"rules": rule("country_code", "FR")}, http.StatusUnprocessableEntity, nil)
	b.read("segments_a_after_create", pa+"/segments")
	b.write("segment_update", "PUT", pa+"/segments/$newSegment", map[string]any{"name": "Golden segment v2", "rules": rule("device_type", "tablet")}, http.StatusOK, nil)
	b.read("segments_a_after_update", pa+"/segments")
	b.write("segment_delete", "DELETE", pa+"/segments/$newSegment", nil, http.StatusNoContent, nil)
	b.read("segments_a_after_delete", pa+"/segments")

	b.write("widget_create", "POST", pa+"/widgets", map[string]any{"event_name": "add_to_cart", "property": "plan", "title": "Golden widget", "position": 5, "size": 1},
		http.StatusCreated, map[string]string{"newWidget": "id"})
	b.write("widget_create_no_event", "POST", pa+"/widgets", map[string]any{"title": "No event"}, http.StatusUnprocessableEntity, nil)
	b.read("widgets_a_after_create", pa+"/widgets")
	b.write("widget_update", "PUT", pa+"/widgets/$newWidget", map[string]any{"event_name": "add_to_cart", "property": "browser", "title": "Golden widget v2", "position": 6, "size": 2}, http.StatusOK, nil)
	b.read("widgets_a_after_update", pa+"/widgets")
	b.write("widget_delete", "DELETE", pa+"/widgets/$newWidget", nil, http.StatusNoContent, nil)
	b.read("widgets_a_after_delete", pa+"/widgets")
}

// There is no update or delete route for A/B tests; create plus read-back is
// the whole surface.
func goldenABTestMutations(b *mutBuilder, s *goldenSeed) {
	pa := "/api/v1/projects/" + s.ProjectA
	b.write("abtest_create", "POST", pa+"/abtests", map[string]any{"name": "Golden A/B", "conversion_event": "purchase",
		"control_filter": `{"property":"variant","value":"x"}`, "variant_filter": `{"property":"variant","value":"y"}`},
		http.StatusCreated, map[string]string{"newABTest": "id"})
	b.read("abtests_a_after_create", pa+"/abtests")
	b.read("abtest_analysis_new_after_create", s.window(pa+"/abtests/$newABTest/analysis"))
}

func goldenCanonicalMutations(b *mutBuilder, s *goldenSeed) {
	pa := "/api/v1/projects/" + s.ProjectA
	b.write("canonical_event_create", "POST", "/api/v1/canonical-events", map[string]any{"key": "golden_evt", "label": "Golden event", "sort_order": 99}, http.StatusCreated, nil)
	b.read("canonical_events_after_create", "/api/v1/canonical-events")
	b.write("canonical_event_update", "PUT", "/api/v1/canonical-events/golden_evt", map[string]any{"label": "Golden event v2", "sort_order": 98}, http.StatusOK, nil)
	b.read("canonical_events_after_update", "/api/v1/canonical-events")
	b.write("canonical_event_delete", "DELETE", "/api/v1/canonical-events/golden_evt", nil, http.StatusNoContent, nil)
	b.read("canonical_events_after_delete", "/api/v1/canonical-events")

	b.write("event_mapping_set", "PUT", pa+"/event-mappings", map[string]any{"mappings": []any{map[string]any{"raw_name": "golden_raw", "canonical_key": "page_view"}}}, http.StatusOK, nil)
	b.read("event_mappings_a_after_set", pa+"/event-mappings")
	b.write("event_mapping_delete", "DELETE", pa+"/event-mappings/golden_raw", nil, http.StatusNoContent, nil)
	b.read("event_mappings_a_after_delete", pa+"/event-mappings")

	steps := []any{map[string]any{"canonical_key": "page_view", "label": "Visit"}, map[string]any{"canonical_key": "sign_up", "label": "Sign up"}}
	b.write("canonical_funnel_create", "POST", "/api/v1/overview/funnels", map[string]any{"name": "Golden canonical funnel", "description": "created by the golden suite",
		"project_ids": []any{s.ProjectA, s.ProjectB}, "steps": steps}, http.StatusCreated, map[string]string{"newCanonicalFunnel": "id"})
	b.read("canonical_funnels_after_create", "/api/v1/overview/funnels")
	b.write("canonical_funnel_update", "PUT", "/api/v1/overview/funnels/$newCanonicalFunnel", map[string]any{"name": "Golden canonical funnel v2",
		"project_ids": []any{s.ProjectA}, "steps": steps}, http.StatusOK, nil)
	b.read("canonical_funnels_after_update", "/api/v1/overview/funnels")
	b.write("canonical_funnel_delete", "DELETE", "/api/v1/overview/funnels/$newCanonicalFunnel", nil, http.StatusNoContent, nil)
	b.read("canonical_funnels_after_delete", "/api/v1/overview/funnels")
}
