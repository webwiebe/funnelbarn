package funnelbarn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// evalServer stands in for FunnelBarn's POST /api/v1/evaluate. handler decides
// what it answers; calls counts how many times it was reached, which is how the
// cache tests tell a hit from a miss.
func evalServer(t *testing.T, calls *int64, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			atomic.AddInt64(calls, 1)
		}
		if r.URL.Path != "/api/v1/evaluate" {
			t.Errorf("path: want /api/v1/evaluate, got %s", r.URL.Path)
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// initSDK points the package-level SDK at srv and tears it down after the test.
func initSDK(t *testing.T, endpoint string, ttl time.Duration) {
	t.Helper()
	Init(Options{APIKey: "k", Endpoint: endpoint, ProjectName: "p", FlagCacheTTL: ttl})
	t.Cleanup(func() { _ = Shutdown(time.Second) })
}

func respond(w http.ResponseWriter, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// ---------------------------------------------------------------------------
// Happy path
// ---------------------------------------------------------------------------

func TestEvaluate_ResolvesValue(t *testing.T) {
	srv := evalServer(t, nil, func(w http.ResponseWriter, _ *http.Request) {
		respond(w, map[string]any{
			"flag_key": "checkout", "variant": "on", "value": true,
			"reason": "SPLIT", "cache_max_age_seconds": 0,
		})
	})
	initSDK(t, srv.URL, 0)

	res := Evaluate(context.Background(), "checkout", false, map[string]any{"targeting_key": "u1"})
	if res.Value != true || res.Variant != "on" || res.Reason != "SPLIT" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !EvaluateBool(context.Background(), "checkout", false, nil) {
		t.Error("EvaluateBool should return the resolved true")
	}
}

func TestEvaluate_SendsExpectedRequest(t *testing.T) {
	var got struct {
		FlagKey string         `json:"flag_key"`
		Default any            `json:"default_value"`
		Context map[string]any `json:"context"`
		Kind    string         `json:"kind"`
	}
	var key, project string
	srv := evalServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("x-funnelbarn-api-key")
		project = r.Header.Get("x-funnelbarn-project")
		_ = json.NewDecoder(r.Body).Decode(&got)
		respond(w, map[string]any{"variant": "v", "value": "x", "reason": "STATIC"})
	})
	Init(Options{APIKey: "the-key", Endpoint: srv.URL, ProjectName: "the-project", FlagKind: "config"})
	defer Shutdown(time.Second) //nolint:errcheck

	Evaluate(context.Background(), "cap", 250, map[string]any{"targeting_key": "svc"})

	// The evaluate endpoint reads these two headers only — Authorization and
	// x-api-key both 401, which is the trap this SDK exists to remove.
	if key != "the-key" || project != "the-project" {
		t.Errorf("headers: got key=%q project=%q", key, project)
	}
	if got.FlagKey != "cap" || got.Default != float64(250) || got.Kind != "config" {
		t.Errorf("body: %+v", got)
	}
	if got.Context["targeting_key"] != "svc" {
		t.Errorf("context not forwarded: %+v", got.Context)
	}
}

// ---------------------------------------------------------------------------
// Every failure mode yields the caller's default
// ---------------------------------------------------------------------------

func TestEvaluate_FailureModesFallBackToDefault(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		code    string
	}{
		{"non-200", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}, "TRANSPORT"},
		{"unauthorized", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}, "TRANSPORT"},
		{"reason ERROR", func(w http.ResponseWriter, _ *http.Request) {
			respond(w, map[string]any{"reason": "ERROR", "error_code": "GENERAL", "value": nil})
		}, "GENERAL"},
		{"flag not found", func(w http.ResponseWriter, _ *http.Request) {
			respond(w, map[string]any{"reason": "ERROR", "error_code": "FLAG_NOT_FOUND", "value": nil})
		}, "FLAG_NOT_FOUND"},
		{"auto-register limit", func(w http.ResponseWriter, _ *http.Request) {
			respond(w, map[string]any{"reason": "ERROR", "error_code": "AUTO_REGISTER_LIMIT", "value": nil})
		}, "AUTO_REGISTER_LIMIT"},
		{"malformed body", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value": `))
		}, "GENERAL"},
		{"empty reason", func(w http.ResponseWriter, _ *http.Request) {
			respond(w, map[string]any{"value": true})
		}, "GENERAL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := evalServer(t, nil, tc.handler)
			initSDK(t, srv.URL, 0)

			res := Evaluate(context.Background(), "f", "my-default", nil)
			if res.Value != "my-default" {
				t.Errorf("value: want the caller's default, got %v", res.Value)
			}
			if res.Reason != "ERROR" || res.ErrorCode != tc.code {
				t.Errorf("want ERROR/%s, got %s/%s", tc.code, res.Reason, res.ErrorCode)
			}
		})
	}
}

// An unreachable server must mean "the caller's default", never "unset" — the
// direction that turns a FunnelBarn outage into unbounded cold email.
func TestEvaluate_UnreachableServer(t *testing.T) {
	srv := evalServer(t, nil, func(http.ResponseWriter, *http.Request) {})
	srv.Close()
	initSDK(t, srv.URL, 0)

	if EvaluateBool(context.Background(), "email_enabled", false, nil) {
		t.Error("an unreachable server must yield the caller's default (false)")
	}
	if got := EvaluateInt(context.Background(), "daily_cap", 25, nil); got != 25 {
		t.Errorf("daily_cap: want the caller's default 25, got %d", got)
	}
}

func TestEvaluate_NotInitialised(t *testing.T) {
	_ = Shutdown(time.Second)
	res := Evaluate(context.Background(), "f", 7, nil)
	if res.Value != 7 || res.ErrorCode != "TRANSPORT" {
		t.Errorf("uninitialised SDK should behave like an outage, got %+v", res)
	}
}

func TestEvaluate_EmptyKey(t *testing.T) {
	srv := evalServer(t, nil, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("an empty key must not reach the server")
	})
	initSDK(t, srv.URL, 0)

	if res := Evaluate(context.Background(), "", "d", nil); res.Value != "d" {
		t.Errorf("want the caller's default, got %+v", res)
	}
}

// ---------------------------------------------------------------------------
// DISABLED returns the server's value, not the caller's
// ---------------------------------------------------------------------------

// An auto-registered flag is inactive and carries its own default variant. That
// value — not the caller's — is what makes it configurable from the dashboard
// before anyone activates it.
func TestEvaluate_DisabledUsesServerValue(t *testing.T) {
	srv := evalServer(t, nil, func(w http.ResponseWriter, _ *http.Request) {
		respond(w, map[string]any{
			"variant": "default", "value": float64(500), "reason": "DISABLED",
			"cache_max_age_seconds": 60,
		})
	})
	initSDK(t, srv.URL, 0)

	if got := EvaluateInt(context.Background(), "daily_cap", 25, nil); got != 500 {
		t.Errorf("want the flag's own default 500, not the caller's 25, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Typed accessors
// ---------------------------------------------------------------------------

func TestTypedAccessors(t *testing.T) {
	values := map[string]any{
		"b": true,
		// JSON has one number type, so an int-valued flag arrives as float64.
		"i": float64(250),
		"s": "slow",
	}
	srv := evalServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			FlagKey string `json:"flag_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		respond(w, map[string]any{"variant": "v", "value": values[body.FlagKey], "reason": "STATIC"})
	})
	initSDK(t, srv.URL, -1) // caching off, so each call re-reads

	ctx := context.Background()
	if !EvaluateBool(ctx, "b", false, nil) {
		t.Error("EvaluateBool")
	}
	if got := EvaluateInt(ctx, "i", 0, nil); got != 250 {
		t.Errorf("EvaluateInt: want 250, got %d", got)
	}
	if got := EvaluateString(ctx, "s", "fast", nil); got != "slow" {
		t.Errorf("EvaluateString: want slow, got %q", got)
	}
	// A type mismatch falls back rather than panicking on the assertion.
	if EvaluateBool(ctx, "s", false, nil) {
		t.Error("a string value read as bool must fall back to the default")
	}
	if got := EvaluateInt(ctx, "s", 9, nil); got != 9 {
		t.Errorf("a string value read as int must fall back, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Cache
// ---------------------------------------------------------------------------

func TestEvaluate_CachesWithinTTL(t *testing.T) {
	var calls int64
	srv := evalServer(t, &calls, func(w http.ResponseWriter, _ *http.Request) {
		respond(w, map[string]any{"variant": "v", "value": float64(250), "reason": "STATIC", "cache_max_age_seconds": 60})
	})
	initSDK(t, srv.URL, 0)

	ctx := context.Background()
	for i := 0; i < 50; i++ {
		if got := EvaluateInt(ctx, "cap", 0, nil); got != 250 {
			t.Fatalf("call %d: got %d", i, got)
		}
	}
	if calls != 1 {
		t.Errorf("a hot loop should hit the server once, got %d calls", calls)
	}
	if !Evaluate(ctx, "cap", 0, nil).Cached {
		t.Error("a repeat read should be marked Cached")
	}
}

// The server's hint wins, including 0 — an experiment resolves per targeting
// key and every read is a data point, so caching one would drop the analytics.
func TestEvaluate_ServerHintOfZeroDisablesCache(t *testing.T) {
	var calls int64
	srv := evalServer(t, &calls, func(w http.ResponseWriter, _ *http.Request) {
		respond(w, map[string]any{"variant": "on", "value": true, "reason": "SPLIT", "cache_max_age_seconds": 0})
	})
	initSDK(t, srv.URL, time.Hour) // generous local TTL, overridden by the hint

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		EvaluateBool(ctx, "exp", false, nil)
	}
	if calls != 3 {
		t.Errorf("an experiment must not be cached; want 3 calls, got %d", calls)
	}
}

// A server that sends no hint at all (an older FunnelBarn) falls back to the
// configured TTL rather than hammering it.
func TestEvaluate_NoHintFallsBackToConfiguredTTL(t *testing.T) {
	var calls int64
	srv := evalServer(t, &calls, func(w http.ResponseWriter, _ *http.Request) {
		respond(w, map[string]any{"variant": "on", "value": true, "reason": "SPLIT"})
	})
	initSDK(t, srv.URL, time.Hour)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		EvaluateBool(ctx, "old", false, nil)
	}
	if calls != 1 {
		t.Errorf("want 1 call under the configured TTL, got %d", calls)
	}
}

func TestEvaluate_NegativeTTLDisablesCache(t *testing.T) {
	var calls int64
	srv := evalServer(t, &calls, func(w http.ResponseWriter, _ *http.Request) {
		respond(w, map[string]any{"variant": "on", "value": true, "reason": "SPLIT"})
	})
	initSDK(t, srv.URL, -1)

	for i := 0; i < 3; i++ {
		EvaluateBool(context.Background(), "f", false, nil)
	}
	if calls != 3 {
		t.Errorf("a negative TTL must disable the cache, got %d calls", calls)
	}
}

// A blip must not pin the caller's default in place for the whole TTL.
func TestEvaluate_ErrorsAreNotCached(t *testing.T) {
	var calls int64
	srv := evalServer(t, &calls, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.LoadInt64(&calls) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		respond(w, map[string]any{"variant": "on", "value": true, "reason": "SPLIT"})
	})
	initSDK(t, srv.URL, time.Hour)

	ctx := context.Background()
	if EvaluateBool(ctx, "f", false, nil) {
		t.Fatal("first call should fall back to the caller's default")
	}
	if !EvaluateBool(ctx, "f", false, nil) {
		t.Error("the next call should retry and get the real value, not a cached error")
	}
}

// Targeting rules and bucketing read the context, so two callers with different
// contexts can legitimately resolve differently and must not share an entry.
func TestEvaluate_CacheKeyIncludesContext(t *testing.T) {
	var calls int64
	srv := evalServer(t, &calls, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Context map[string]any `json:"context"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		respond(w, map[string]any{"variant": "v", "value": body.Context["targeting_key"], "reason": "SPLIT"})
	})
	initSDK(t, srv.URL, time.Hour)

	ctx := context.Background()
	a := EvaluateString(ctx, "f", "", map[string]any{"targeting_key": "user-a"})
	b := EvaluateString(ctx, "f", "", map[string]any{"targeting_key": "user-b"})
	if a != "user-a" || b != "user-b" {
		t.Errorf("contexts must not share a cache entry: got %q and %q", a, b)
	}
	if calls != 2 {
		t.Errorf("want 2 calls for 2 distinct contexts, got %d", calls)
	}
	// Key order must not matter.
	if k1, k2 := flagCacheKey("f", map[string]any{"a": 1, "b": 2}), flagCacheKey("f", map[string]any{"b": 2, "a": 1}); k1 != k2 {
		t.Errorf("cache key should be order-independent: %q vs %q", k1, k2)
	}
}

func TestFlagCache_Expiry(t *testing.T) {
	c := newFlagCache()
	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }

	c.put("k", EvalResult{Value: 1}, time.Minute)
	if _, ok := c.get("k"); !ok {
		t.Fatal("entry should be live immediately after put")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := c.get("k"); ok {
		t.Error("entry should be gone once its TTL has passed")
	}
}

// A cancelled context fails to the caller's default rather than blocking.
func TestEvaluate_RespectsContextCancellation(t *testing.T) {
	srv := evalServer(t, nil, func(w http.ResponseWriter, _ *http.Request) {
		respond(w, map[string]any{"variant": "on", "value": true, "reason": "SPLIT"})
	})
	initSDK(t, srv.URL, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if EvaluateBool(ctx, "f", false, nil) {
		t.Error("a cancelled context must yield the caller's default")
	}
}
