package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// ---------------------------------------------------------------------------
// handlePlaygroundEvaluateFlag — session-authed eval used by the dashboard.
// ---------------------------------------------------------------------------

// playgroundFlag creates a project + simple boolean flag and returns both,
// plus the CSRF token a session-authed request needs.
func playgroundFlag(t *testing.T, srv *Server, store *repository.Store, slug string) (repository.Project, repository.FeatureFlag, *http.Cookie, string) {
	t.Helper()
	ctx := context.Background()
	p, err := store.CreateProject(ctx, "FlagEval-"+slug, "flageval-"+slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	flag, err := store.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "test_flag",
		Name:           "Test Flag",
		FlagType:       "release",
		Variants:       `{"on":true,"off":false}`,
		DefaultVariant: "off",
		Split:          `{"on":100,"off":0}`,
		Status:         "active",
	})
	if err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	// Mint a session token once and derive both the cookie and the matching
	// CSRF token from it — they have to come from the same token because the
	// CSRF check on the server side computes CSRFToken(cookie.Value).
	token, expires, err := srv.sessionManager.Create("test-user")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return p, flag, auth.SessionCookie(token, expires, false), auth.CSRFToken(token)
}

func TestPlaygroundEvaluate_HappyPath(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, flag, cookie, csrf := playgroundFlag(t, srv, store, "happy")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/evaluate", map[string]any{
		"flag_key": flag.FlagKey,
		"context":  map[string]any{"user_id": "u1"},
	}, cookie, csrf)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["flag_key"] != flag.FlagKey {
		t.Errorf("flag_key: want %q, got %v", flag.FlagKey, resp["flag_key"])
	}
	if _, ok := resp["variant"]; !ok {
		t.Error("response missing 'variant'")
	}
	if _, ok := resp["reason"]; !ok {
		t.Error("response missing 'reason'")
	}
}

func TestPlaygroundEvaluate_FlagNotFound_ReturnsErrorReason(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, _, cookie, csrf := playgroundFlag(t, srv, store, "notfound")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/evaluate", map[string]any{
		"flag_key":      "does_not_exist",
		"default_value": "fallback",
		"context":       map[string]any{},
	}, cookie, csrf)

	// The handler intentionally returns 200 with reason=ERROR so SDKs can fall
	// back to the default value rather than crashing on a 404.
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for missing flag, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["reason"] != "ERROR" {
		t.Errorf("reason: want ERROR, got %v", resp["reason"])
	}
	if resp["error_code"] != "FLAG_NOT_FOUND" {
		t.Errorf("error_code: want FLAG_NOT_FOUND, got %v", resp["error_code"])
	}
	if resp["value"] != "fallback" {
		t.Errorf("value: want fallback (default), got %v", resp["value"])
	}
}

func TestPlaygroundEvaluate_MissingFlagKey_Returns422(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, _, cookie, csrf := playgroundFlag(t, srv, store, "missingkey")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/evaluate", map[string]any{
		"context": map[string]any{"user_id": "u1"},
	}, cookie, csrf)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestPlaygroundEvaluate_NoSession_Returns401(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, flag, _, _ := playgroundFlag(t, srv, store, "noses")

	// No cookie, no CSRF — requireSession should reject before reaching the handler.
	w := postJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags/evaluate", map[string]any{
		"flag_key": flag.FlagKey,
	}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestPlaygroundEvaluate_MissingCSRF_Returns403(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, flag, cookie, _ := playgroundFlag(t, srv, store, "nocsrf")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/evaluate", map[string]any{
		"flag_key": flag.FlagKey,
	}, cookie, "" /* empty CSRF */)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestPlaygroundEvaluate_ContextSplitsEvenly(t *testing.T) {
	// Confirm the context map is actually being passed through — flag with a
	// 50/50 split should produce both variants across a handful of distinct
	// user_ids, not always the same one.
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FlagEval-Split", "flageval-split")
	flag, _ := store.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "split_test",
		Name:           "Split Test",
		FlagType:       "experiment",
		Variants:       `{"a":"a","b":"b"}`,
		DefaultVariant: "a",
		Split:          `{"a":50,"b":50}`,
		Status:         "active",
	})

	token, expires, _ := srv.sessionManager.Create("test-user")
	cookie := auth.SessionCookie(token, expires, false)
	csrf := auth.CSRFToken(token)

	// 50 distinct user IDs — overwhelmingly improbable to all bucket to the
	// same variant if the context is being honored.
	userIDs := []string{
		"alice", "bob", "carol", "dave", "eve", "frank", "grace", "heidi",
		"ivan", "judy", "kim", "leo", "mallory", "nick", "olivia", "peggy",
		"quentin", "ruth", "steve", "trent", "ursula", "victor", "wendy",
		"xavier", "yvonne", "zoe", "alex", "blake", "casey", "drew",
		"erin", "finley", "gale", "harper", "indigo", "jordan", "kai",
		"lane", "morgan", "noel", "oakley", "parker", "quinn", "river",
		"sage", "taylor", "umi", "val", "winter", "yael",
	}
	seen := map[string]int{}
	for _, uid := range userIDs {
		w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/evaluate", map[string]any{
			"flag_key": flag.FlagKey,
			"context":  map[string]any{"targeting_key": uid},
		}, cookie, csrf)
		if w.Code != http.StatusOK {
			t.Fatalf("user_id %q: want 200, got %d", uid, w.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if v, ok := resp["variant"].(string); ok {
			seen[v]++
		}
	}
	if len(seen) < 2 {
		t.Errorf("expected both variants across %d distinct user_ids, only saw %v", len(userIDs), seen)
	}
}

// ---------------------------------------------------------------------------
// handleClientConfig — public config, no auth.
// ---------------------------------------------------------------------------

func TestHandleClientConfig_ExposesBugbarnFields(t *testing.T) {
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	ingestHandler := ingest.NewHandler(auth.New("test-key"), sp, 0)
	sm := auth.NewSessionManager("test-secret", time.Hour)
	userAuth, _ := auth.NewUserAuthenticator("", "", "")

	srv := NewServer(ServerConfig{
		Ingest:              ingestHandler,
		Projects:            service.NewProjectService(store),
		Funnels:             service.NewFunnelService(store),
		ABTests:             service.NewABTestService(store),
		Flags:               service.NewFlagService(store),
		Events:              service.NewEventService(store),
		Sessions:            service.NewSessionService(store),
		APIKeys:             service.NewAPIKeyService(store),
		Widgets:             service.NewWidgetService(store),
		UserAuth:            userAuth,
		SessionManager:      sm,
		SessionSecret:       "test-secret",
		PublicURL:           "http://localhost",
		LoginRatePerMinute:  1000,
		LoginRateBurst:      1000,
		APIRatePerMinute:    1000,
		APIRateBurst:        1000,
		IngestRatePerMinute: 1000,
		IngestRateBurst:     1000,
		DB:                  store,
		Version:             "test",
		BugbarnEndpoint:     "https://bugbarn.example.com",
		BugbarnIngestKey:    "secret-key",
		BugbarnProject:      "funnelbarn",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/client-config", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["bugbarn_endpoint"] != "https://bugbarn.example.com" {
		t.Errorf("bugbarn_endpoint: got %v", resp["bugbarn_endpoint"])
	}
	if resp["bugbarn_ingest_key"] != "secret-key" {
		t.Errorf("bugbarn_ingest_key: got %v", resp["bugbarn_ingest_key"])
	}
	if resp["bugbarn_project"] != "funnelbarn" {
		t.Errorf("bugbarn_project: got %v (expected dogfood routing slug)", resp["bugbarn_project"])
	}
}
