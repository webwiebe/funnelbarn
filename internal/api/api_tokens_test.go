package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// newFlagTokenServer builds a server whose ingest authorizer resolves real
// api_keys rows, plus a static instance-wide key, so both the scoped-token and
// global-key paths can be exercised.
func newFlagTokenServer(t *testing.T) (*Server, *repository.Store) {
	t.Helper()
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	authz := auth.New("global-instance-key").WithDBLookup(store.ValidAPIKeySHA256, store.TouchAPIKey)
	sm := auth.NewSessionManager("test-secret", time.Hour)
	userAuth, _ := auth.NewUserAuthenticator("admin", "pw", "")

	srv := NewServer(ServerConfig{
		Ingest:              ingest.NewHandler(authz, sp, 0),
		Projects:            service.NewProjectService(store),
		Funnels:             service.NewFunnelService(store),
		ABTests:             service.NewABTestService(store),
		Flags:               service.NewFlagService(store),
		Events:              service.NewEventService(store),
		Overview:            service.NewOverviewService(store),
		Sessions:            service.NewSessionService(store),
		APIKeys:             service.NewAPIKeyService(store),
		Widgets:             service.NewWidgetService(store),
		UserAuth:            userAuth,
		SessionManager:      sm,
		WebSessions:         store,
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
	})
	return srv, store
}

func scopedKey(t *testing.T, store *repository.Store, projectID, plaintext, scope string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(plaintext))
	if _, err := store.CreateAPIKey(context.Background(), "test-"+scope, projectID, hex.EncodeToString(sum[:]), scope); err != nil {
		t.Fatalf("CreateAPIKey(%s): %v", scope, err)
	}
	return plaintext
}

func seedFlag(t *testing.T, store *repository.Store, projectID, key string) repository.FeatureFlag {
	t.Helper()
	f, err := store.CreateFlag(context.Background(), repository.FeatureFlag{
		ProjectID: projectID, FlagKey: key, Name: key, FlagType: "boolean",
		Variants: `{"on":true,"off":false}`, DefaultVariant: "on",
		Split: `{"on":100,"off":0}`, TargetingRules: "[]", Status: "active",
	})
	if err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	return f
}

func withKey(t *testing.T, srv *Server, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(auth.HeaderAPIKey, key)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func TestFlagToken_ReadsWithoutSession(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "Scherpstel", "scherpstel")
	seedFlag(t, store, p.ID, "outbound_email")
	key := scopedKey(t, store, p.ID, "read-key", repository.APIKeyScopeFlagsRead)

	w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/flags", key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Flags []repository.FeatureFlag `json:"flags"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Flags) != 1 || resp.Flags[0].FlagKey != "outbound_email" {
		t.Fatalf("unexpected flags: %+v", resp.Flags)
	}
	// The point of read access: report what the project holds, not what the
	// caller's own client resolved.
	if resp.Flags[0].Status != "active" {
		t.Errorf("status should be the project's real state, got %q", resp.Flags[0].Status)
	}
}

// A read token must not be able to change anything.
func TestFlagToken_ReadScopeCannotWrite(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	f := seedFlag(t, store, p.ID, "gate")
	key := scopedKey(t, store, p.ID, "read-key", repository.APIKeyScopeFlagsRead)

	w := withKey(t, srv, http.MethodPut, "/api/v1/projects/"+p.ID+"/flags/"+f.ID, key,
		map[string]any{"status": "paused"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for a read token attempting a write, got %d (body: %s)", w.Code, w.Body.String())
	}
	after, _ := store.FlagByID(context.Background(), f.ID)
	if after.Status != "active" {
		t.Errorf("the flag must be unchanged, got status %q", after.Status)
	}
}

// ---------------------------------------------------------------------------
// Write
// ---------------------------------------------------------------------------

// The operator surface's whole job: turning the gate off is one deliberate
// action, not a trip to a second dashboard.
func TestFlagToken_WriteTogglesFlag(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	f := seedFlag(t, store, p.ID, "outbound_email")
	key := scopedKey(t, store, p.ID, "write-key", repository.APIKeyScopeFlagsWrite)

	w := withKey(t, srv, http.MethodPut, "/api/v1/projects/"+p.ID+"/flags/"+f.ID, key,
		map[string]any{"status": "paused"})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	after, _ := store.FlagByID(context.Background(), f.ID)
	if after.Status != "paused" {
		t.Errorf("status: want paused, got %q", after.Status)
	}
	// A partial update must not blank out everything the caller didn't send.
	if after.Name != f.Name || after.Variants != f.Variants || after.DefaultVariant != f.DefaultVariant {
		t.Errorf("partial update clobbered the flag: %+v", after)
	}
}

// A write token can read too — read is implied by write.
func TestFlagToken_WriteScopeCanRead(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedFlag(t, store, p.ID, "gate")
	key := scopedKey(t, store, p.ID, "write-key", repository.APIKeyScopeFlagsWrite)

	if w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/flags", key, nil); w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

// Auto-registration covers creation, and a token that can turn a gate off
// should not also be able to delete the gate.
func TestFlagToken_CannotCreateOrDelete(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	f := seedFlag(t, store, p.ID, "gate")
	key := scopedKey(t, store, p.ID, "write-key", repository.APIKeyScopeFlagsWrite)

	w := withKey(t, srv, http.MethodPost, "/api/v1/projects/"+p.ID+"/flags", key,
		map[string]any{"flag_key": "new", "name": "New"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("create: want 401 (session-only route), got %d", w.Code)
	}

	w = withKey(t, srv, http.MethodDelete, "/api/v1/projects/"+p.ID+"/flags/"+f.ID, key, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("delete: want 401 (session-only route), got %d", w.Code)
	}
	if _, err := store.FlagByID(context.Background(), f.ID); err != nil {
		t.Errorf("the flag must still exist: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Scoping
// ---------------------------------------------------------------------------

// rapid-root's scherpstel token must not be able to touch places.
func TestFlagToken_CannotCrossProjects(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	ctx := context.Background()
	mine, _ := store.CreateProject(ctx, "Scherpstel", "scherpstel")
	theirs, _ := store.CreateProject(ctx, "Places", "places")
	theirFlag := seedFlag(t, store, theirs.ID, "their_gate")
	key := scopedKey(t, store, mine.ID, "write-key", repository.APIKeyScopeFlagsWrite)

	w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+theirs.ID+"/flags", key, nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("listing another project: want 403, got %d (body: %s)", w.Code, w.Body.String())
	}

	w = withKey(t, srv, http.MethodPut, "/api/v1/projects/"+theirs.ID+"/flags/"+theirFlag.ID, key,
		map[string]any{"status": "paused"})
	if w.Code != http.StatusForbidden {
		t.Errorf("writing another project's flag: want 403, got %d", w.Code)
	}
}

// The URL's project must be more than decoration: addressing another project's
// flag through your own project's URL has to fail too.
func TestFlagToken_CannotReachForeignFlagIDThroughOwnProject(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	ctx := context.Background()
	mine, _ := store.CreateProject(ctx, "Scherpstel", "scherpstel")
	theirs, _ := store.CreateProject(ctx, "Places", "places")
	theirFlag := seedFlag(t, store, theirs.ID, "their_gate")
	key := scopedKey(t, store, mine.ID, "write-key", repository.APIKeyScopeFlagsWrite)

	// 404, not 403: a caller scoped elsewhere shouldn't learn the ID exists.
	w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+mine.ID+"/flags/"+theirFlag.ID, key, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d (body: %s)", w.Code, w.Body.String())
	}

	w = withKey(t, srv, http.MethodPut, "/api/v1/projects/"+mine.ID+"/flags/"+theirFlag.ID, key,
		map[string]any{"status": "paused"})
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
	after, _ := store.FlagByID(ctx, theirFlag.ID)
	if after.Status != "active" {
		t.Errorf("the other project's flag must be unchanged, got %q", after.Status)
	}
}

// An ingest key is for events. It must not read or toggle flags.
func TestFlagToken_IngestScopeRejected(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedFlag(t, store, p.ID, "gate")
	key := scopedKey(t, store, p.ID, "ingest-key", repository.APIKeyScopeIngest)

	if w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/flags", key, nil); w.Code != http.StatusForbidden {
		t.Errorf("want 403 for an ingest-scoped key, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// The instance-wide env-var key resolves with no project, which would make it a
// master key for every project's flags.
func TestFlagToken_InstanceWideKeyRejected(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedFlag(t, store, p.ID, "gate")

	w := withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/flags", "global-instance-key", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for the instance-wide key, got %d (body: %s)", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("project-scoped")) {
		t.Errorf("the message should name the fix, got %s", w.Body.String())
	}
}

// A revoked or mistyped key must not fall through to whatever session happens
// to be on the same browser.
func TestFlagToken_BadKeyDoesNotFallBackToSession(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedFlag(t, store, p.ID, "gate")
	cookie := sessionCookieFor(t, srv, "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+p.ID+"/flags", nil)
	req.Header.Set(auth.HeaderAPIKey, "revoked-key")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// A dashboard session keeps working exactly as before.
func TestFlagToken_SessionStillWorks(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedFlag(t, store, p.ID, "gate")
	cookie := sessionCookieFor(t, srv, "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+p.ID+"/flags", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// scopeAllows
// ---------------------------------------------------------------------------

func TestScopeAllows(t *testing.T) {
	cases := []struct {
		have, want string
		ok         bool
	}{
		{repository.APIKeyScopeFull, repository.APIKeyScopeFlagsRead, true},
		{repository.APIKeyScopeFull, repository.APIKeyScopeFlagsWrite, true},
		{repository.APIKeyScopeFlagsWrite, repository.APIKeyScopeFlagsRead, true},
		{repository.APIKeyScopeFlagsWrite, repository.APIKeyScopeFlagsWrite, true},
		{repository.APIKeyScopeFlagsRead, repository.APIKeyScopeFlagsRead, true},
		{repository.APIKeyScopeFlagsRead, repository.APIKeyScopeFlagsWrite, false},
		{repository.APIKeyScopeIngest, repository.APIKeyScopeFlagsRead, false},
		{repository.APIKeyScopeIngest, repository.APIKeyScopeFlagsWrite, false},
		{"something-new", repository.APIKeyScopeFlagsRead, false},

		// analytics:read is its own axis: full covers it, flag scopes do not,
		// and it grants nothing on the flag routes in return.
		{repository.APIKeyScopeFull, repository.APIKeyScopeAnalyticsRead, true},
		{repository.APIKeyScopeAnalyticsRead, repository.APIKeyScopeAnalyticsRead, true},
		{repository.APIKeyScopeAnalyticsRead, repository.APIKeyScopeFlagsRead, false},
		{repository.APIKeyScopeAnalyticsRead, repository.APIKeyScopeFlagsWrite, false},
		{repository.APIKeyScopeFlagsRead, repository.APIKeyScopeAnalyticsRead, false},
		{repository.APIKeyScopeFlagsWrite, repository.APIKeyScopeAnalyticsRead, false},
		{repository.APIKeyScopeIngest, repository.APIKeyScopeAnalyticsRead, false},
		{"something-new", repository.APIKeyScopeAnalyticsRead, false},
	}
	for _, c := range cases {
		if got := scopeAllows(c.have, c.want); got != c.ok {
			t.Errorf("scopeAllows(%q, %q) = %v, want %v", c.have, c.want, got, c.ok)
		}
	}
}

// ---------------------------------------------------------------------------
// Issuance
// ---------------------------------------------------------------------------

func TestCreateAPIKey_AcceptsFlagScopes(t *testing.T) {
	srv, store := newAuthedServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	for _, scope := range []string{repository.APIKeyScopeFlagsRead, repository.APIKeyScopeFlagsWrite} {
		w := postJSONWithCSRF(t, srv, "/api/v1/apikeys", map[string]any{
			"project_id": p.ID, "name": "flags " + scope, "scope": scope,
		}, cookie, csrf)
		if w.Code != http.StatusCreated {
			t.Fatalf("scope %s: want 201, got %d (body: %s)", scope, w.Code, w.Body.String())
		}
		var resp struct {
			Key     string `json:"key"`
			APIKey  safeKey
			RawBody map[string]any
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp.RawBody)
		apiKey, _ := resp.RawBody["api_key"].(map[string]any)
		if apiKey["scope"] != scope {
			t.Errorf("scope: want %s, got %v", scope, apiKey["scope"])
		}
	}

	w := postJSONWithCSRF(t, srv, "/api/v1/apikeys", map[string]any{
		"project_id": p.ID, "name": "bogus", "scope": "flags:admin",
	}, cookie, csrf)
	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Errorf("an unknown scope must be refused, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// last_used_at is what tells a token something depends on from one that was
// issued and forgotten, before you revoke it.
func TestAPIKeyList_ReportsLastUsed(t *testing.T) {
	srv, store := newFlagTokenServer(t)
	p, _ := store.CreateProject(context.Background(), "P", "p")
	seedFlag(t, store, p.ID, "gate")
	key := scopedKey(t, store, p.ID, "read-key", repository.APIKeyScopeFlagsRead)
	cookie := sessionCookieFor(t, srv, "admin")

	read := func() map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/apikeys?project_id="+p.ID, nil)
		req.AddCookie(cookie)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		var resp struct {
			APIKeys []map[string]any `json:"api_keys"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp.APIKeys) != 1 {
			t.Fatalf("want 1 key, got %d", len(resp.APIKeys))
		}
		return resp.APIKeys[0]
	}

	if got := read()["last_used_at"]; got != nil {
		t.Errorf("a never-used key should report no last_used_at, got %v", got)
	}
	withKey(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/flags", key, nil)
	if got := read()["last_used_at"]; got == nil || got == "" {
		t.Errorf("last_used_at should be set after the key authenticates, got %v", got)
	}
}
