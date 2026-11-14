package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// sessionAndCSRF mints a session (cookie + server-side row) + matching CSRF
// for a server with auth enabled. Both must come from the same token (server
// derives CSRF from cookie.Value with the session secret).
func sessionAndCSRF(t *testing.T, srv *Server, username string) (*http.Cookie, string) {
	t.Helper()
	cookie := sessionCookieFor(t, srv, username)
	return cookie, srv.sessionManager.CSRFToken(cookie.Value)
}

func TestHandleListFlags_Empty(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "Empty", "empty")
	cookie, _ := sessionAndCSRF(t, srv, "u")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("list flags: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	flags, ok := resp["flags"].([]any)
	if !ok {
		t.Fatalf("response missing flags array: %v", resp)
	}
	if len(flags) != 0 {
		t.Errorf("expected empty flags list, got %d", len(flags))
	}
}

func TestHandleListFlags_ReturnsCreated(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "WithFlags", "withflags")
	_, _ = store.CreateFlag(ctx, domain.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "feature_x",
		Name:           "Feature X",
		FlagType:       "release",
		Variants:       `{"on":true,"off":false}`,
		DefaultVariant: "off",
		Split:          `{"on":50,"off":50}`,
		Status:         "active",
	})
	cookie, _ := sessionAndCSRF(t, srv, "u")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("list flags: want 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	flags := resp["flags"].([]any)
	if len(flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(flags))
	}
}

func TestHandleCreateFlag(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "CreateFlag", "createflag")
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	// The flag handler takes variants/split as already-JSON-encoded strings.
	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags", map[string]any{
		"flag_key":        "new_flag",
		"name":            "New Flag",
		"flag_type":       "release",
		"variants":        `{"on":true,"off":false}`,
		"default_variant": "off",
		"split":           `{"on":0,"off":100}`,
	}, cookie, csrf)

	if w.Code != http.StatusCreated {
		t.Fatalf("create flag: want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["flag_key"] != "new_flag" {
		t.Errorf("flag_key: got %v", resp["flag_key"])
	}
}

func TestHandleCreateFlag_MissingKey_Returns422(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "BadFlag", "badflag")
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags", map[string]any{
		"name": "no key",
	}, cookie, csrf)

	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Errorf("expected 4xx for missing flag_key, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleGetFlag(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "GetFlag", "getflag")
	flag, _ := store.CreateFlag(ctx, domain.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "get_me",
		Name:           "Get Me",
		FlagType:       "release",
		Variants:       `{"on":true}`,
		DefaultVariant: "on",
		Split:          `{"on":100}`,
		Status:         "active",
	})
	cookie, _ := sessionAndCSRF(t, srv, "u")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags/"+flag.ID, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("get flag: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != flag.ID {
		t.Errorf("id: want %s, got %v", flag.ID, resp["id"])
	}
}

func TestHandleGetFlag_NotFound(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "MissingFlag", "missingflag")
	cookie, _ := sessionAndCSRF(t, srv, "u")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags/does-not-exist", cookie)
	if w.Code != http.StatusNotFound {
		t.Errorf("get missing flag: want 404, got %d", w.Code)
	}
}

func TestHandleDeleteFlag(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "DeleteFlag", "deleteflag")
	flag, _ := store.CreateFlag(ctx, domain.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "kill_me",
		Name:           "Kill Me",
		FlagType:       "release",
		Variants:       `{"on":true}`,
		DefaultVariant: "on",
		Split:          `{"on":100}`,
		Status:         "active",
	})
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	// DELETE with CSRF header set on the request manually (deleteReq doesn't take one).
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/projects/"+p.ID+"/flags/"+flag.ID, nil)
	req.AddCookie(cookie)
	req.Header.Set("X-FunnelBarn-CSRF", csrf)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Errorf("delete flag: want 2xx, got %d (body: %s)", w.Code, w.Body.String())
	}

	// Confirm it's gone.
	w2 := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags/"+flag.ID, cookie)
	if w2.Code != http.StatusNotFound {
		t.Errorf("after delete: want 404, got %d", w2.Code)
	}
}

// ---------------------------------------------------------------------------
// flag_kind
// ---------------------------------------------------------------------------

// A flag created without a kind is an experiment — what every flag was before
// the column existed.
func TestHandleCreateFlag_KindDefaultsToExperiment(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, _ := store.CreateProject(context.Background(), "KindDefault", "kinddefault")
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags", map[string]any{
		"flag_key": "k", "name": "K", "variants": `{"on":true}`, "default_variant": "on",
	}, cookie, csrf)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["flag_kind"] != domain.FlagKindExperiment {
		t.Errorf("flag_kind: want experiment, got %v", resp["flag_kind"])
	}
}

// A partial update must not silently turn a config flag back into an
// experiment — that would start writing an evaluation row per read again.
func TestHandleUpdateFlag_PreservesKind(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "KindKeep", "kindkeep")
	f, err := store.CreateFlag(ctx, domain.FeatureFlag{
		ProjectID: p.ID, FlagKey: "cap", Name: "Cap", FlagType: "number",
		Variants: `{"default":10}`, DefaultVariant: "default", Split: `{"default":100}`,
		TargetingRules: "[]", Status: "active", Kind: domain.FlagKindConfig,
	})
	if err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := putJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/"+f.ID, map[string]any{
		"name": "Cap (renamed)", "variants": `{"default":25}`,
		"default_variant": "default", "split": `{"default":100}`, "status": "active",
	}, cookie, csrf)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["flag_kind"] != domain.FlagKindConfig {
		t.Errorf("flag_kind: want config preserved through a partial update, got %v", resp["flag_kind"])
	}
}

// A service toggling a gate sends {"status": "paused"} and nothing else. The
// flag's conversion event has to survive it, or flag analysis reports no
// conversions from then on (#495).
func TestHandleUpdateFlag_StatusOnlyKeepsConversionEvent(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ConvKeep", "convkeep")
	f, err := store.CreateFlag(ctx, domain.FeatureFlag{
		ProjectID: p.ID, FlagKey: "gate", Name: "Gate", FlagType: "boolean",
		Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: `{"on":50,"off":50}`,
		ConversionEvent: "signup", TargetingRules: "[]", Status: "active",
	})
	if err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := putJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/"+f.ID, map[string]any{"status": "paused"}, cookie, csrf)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["conversion_event"] != "signup" {
		t.Errorf("conversion_event: want signup kept through a status-only update, got %v", resp["conversion_event"])
	}
	got, err := store.Reader().FlagByID(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFlag: %v", err)
	}
	if got.ConversionEvent != "signup" {
		t.Errorf("stored conversion_event: want signup, got %q", got.ConversionEvent)
	}
}

// An explicit empty conversion_event still clears it: the edit form sends the
// field on every save, so the merge has to tell an omitted field from an empty
// one.
func TestHandleUpdateFlag_EmptyConversionEventClearsIt(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "ConvClear", "convclear")
	f, err := store.CreateFlag(ctx, domain.FeatureFlag{
		ProjectID: p.ID, FlagKey: "gate", Name: "Gate", FlagType: "boolean",
		Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: `{"on":50,"off":50}`,
		ConversionEvent: "signup", TargetingRules: "[]", Status: "active",
	})
	if err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := putJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/"+f.ID, map[string]any{"conversion_event": ""}, cookie, csrf)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	got, err := store.Reader().FlagByID(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFlag: %v", err)
	}
	if got.ConversionEvent != "" {
		t.Errorf("stored conversion_event: want cleared, got %q", got.ConversionEvent)
	}
}

// A typo would otherwise create a flag whose evaluation semantics nobody can
// predict, so an unknown kind is refused outright.
func TestHandleCreateFlag_RejectsUnknownKind(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, _ := store.CreateProject(context.Background(), "KindBad", "kindbad")
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags", map[string]any{
		"flag_key": "k", "name": "K", "flag_kind": "configuration",
	}, cookie, csrf)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422 for an unknown flag_kind, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// putJSONWithCSRF is putJSON with the CSRF header the mutating routes require.
func putJSONWithCSRF(t *testing.T, srv *Server, path string, body any, cookie *http.Cookie, csrfToken string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body) //nolint:errcheck
	req := httptest.NewRequest(http.MethodPut, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FunnelBarn-CSRF", csrfToken)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// A config flag records no evaluations, so its variant/conversion report would
// be an empty table dressed up as a result. The endpoint says so instead.
func TestHandleFlagAnalysis_ConfigKindReportsUnavailable(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "KindAnalysis", "kindanalysis")
	f, err := store.CreateFlag(ctx, domain.FeatureFlag{
		ProjectID: p.ID, FlagKey: "cap", Name: "Cap", FlagType: "number",
		Variants: `{"default":10}`, DefaultVariant: "default", Split: `{"default":100}`,
		TargetingRules: "[]", Status: "active", Kind: domain.FlagKindConfig,
	})
	if err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	cookie, _ := sessionAndCSRF(t, srv, "u")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+p.ID+"/flags/"+f.ID+"/analysis", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["unavailable"] != "config" {
		t.Errorf(`want unavailable="config", got %v`, resp["unavailable"])
	}
	if results, ok := resp["results"].([]any); !ok || len(results) != 0 {
		t.Errorf("want an empty results list, got %v", resp["results"])
	}
}

// A rule naming a variant the flag lacks can never be served, so create and
// update refuse it with a 422 and nothing is stored (#447).
func TestHandleFlagWrite_RejectsRuleNamingMissingVariant(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "RuleVariant", "rulevariant")
	cookie, csrf := sessionAndCSRF(t, srv, "u")
	rule := `[{"name":"testing sends","match":"all","conditions":[{"context_key":"environment","operator":"eq","value":"testing"}],"variant":"on"}]`

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags", map[string]any{
		"flag_key": "outreach", "name": "Outreach", "variants": `{"default":false}`,
		"default_variant": "default", "targeting_rules": rule,
	}, cookie, csrf)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create: want 422, got %d (body: %s)", w.Code, w.Body.String())
	}
	if flags, _ := store.ListFlags(ctx, p.ID); len(flags) != 0 {
		t.Errorf("a rejected create must store nothing, got %d flags", len(flags))
	}

	flag, err := store.CreateFlag(ctx, domain.FeatureFlag{
		ProjectID: p.ID, FlagKey: "ok", Name: "Ok", FlagType: "boolean",
		Variants: `{"default":false}`, DefaultVariant: "default", Split: "{}", TargetingRules: "[]", Status: "active",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	w = putJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/"+flag.ID, map[string]any{"targeting_rules": rule}, cookie, csrf)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("update: want 422, got %d (body: %s)", w.Code, w.Body.String())
	}
}
