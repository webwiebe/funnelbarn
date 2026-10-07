package api

// Golden case definitions: every GET route, plus (in golden_cases_post_test.go)
// the SDK and playground evaluate endpoints. Cases are built from the seed so
// path parameters are the real fixed ids, and every time-relative route gets
// the explicit from/to window instead of "last 30 days from now".

import (
	"net/http"
	"reflect"
)

type goldenAuth int

const (
	authSession goldenAuth = iota // dashboard session cookie (+ CSRF on writes)
	authKey                       // x-funnelbarn-api-key header
	authNone                      // public route
	authSpare                     // the spare session cookie (+ CSRF on writes)
)

// goldenPhase orders cases. Read-only GETs run first so that cases which
// write (a GET that provisions, evaluations, dashboard mutations) cannot
// change what an earlier case sees. The order is: reads, then the evaluate
// writes, then the read-after-evaluate phase, then the dashboard mutations
// each followed by its read-back.
type goldenPhase int

const (
	phaseRead goldenPhase = iota
	phaseGETWrites
	phasePOST          // flag evaluation (async bookkeeping)
	phaseAfterEvaluate // reads of what evaluation wrote
	phaseMutate        // dashboard mutations, each followed by a read-back
)

// goldenCase is one request and the golden file that records its response.
type goldenCase struct {
	Name   string // golden file stem; unique
	Method string
	Path   string // path and query; may contain $var placeholders (see Save)
	Auth   goldenAuth
	Key    string // plaintext API key when Auth == authKey; may be a $var
	Body   any    // JSON request body for POST/PUT; $var placeholders allowed
	Phase  goldenPhase
	// WantStatus is the status the case must return, checked on every run
	// including -update, so a recording can never capture an unexpected error.
	// Zero means 200.
	WantStatus int
	// Save captures values of the JSON response into variables for later
	// cases: var name -> dotted path ("id", "api_key.id"). Cases refer to a
	// variable as $name in Path, Key and Body.
	Save map[string]string
	// Await names async bookkeeping that must have landed before the case
	// runs (see waitGoldenAsync).
	Await string
	// AwaitFlags lists the flag rows an awaitAutoFlag wait requires.
	AwaitFlags []goldenFlagRef
	// wantKeys and wantSetupA are derived by goldenAnnotate: the API keys the
	// earlier cases used, and whether the setup guide was requested.
	wantKeys   int
	wantSetupA bool
}

func (c goldenCase) wantStatus() int {
	if c.WantStatus == 0 {
		return http.StatusOK
	}
	return c.WantStatus
}

// goldenSkips lists routes ("METHOD /pattern", method "*" for a registration
// without one) that cannot have a deterministic golden case. Keep this list
// minimal; every entry needs a reason a reviewer can check.
var goldenSkips = map[string]string{
	"GET /metrics":                                       "Prometheus exposition of the process-global registry: values depend on every request any test in the process made, and on runtime stats.",
	"GET /api/v1/oidc/login":                             "Redirects to an external OIDC provider and needs one configured; the redirect target and state are random per call.",
	"GET /api/v1/oidc/callback":                          "Completes an OIDC code exchange against an external provider; cannot run without one.",
	"GET /api/v1/auth/oidc/logged-out":                   "Revokes the session cookie every other session-authed case runs under, then redirects; its 302 is fixed code, covered by oidc_test.go.",
	"* /api/v1/mcp":                                      "Only mounted when OIDC is configured (s.mcpEnabled) and then needs an IAMBarn-signed bearer token; the harness server has neither. Covered end to end by TestMCP_* in mcp_test.go (fake IdP, initialize, tool list, rate limit, 401s) and by the tool tests in internal/mcp.",
	"* /.well-known/oauth-protected-resource/api/v1/mcp": "Mounted together with the MCP endpoint, so absent without OIDC. Covered by TestMCP_ProtectedResourceMetadata and TestMCP_NotServedWithoutOIDC in mcp_test.go.",
}

// goldenGETCases returns one or more cases per GET route.
func goldenGETCases(s *goldenSeed) []goldenCase {
	A, B := s.ProjectA, s.ProjectB
	p := func(id, rest string) string { return "/api/v1/projects/" + id + rest }
	w := s.window
	var cs []goldenCase
	add := func(name, path string, auth goldenAuth, key string) {
		cs = append(cs, goldenCase{Name: name, Method: "GET", Path: path, Auth: auth, Key: key})
	}
	sess := func(name, path string) { add(name, path, authSession, "") }

	// Public.
	add("health", "/api/v1/health", authNone, "")
	add("client_config", "/api/v1/client-config", authNone, "")
	add("theme_manifest", themeManifestPath, authNone, "")
	cs = append(cs, goldenCase{Name: "setup_guide", Method: "GET", Path: "/api/v1/setup/" + s.SlugA, Auth: authNone, Phase: phaseGETWrites})

	// API-key authed.
	add("trace_lookup", "/api/v1/traces/"+s.TraceA, authKey, s.KeyAFull)
	cs = append(cs, goldenCase{Name: "trace_lookup_not_found", Method: "GET", Path: "/api/v1/traces/00000000000000000000000000000000", Auth: authKey, Key: s.KeyAFull, WantStatus: http.StatusNotFound})
	add("recording_chunk_by_key", "/api/v1/recordings/"+s.RecordingA1+"/chunks/0", authKey, s.KeyAFull)

	// Session and account.
	sess("me", "/api/v1/me")
	sess("projects_list", "/api/v1/projects")
	add("projects_list_token", "/api/v1/projects", authKey, s.KeyAAnalytics)
	// last_used_at of the keys used by the cases above is written off the
	// request path, so these reads wait for it.
	cs = append(cs,
		goldenCase{Name: "apikeys_list_all", Method: "GET", Path: "/api/v1/apikeys", Auth: authSession, Await: awaitAPIKeys},
		goldenCase{Name: "apikeys_list_project", Method: "GET", Path: "/api/v1/apikeys?project_id=" + A, Auth: authSession, Await: awaitAPIKeys})
	sess("instance_settings", "/api/v1/instance-settings")

	// Dashboard and flows.
	sess("dashboard_a", w(p(A, "/dashboard")))
	sess("dashboard_a_production", w(p(A, "/dashboard?environment=production")))
	add("dashboard_a_token", w(p(A, "/dashboard")), authKey, s.KeyAAnalytics)
	sess("dashboard_b", w(p(B, "/dashboard")))
	sess("flows_a", w(p(A, "/flows")))
	sess("flows_a_page", w(p(A, "/flows?page=https://acme.example/pricing&depth=3")))
	sess("flows_sessions_a", w(p(A, "/flows/sessions?page=https://acme.example/pricing")))

	// Events.
	sess("events_a", p(A, "/events?limit=25"))
	sess("events_a_page2", p(A, "/events?limit=25&offset=25"))
	add("events_a_token", p(A, "/events?limit=10"), authKey, s.KeyAAnalytics)
	sess("event_names_a", p(A, "/event-names"))
	sess("event_counts_a", w(p(A, "/event-counts")))
	add("event_counts_a_token", w(p(A, "/event-counts?limit=5")), authKey, s.KeyAAnalytics)
	sess("event_properties_a", p(A, "/event-properties?event_name=purchase"))
	sess("event_property_values_a", p(A, "/event-property-values?event_name=purchase&property=plan"))
	sess("environments_a", p(A, "/environments"))

	// Overview and canonical vocabulary.
	sess("overview", w("/api/v1/overview"))
	sess("overview_dimension", w("/api/v1/overview?dimension=device_type"))
	sess("overview_events", "/api/v1/overview/events?limit=30")
	sess("overview_events_filtered", "/api/v1/overview/events?limit=10&project_id="+B+"&name=signup")
	sess("canonical_events", "/api/v1/canonical-events")
	sess("event_mappings_a", p(A, "/event-mappings"))
	sess("event_mapping_suggestions_a", p(A, "/event-mappings/suggestions"))
	sess("canonical_funnels", "/api/v1/overview/funnels")
	sess("canonical_funnel_analysis", w("/api/v1/overview/funnels/"+s.CanonicalFunnel+"/analysis"))
	sess("canonical_funnel_analysis_project_a", w("/api/v1/overview/funnels/"+s.CanonicalFunnel+"/analysis?project_ids="+A))

	// Funnels.
	sess("funnels_a", p(A, "/funnels"))
	add("funnels_a_token", p(A, "/funnels"), authKey, s.KeyAAnalytics)
	sess("funnels_b", p(B, "/funnels"))
	sess("funnel_analysis_a", w(p(A, "/funnels/"+s.FunnelA+"/analysis")))
	sess("funnel_analysis_a_segment", w(p(A, "/funnels/"+s.FunnelA+"/analysis?segment_id="+s.SegmentMobile)))
	sess("funnel_analysis_a_page_scope", w(p(A, "/funnels/"+s.FunnelAPage+"/analysis")))
	add("funnel_analysis_a_token", w(p(A, "/funnels/"+s.FunnelA+"/analysis")), authKey, s.KeyAAnalytics)
	sess("funnel_analysis_b", w(p(B, "/funnels/"+s.FunnelB+"/analysis")))
	sess("funnel_segments_a", w(p(A, "/funnels/"+s.FunnelA+"/segments")))
	sess("funnel_step_sessions_a", w(p(A, "/funnels/"+s.FunnelA+"/steps/2/sessions")))

	// Flags.
	sess("flags_a", p(A, "/flags"))
	add("flags_a_token", p(A, "/flags"), authKey, s.KeyAFlags)
	sess("flags_b", p(B, "/flags"))
	sess("flag_experiment", p(A, "/flags/"+s.FlagExperiment))
	sess("flag_targeted", p(A, "/flags/"+s.FlagTargeted))
	sess("flag_config", p(A, "/flags/"+s.FlagConfig))
	sess("flag_inactive", p(A, "/flags/"+s.FlagInactive))
	sess("flag_auto", p(A, "/flags/"+s.FlagAuto))
	sess("flag_analysis_experiment", p(A, "/flags/"+s.FlagExperiment+"/analysis?range=30d"))
	sess("flag_analysis_targeted", p(A, "/flags/"+s.FlagTargeted+"/analysis?range=30d"))
	sess("flag_analysis_config", p(A, "/flags/"+s.FlagConfig+"/analysis"))
	sess("flag_context_keys_a", p(A, "/flags/context-keys"))

	// A/B tests, widgets, segments.
	sess("abtests_a", p(A, "/abtests"))
	sess("abtest_analysis_a", w(p(A, "/abtests/"+s.ABTestA+"/analysis")))
	sess("widgets_a", p(A, "/widgets"))
	sess("widget_breakdown_a", p(A, "/widgets/"+s.WidgetA1+"/breakdown?window=500&limit=5"))
	sess("widget_breakdowns_a", p(A, "/widgets/breakdowns"))
	sess("widget_breakdowns_b", p(B, "/widgets/breakdowns"))
	sess("segments_a", p(A, "/segments"))

	// Sessions and recordings.
	sess("sessions_a", p(A, "/sessions?limit=20"))
	sess("sessions_a_page2", p(A, "/sessions?limit=20&offset=20"))
	sess("sessions_active_a", p(A, "/sessions/active"))
	sess("session_distributions_a", p(A, "/session-distributions"))
	sess("recordings_a", p(A, "/recordings"))
	sess("recordings_a_mobile", p(A, "/recordings?device_type=mobile&human_only=true"))
	sess("recordings_b", p(B, "/recordings"))
	sess("recording_chunk_a", p(A, "/recordings/"+s.RecordingA1+"/chunks/1"))
	sess("recording_flags_a", p(A, "/recordings/"+s.RecordingA1+"/flags"))
	sess("recording_traces_a", p(A, "/recordings/"+s.RecordingA1+"/traces"))
	sess("recording_settings_a", p(A, "/recording-settings"))
	add("recording_config", "/api/v1/recording-config", authKey, s.KeyAFull)

	// Project health.
	sess("project_health_a", p(A, "/health"))
	return cs
}

// goldenPlaceholderSeed fills every string field with a non-empty value so the
// coverage test can build the case list (and match paths to routes) without
// seeding a database.
func goldenPlaceholderSeed() *goldenSeed {
	s := &goldenSeed{}
	v := reflect.ValueOf(s).Elem()
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.String {
			f.SetString("x" + v.Type().Field(i).Name)
		}
	}
	return s
}
