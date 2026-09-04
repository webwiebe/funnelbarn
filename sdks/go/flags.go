package funnelbarn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultFlagCacheTTL is how long a resolved flag is reused when the server
// does not advertise a cache hint of its own — an older FunnelBarn, or one
// where the field is absent. A server that does send `cache_max_age_seconds`
// always wins, including when it sends 0 to mean "do not cache" (which is what
// an experiment sends: it resolves per targeting key and every read is a data
// point, so caching one would silently drop the analytics).
const DefaultFlagCacheTTL = 60 * time.Second

// flagEvalTimeout bounds a single evaluation. A flag lookup sits in front of
// application work; it must fail fast to the caller's default rather than hold
// a request open.
const flagEvalTimeout = 3 * time.Second

// EvalResult is the full resolution detail for a flag, for callers that care
// about which variant they got and why.
type EvalResult struct {
	FlagKey string
	// Value is the resolved value, or the caller's default on any failure.
	Value   any
	Variant string
	// Reason is the server's resolution reason — SPLIT, TARGETING_MATCH,
	// STATIC, DISABLED — or ERROR when the value is the caller's default.
	Reason string
	// ErrorCode is set when Reason is ERROR (FLAG_NOT_FOUND,
	// AUTO_REGISTER_LIMIT, GENERAL) or when the SDK itself could not reach
	// the server (TRANSPORT).
	ErrorCode string
	// CacheMaxAgeSeconds is the server's advertised cache hint, if any.
	CacheMaxAgeSeconds int
	// Cached reports whether this result came from the in-process cache.
	Cached bool

	// serverHinted records that CacheMaxAgeSeconds came from the server, so a
	// hint of 0 ("do not cache") is distinguishable from no hint at all.
	serverHinted bool
}

// wireEvalResult mirrors the JSON body of POST /api/v1/evaluate.
type wireEvalResult struct {
	FlagKey   string `json:"flag_key"`
	Value     any    `json:"value"`
	Variant   string `json:"variant"`
	Reason    string `json:"reason"`
	ErrorCode string `json:"error_code"`
	// Pointer so "the server said 0" (don't cache) is distinguishable from
	// "the server said nothing" (fall back to the configured TTL).
	CacheMaxAgeSeconds *int `json:"cache_max_age_seconds"`
}

// Evaluate resolves a flag, returning def on every failure mode — a network
// error, a non-200, reason ERROR, an unknown flag, or a malformed body. It
// never returns an error the caller can accidentally ignore: an outage must
// mean "the caller's default", never "unset".
//
// A paused or not-yet-activated flag (reason DISABLED) returns the value the
// *server* sends, which is the flag's own default variant — not def. That is
// what makes an auto-registered flag configurable from the dashboard before
// anyone activates it.
func Evaluate(ctx context.Context, key string, def any, evalCtx map[string]any) EvalResult {
	mu.Lock()
	t, o := tp, opts
	mu.Unlock()

	if key == "" {
		return EvalResult{FlagKey: key, Value: def, Reason: "ERROR", ErrorCode: "GENERAL"}
	}
	if t == nil || t.opts.Endpoint == "" {
		// Not initialised: behave exactly like an outage.
		return EvalResult{FlagKey: key, Value: def, Reason: "ERROR", ErrorCode: "TRANSPORT"}
	}

	cacheKey := flagCacheKey(key, evalCtx)
	if hit, ok := t.flags.get(cacheKey); ok {
		hit.Cached = true
		return hit
	}

	res := t.evaluate(ctx, key, def, evalCtx)
	if ttl := effectiveFlagTTL(res, o.FlagCacheTTL); ttl > 0 {
		t.flags.put(cacheKey, res, ttl)
	}
	return res
}

// EvaluateBool resolves a boolean flag, falling back to def.
func EvaluateBool(ctx context.Context, key string, def bool, evalCtx map[string]any) bool {
	if v, ok := Evaluate(ctx, key, def, evalCtx).Value.(bool); ok {
		return v
	}
	return def
}

// EvaluateInt resolves an integer flag, falling back to def. JSON has one
// number type, so an int-valued flag arrives as a float64; this does the
// convert-and-truncate every caller would otherwise write by hand.
func EvaluateInt(ctx context.Context, key string, def int, evalCtx map[string]any) int {
	switch v := Evaluate(ctx, key, def, evalCtx).Value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return def
}

// EvaluateString resolves a string flag, falling back to def.
func EvaluateString(ctx context.Context, key, def string, evalCtx map[string]any) string {
	if v, ok := Evaluate(ctx, key, def, evalCtx).Value.(string); ok {
		return v
	}
	return def
}

// effectiveFlagTTL decides how long to keep a result. An ERROR result is the
// caller's own default, never a resolved value, so it is never cached — a
// blip must not pin the default in place for a minute.
func effectiveFlagTTL(res EvalResult, configured time.Duration) time.Duration {
	if res.Reason == "ERROR" {
		return 0
	}
	if res.serverHinted {
		return time.Duration(res.CacheMaxAgeSeconds) * time.Second
	}
	if configured < 0 {
		return 0
	}
	if configured == 0 {
		return DefaultFlagCacheTTL
	}
	return configured
}

// flagCacheKey identifies a result. The context is part of it: targeting rules
// and bucketing read it, so two callers with different contexts can legitimately
// resolve to different variants and must not share a cache entry.
func flagCacheKey(key string, evalCtx map[string]any) string {
	if len(evalCtx) == 0 {
		return key
	}
	names := make([]string, 0, len(evalCtx))
	for k := range evalCtx {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(key)
	for _, n := range names {
		b.WriteByte('\x1f')
		b.WriteString(n)
		b.WriteByte('=')
		fmt.Fprintf(&b, "%v", evalCtx[n])
	}
	return b.String()
}

// --------------------------------------------------------------------------
// Transport
// --------------------------------------------------------------------------

func (t *transport) evaluate(ctx context.Context, key string, def any, evalCtx map[string]any) EvalResult {
	fail := func(code string) EvalResult {
		return EvalResult{FlagKey: key, Value: def, Reason: "ERROR", ErrorCode: code}
	}

	if evalCtx == nil {
		evalCtx = map[string]any{}
	}
	body, err := json.Marshal(map[string]any{
		"flag_key":      key,
		"default_value": def,
		"context":       evalCtx,
		"kind":          t.opts.FlagKind,
	})
	if err != nil {
		return fail("GENERAL")
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, flagEvalTimeout)
	defer cancel()

	// Endpoint is normalised by newTransport, so it is a bare base URL here.
	url := t.opts.Endpoint + "/api/v1/evaluate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fail("TRANSPORT")
	}
	req.Header.Set("Content-Type", "application/json")
	// Note for anyone reading this while debugging a 401: the evaluate endpoint
	// reads these two headers only. Authorization and x-api-key both fail.
	req.Header.Set("x-funnelbarn-api-key", t.opts.APIKey)
	if t.opts.ProjectName != "" {
		req.Header.Set("x-funnelbarn-project", t.opts.ProjectName)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fail("TRANSPORT")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fail("TRANSPORT")
	}

	var wire wireEvalResult
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return fail("GENERAL")
	}
	if wire.Reason == "ERROR" || wire.Reason == "" {
		code := wire.ErrorCode
		if code == "" {
			code = "GENERAL"
		}
		return fail(code)
	}

	res := EvalResult{
		FlagKey:   key,
		Value:     wire.Value,
		Variant:   wire.Variant,
		Reason:    wire.Reason,
		ErrorCode: wire.ErrorCode,
	}
	if wire.CacheMaxAgeSeconds != nil {
		res.CacheMaxAgeSeconds = *wire.CacheMaxAgeSeconds
		res.serverHinted = true
	}
	return res
}

// --------------------------------------------------------------------------
// In-process cache
// --------------------------------------------------------------------------

type flagCacheEntry struct {
	res       EvalResult
	expiresAt time.Time
}

type flagCache struct {
	mu  sync.Mutex
	m   map[string]flagCacheEntry
	now func() time.Time
}

func newFlagCache() *flagCache {
	return &flagCache{m: make(map[string]flagCacheEntry), now: time.Now}
}

func (c *flagCache) get(key string) (EvalResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return EvalResult{}, false
	}
	if c.now().After(e.expiresAt) {
		delete(c.m, key)
		return EvalResult{}, false
	}
	return e.res, true
}

func (c *flagCache) put(key string, res EvalResult, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = flagCacheEntry{res: res, expiresAt: c.now().Add(ttl)}
}
