package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

// Make sure the display name we ship in the setup doc matches the wire
// constant the auth layer reads. HTTP headers are case-insensitive, but if
// someone renamed one (e.g. "X-FunnelBarn-ApiKey" without the hyphen), the
// doc and the server would silently drift apart.
func TestSetupDoc_DisplayHeaderMatchesWireConstant(t *testing.T) {
	const displayed = "X-FunnelBarn-Api-Key"
	if !strings.EqualFold(displayed, auth.HeaderAPIKey) {
		t.Errorf("setup doc displays %q but server reads %q — they no longer match", displayed, auth.HeaderAPIKey)
	}
}

// The /api/v1/setup/{slug} page is what LLMs and humans copy from when wiring
// up a tracker. If it ever ships the wrong header name (we've shipped X-API-Key
// in the past — the actual header is x-funnelbarn-api-key), every new project
// would get 401s on first event. This test locks in the correct headers and
// the absence of the legacy wrong one.
func TestSetupDoc_UsesCorrectHeaders(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/loadtest-project", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("setup doc: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	body := w.Body.String()

	// Must mention the correct headers, class names, and option keys.
	for _, want := range []string{
		"X-FunnelBarn-Api-Key", // auth header
		"X-FunnelBarn-Project", // project routing header
		"loadtest-project",     // the slug we requested
		"FunnelBarnClient",     // correct TS class name (not the alias FunnelBarn)
		"projectName:",         // correct FunnelBarnOptions field (not project:)
		"data-project-name",    // IIFE script tag attribute for project routing
		"NOT `event`",          // body schema must call out the wrong field name
		"`properties`",         // the main extension point
		"`utm_source`",         // explicit UTM override fields
		"UTM params",           // auto-extraction call-out
		"Hashed server-side",   // privacy property of user_id
		"`user_agent`",         // server-side ingest UA override
	} {
		if !strings.Contains(body, want) {
			t.Errorf("setup doc missing %q", want)
		}
	}

	// Must NOT mention legacy/wrong values that would cause 401s or silent failures.
	for _, unwanted := range []string{
		"X-API-Key:",                 // the old, wrong header name
		"X-Api-Key:",                 // case variant that's still wrong (different header altogether)
		"/api/v1/releases",           // funnelbarn doesn't have this endpoint; only bugbarn does
		"import { FunnelBarn } from", // wrong class import — exports FunnelBarnClient
		"npm install @funnelbarn/js", // package isn't published — CDN script or vendoring is the only working path
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("setup doc still references %q — would mislead consumers", unwanted)
		}
	}

	// The body schema should mention `timestamp`, not the never-parsed
	// `occurred_at` field we used to ship in examples.
	if strings.Contains(body, `"occurred_at"`) {
		t.Error("setup doc uses occurred_at — server ignores it; should be 'timestamp'")
	}
	if !strings.Contains(body, `"timestamp"`) {
		t.Error("setup doc missing timestamp field reference")
	}
}

// Lock in the Feature Flags section. FunnelBarn doubles as a flag service,
// and any LLM/customer wiring up the setup endpoint should see how to
// evaluate flags from their app.
func TestSetupDoc_DocumentsFlagEvaluation(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/loadtest-project", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	body := w.Body.String()

	for _, want := range []string{
		"## Feature Flags",           // section header
		"/api/v1/evaluate",           // endpoint path
		"`flag_key`",                 // request field
		"`default_value`",            // request field — falls back on missing flag
		"`context`",                  // request field — for targeting + bucketing
		"`targeting_key`",            // the actual bucket key (not user_id!)
		"FLAG_NOT_FOUND",             // documented error code
		"TARGETING_MATCH",            // documented reason
		"flag_metadata",              // response field for targeting-match debugging
		"evaluated_rule_name",        // what flag_metadata exposes
		"Bucketing is deterministic", // determinism guarantee
	} {
		if !strings.Contains(body, want) {
			t.Errorf("setup doc Feature Flags section missing %q", want)
		}
	}

	// Common SDK mistake: assume `user_id` is the bucket key. The doc must
	// explicitly call out that it isn't.
	if !strings.Contains(body, "`user_id` is **not**") && !strings.Contains(body, "user_id is not") {
		t.Error("setup doc should warn that user_id is not the bucket key (targeting_key is)")
	}
}
