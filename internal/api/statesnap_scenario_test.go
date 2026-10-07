package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/worker"
)

const (
	snapSDKKey     = "snap-sdk-key"
	snapUnusedKey  = "snap-unused-key"
	snapAutoMax    = 3
	snapSession1   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1"
	snapSession2   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb2"
	snapUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	snapEvalKeyHdr = auth.HeaderAPIKey
)

var snapT0 = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// snapStack is the full stack of the scenario: DB-backed API key auth (so
// TouchAPIKey runs), a real spool, project health wired as in main.go, and
// session auth for the playground.
type snapStack struct {
	srv     *Server
	store   *repository.Store
	sp      *spool.Spool
	handler *ingest.Handler
	// Flush drains the async writes. Nil today (writes are fire-and-forget
	// goroutines, so flushAsync only polls). PR 2 sets it to dispatcher.Flush.
	Flush func(context.Context) error
}

func (s *snapStack) flush(ctx context.Context) error {
	if s.Flush == nil {
		return nil
	}
	return s.Flush(ctx)
}

func newSnapStack(t *testing.T) *snapStack {
	t.Helper()
	store, err := repository.Open(filepath.Join(t.TempDir(), "snap.db"))
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	sp := newTestSpool(t)
	var base *auth.Authorizer // nil: only DB-stored keys are accepted
	authz := base.WithDBLookup(store.ValidAPIKeySHA256, store.TouchAPIKey)
	handler := ingest.NewHandler(authz, sp, 0)
	healthSvc := service.NewProjectHealthService(store)
	handler.OnEventsReceived = func(ctx context.Context, projectID string) {
		if _, err := healthSvc.MarkEventsReceived(ctx, projectID); err != nil {
			t.Errorf("MarkEventsReceived: %v", err)
		}
	}
	userAuth, _ := auth.NewUserAuthenticator("admin", "password", "")
	srv := NewServer(ServerConfig{
		Ingest:              handler,
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
		SessionManager:      auth.NewSessionManager("test-secret", time.Hour),
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
		ProjectHealth:       healthSvc,
		FlagAutoRegisterMax: snapAutoMax,
	})
	st := &snapStack{srv: srv, store: store, sp: sp, handler: handler}
	snapStacks.Store(store, st)
	t.Cleanup(func() { snapStacks.Delete(store) })
	return st
}

// seed creates the project, its two API keys and the four known flags.
func (s *snapStack) seed(t *testing.T) repository.Project {
	t.Helper()
	ctx := context.Background()
	p, err := s.store.CreateProject(ctx, "Snapshot", "snap")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	for name, key := range map[string]string{"sdk": snapSDKKey, "unused": snapUnusedKey} {
		if _, err := s.store.CreateAPIKey(ctx, name, p.ID, sha256Hex(key), "full"); err != nil {
			t.Fatalf("CreateAPIKey %s: %v", name, err)
		}
	}
	rules := `[{"name":"pro-users","variant":"on","match":"all","conditions":[{"context_key":"plan","operator":"eq","value":"pro"}]}]`
	base := repository.FeatureFlag{
		ProjectID: p.ID, FlagType: "boolean", Variants: `{"on":true,"off":false}`,
		DefaultVariant: "off", Split: `{"on":50,"off":50}`, Status: "active",
	}
	mk := func(mut func(f *repository.FeatureFlag)) {
		f := base
		mut(&f)
		if _, err := s.store.CreateFlag(ctx, f); err != nil {
			t.Fatalf("CreateFlag %s: %v", f.FlagKey, err)
		}
	}
	mk(func(f *repository.FeatureFlag) { f.FlagKey, f.Name, f.ConversionEvent = "exp_split", "Split", "signup" })
	mk(func(f *repository.FeatureFlag) { f.FlagKey, f.Name, f.TargetingRules = "exp_target", "Targeted", rules })
	mk(func(f *repository.FeatureFlag) {
		f.FlagKey, f.Name, f.Kind = "cfg_flag", "Config", repository.FlagKindConfig
		f.FlagType, f.Variants, f.DefaultVariant, f.Split = "string", `{"v1":"alpha","v2":"beta"}`, "v2", "{}"
	})
	mk(func(f *repository.FeatureFlag) { f.FlagKey, f.Name, f.Status = "off_flag", "Inactive", "inactive" })
	return p
}

type snapEvent struct {
	name, url, referrer, session, user, env string
	at                                      time.Duration
}

var snapEvents = []snapEvent{
	{"page_view", "https://example.com/?utm_source=news&utm_medium=email", "", snapSession1, "u1", "production", 0},
	{"signup", "https://example.com/signup", "https://example.com/", snapSession1, "u1", "prodution", time.Minute},
	{"page_view", "https://example.com/pricing", "https://www.google.com/search?q=x", snapSession2, "u2", "staging", 2 * time.Minute},
	{"page_view", "https://example.com/about", "", "", "", "", 3 * time.Minute},
}

func (s *snapStack) post(t *testing.T, path, apiKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", snapUserAgent)
	req.Header.Set(snapEvalKeyHdr, apiKey)
	w := httptest.NewRecorder()
	s.srv.ServeHTTP(w, req)
	return w
}

// ingestAndPersist posts the events through POST /api/v1/events, waits for the
// ingest queue to reach the spool, then persists each spool record the way the
// worker loop in cmd/funnelbarn/main.go does.
func (s *snapStack) ingestAndPersist(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.handler.Start(ctx) }()
	for _, e := range snapEvents {
		payload := map[string]any{
			"name": e.name, "url": e.url, "referrer": e.referrer, "session_id": e.session,
			"user_id": e.user, "environment": e.env, "timestamp": snapT0.Add(e.at).Format(time.RFC3339),
		}
		if w := s.post(t, "/api/v1/events", snapSDKKey, payload); w.Code != http.StatusAccepted {
			t.Fatalf("ingest %s: want 202, got %d: %s", e.name, w.Code, w.Body.String())
		}
	}
	// Wait for the real transition: all records are in the spool file.
	deadline := time.Now().Add(asyncTimeout)
	for {
		recs, err := spool.ReadRecords(s.sp.Path())
		if err == nil && len(recs) == len(snapEvents) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("spool did not reach %d records (err=%v)", len(snapEvents), err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	<-done

	recs, err := spool.ReadRecords(s.sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	// TODO(#302 PR2): replace this loop body with the exported per-record
	// persist function extracted from the worker loop in cmd/funnelbarn/main.go
	// (e.g. worker.PersistRecord) once production code may change, so the
	// harness exercises the same path as production.
	for _, rec := range recs {
		ev, err := worker.ProcessRecord(rec)
		if err != nil {
			t.Fatalf("ProcessRecord: %v", err)
		}
		proj, err := s.store.EnsureProject(context.Background(), rec.ProjectSlug)
		if err != nil {
			t.Fatalf("EnsureProject(%s): %v", rec.ProjectSlug, err)
		}
		ev.ProjectID = proj.ID
		if err := worker.PersistEvent(context.Background(), s.store, ev, nil); err != nil {
			t.Fatalf("PersistEvent: %v", err)
		}
	}
}

type snapEval struct {
	key, kind string
	def       any
	ctx       map[string]any
	// waitAuto: this evaluation auto-registers a flag; wait for its row.
	waitAuto bool
}

var snapEvals = []snapEval{
	{key: "exp_split", ctx: map[string]any{"targetingKey": "u1", "session_id": snapSession1}},
	{key: "exp_split", ctx: map[string]any{"targetingKey": "u2"}},
	{key: "exp_target", ctx: map[string]any{"targetingKey": "u3", "plan": "pro"}},
	{key: "exp_target", ctx: map[string]any{"targetingKey": "u4", "plan": "free"}},
	{key: "cfg_flag", def: "none", ctx: map[string]any{}},
	{key: "off_flag", def: false, ctx: map[string]any{"targetingKey": "u5"}},
	{key: "auto_exp", def: true, ctx: map[string]any{"targetingKey": "u6"}, waitAuto: true},
	{key: "auto_cfg", kind: "config", def: "x", ctx: map[string]any{}, waitAuto: true},
	{key: "auto_third", def: 5, ctx: map[string]any{"targetingKey": "u7"}, waitAuto: true},
	{key: "auto_over", def: 1, ctx: map[string]any{}},
	{key: "bad key!", def: "d", ctx: map[string]any{}},
	{key: "auto_exp", def: true, ctx: map[string]any{"targetingKey": "u8"}},
}

// evaluateAll runs the SDK and playground evaluations and returns one line per
// response so the goldens also pin what callers saw.
func (s *snapStack) evaluateAll(t *testing.T, p repository.Project) string {
	t.Helper()
	var out strings.Builder
	for _, e := range snapEvals {
		body := map[string]any{"flag_key": e.key, "default_value": e.def, "context": e.ctx}
		if e.kind != "" {
			body["kind"] = e.kind
		}
		w := s.post(t, "/api/v1/evaluate", snapSDKKey, body)
		fmt.Fprintf(&out, "sdk %s %d cache=%q %s\n", e.key, w.Code, w.Header().Get("Cache-Control"), compactJSON(t, w.Body.Bytes()))
		// Auto-registration is capped at snapAutoMax and may become async in
		// PR 2. Wait for each new row before the next call so the cap is hit by
		// auto_over deterministically, not by whichever write wins a race.
		if e.waitAuto {
			s.waitFlagRow(t, p.ID, e.key)
		}
	}
	cookie := sessionCookieFor(t, s.srv, "snap-user")
	csrf := s.srv.sessionManager.CSRFToken(cookie.Value)
	path := "/api/v1/projects/" + p.ID + "/flags/evaluate"
	for _, e := range []snapEval{
		{key: "exp_split", ctx: map[string]any{"targetingKey": "u9"}},
		{key: "playground_unknown", def: "d", ctx: map[string]any{}},
	} {
		w := postJSONWithCSRF(t, s.srv, path, map[string]any{"flag_key": e.key, "default_value": e.def, "context": e.ctx}, cookie, csrf)
		fmt.Fprintf(&out, "playground %s %d %s\n", e.key, w.Code, compactJSON(t, w.Body.Bytes()))
	}
	return strings.ReplaceAll(out.String(), p.ID, "<project:"+p.Slug+">")
}

// waitFlagRow flushes async writes, then polls until the feature flag row
// exists.
func (s *snapStack) waitFlagRow(t *testing.T, projectID, key string) {
	t.Helper()
	if err := s.flush(context.Background()); err != nil {
		t.Fatalf("waitFlagRow(%s): Flush: %v", key, err)
	}
	deadline := time.Now().Add(asyncTimeout)
	for {
		var n int
		err := s.store.DB().QueryRow(`SELECT COUNT(*) FROM feature_flags WHERE project_id = ? AND flag_key = ?`, projectID, key).Scan(&n)
		if err == nil && n > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("auto flag %q not registered within %s (err=%v)\n%s", key, asyncTimeout, err, dumpSnapshotTables(t, s.store.DB()))
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func compactJSON(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("response is not JSON: %v: %s", err, b)
	}
	return marshalSorted(v)
}

// TestStateSnapshot_IngestEvaluate pins the rows that ingest, the worker and
// the evaluate endpoints leave behind, including the bookkeeping writes
// (flag_evaluations, last_evaluated_at, api key use, project health).
func TestStateSnapshot_IngestEvaluate(t *testing.T) {
	s := newSnapStack(t)
	p := s.seed(t)

	s.ingestAndPersist(t)
	responses := s.evaluateAll(t, p)

	// Expected last_evaluated_at touches: every flag that evaluated without
	// error, once each (the touch is throttled to one per minute per flag).
	touched := []string{"exp_split", "exp_target", "cfg_flag", "off_flag", "auto_exp", "auto_cfg", "auto_third"}
	flushAsync(t, s.store, p.ID, touched)

	compareGolden(t, "responses", responses)
	snapshotAll(t, s.store)

	// The unused key must stay untouched while the SDK key was used.
	var used int
	if err := s.store.DB().QueryRow(`SELECT COUNT(*) FROM api_keys WHERE last_used_at IS NOT NULL`).Scan(&used); err != nil || used != 1 {
		t.Errorf("api keys with last_used_at: want 1, got %d (err=%v)", used, err)
	}
}
