package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const setupInsecureFallback = "funnelbarn-setup-insecure"

// setupKey derives a deterministic ingest API key from the session secret and slug.
// Returns (plaintext40, keySHA256).
func setupKey(secret, slug string) (plaintext, keySHA256 string) {
	if secret == "" {
		slog.Warn("session secret is empty; using insecure fallback for setup key derivation")
		secret = setupInsecureFallback
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("setup:" + slug))
	raw := mac.Sum(nil)
	plaintext = hex.EncodeToString(raw)[:40]
	sum := sha256.Sum256([]byte(plaintext))
	keySHA256 = hex.EncodeToString(sum[:])
	return
}

// handleSetup is a public endpoint that returns a self-service setup page in
// Markdown format. It creates the project (status='pending') if it doesn't
// exist yet and upserts a deterministic ingest API key.
//
// GET /api/v1/setup/{slug}
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Ensure project exists (pending if new).
	project, err := s.projects.EnsureProjectPending(ctx, slug, slug)
	if err != nil {
		slog.Error("setup: ensure project", "slug", slug, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Derive deterministic ingest key.
	plaintext, keySHA256 := setupKey(s.sessionSecret, slug)

	// Upsert the setup key so it is valid for ingest.
	if err := s.projects.EnsureSetupAPIKey(ctx, project.ID, keySHA256); err != nil {
		slog.Error("setup: ensure api key", "project_id", project.ID, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	publicURL := s.publicURL
	if publicURL == "" {
		publicURL = "https://funnelbarn.wiebe.xyz"
	}

	endpoint := publicURL + "/api/v1/events"
	sdkURL := publicURL + "/sdk.js"
	setupURL := publicURL + "/api/v1/setup/" + slug
	now := time.Now().UTC().Format(time.RFC3339)

	b := &strings.Builder{}

	fmt.Fprintf(b, "# FunnelBarn Setup: %s\n\n", slug)
	fmt.Fprintf(b, "> **Status**: %s — this page is idempotent. Revisit at any time to retrieve the same configuration.\n\n", project.Status)
	fmt.Fprintf(b, "Generated: %s\n\n---\n\n", now)

	fmt.Fprintf(b, "## Project Configuration\n\n")
	fmt.Fprintf(b, "| Key        | Value |\n")
	fmt.Fprintf(b, "|------------|-------|\n")
	fmt.Fprintf(b, "| Endpoint   | %s |\n", endpoint)
	fmt.Fprintf(b, "| Project    | %s |\n", slug)
	fmt.Fprintf(b, "| API Key    | %s |\n", plaintext)
	fmt.Fprintf(b, "| Status     | %s |\n", project.Status)
	fmt.Fprintf(b, "| Setup URL  | %s |\n\n", setupURL)
	fmt.Fprintf(b, "> The API key above is scoped to **ingest** only (event tracking). It is deterministic and will be identical on every visit. No plaintext is stored server-side.\n\n---\n\n")

	fmt.Fprintf(b, "## What FunnelBarn Tracks\n\n")
	fmt.Fprintf(b, "- **Page views** — call `fb.page()` on route changes\n")
	fmt.Fprintf(b, "- **Custom events** — call `fb.track(\"event-name\", { key: \"value\" })` for any user action\n")
	fmt.Fprintf(b, "- **Funnel steps** — define funnels in the admin UI after approval; FunnelBarn will analyse conversion rates and drop-off\n")
	fmt.Fprintf(b, "- **Feature flags** — evaluate release/experiment flags at runtime via `POST /api/v1/evaluate` (see below)\n\n---\n\n")

	// HTTP header names are case-insensitive on the wire. The display form
	// here is title-case for readability; the lowercase wire constant
	// auth.HeaderAPIKey matches it under EqualFold (verified in test).
	const keyHeader = "X-FunnelBarn-Api-Key"
	const projHeader = "X-FunnelBarn-Project"

	fmt.Fprintf(b, "## Required headers\n\n")
	fmt.Fprintf(b, "Every request to `/api/v1/events` must include:\n\n")
	fmt.Fprintf(b, "| Header | Value |\n")
	fmt.Fprintf(b, "|--------|-------|\n")
	fmt.Fprintf(b, "| `%s` | `%s` |\n", keyHeader, plaintext)
	fmt.Fprintf(b, "| `%s` | `%s` |\n\n", projHeader, slug)
	fmt.Fprintf(b, "> **Do not use** `x-api-key`, `Authorization`, or `X-Api-Key` — the server returns 401.\n")
	fmt.Fprintf(b, "> The auth header must be `%s` (HTTP header names are case-insensitive).\n\n", keyHeader)
	fmt.Fprintf(b, "> Project routing reads `%s` only — do not put the slug in the request body.\n\n---\n\n", projHeader)

	fmt.Fprintf(b, "## Event body schema\n\n")
	fmt.Fprintf(b, "| Field | Type | Required | Notes |\n")
	fmt.Fprintf(b, "|-------|------|----------|-------|\n")
	fmt.Fprintf(b, "| `name` | string | **yes** | Event name — e.g. `page_view`, `cta_clicked`. Field is `name`, NOT `event`, `type`, or `event_name` |\n")
	fmt.Fprintf(b, "| `session_id` | string | no | Persist this client-side across page loads. Omitted → server derives one from IP+UA (page-scoped only — no proper session stitching). |\n")
	fmt.Fprintf(b, "| `user_id` | string | no | Stable identifier for a logged-in user. **Hashed server-side** (SHA-256) — the raw value is never stored. Same user → same hash, so funnels and cohorts still work. |\n")
	fmt.Fprintf(b, "| `timestamp` | ISO 8601 string | no | Defaults to server receive time. Send your own for back-dated / batched events. |\n")
	fmt.Fprintf(b, "| `url` | string | no | Page URL. **UTM params in the query string are auto-extracted** — you don't need to also pass them in `utm_*` fields. |\n")
	fmt.Fprintf(b, "| `referrer` | string | no | Referring URL |\n")
	fmt.Fprintf(b, "| `user_agent` | string | no | Overrides the `User-Agent` request header. Useful for server-side ingest where the UA header reflects your backend, not the end user's browser. |\n")
	fmt.Fprintf(b, "| `properties` | object | no | Arbitrary key/value pairs — strings stay strings, numbers stay numbers. The main extension point for custom analytics. |\n")
	fmt.Fprintf(b, "| `utm_source` / `utm_medium` / `utm_campaign` / `utm_term` / `utm_content` | string | no | Explicit override of values auto-extracted from `url`. Send these only when the URL doesn't already carry them. |\n\n")
	fmt.Fprintf(b, "> Fields not listed above (`event`, `environment`, `type`, etc.) are silently discarded.\n\n---\n\n")

	fmt.Fprintf(b, "## JavaScript / TypeScript (recommended)\n\n")
	fmt.Fprintf(b, "The fastest path is the **CDN script tag** — auto-tracks page views on init, no install step:\n\n")
	fmt.Fprintf(b, "```html\n")
	fmt.Fprintf(b, "<script src=\"%s\"\n", sdkURL)
	fmt.Fprintf(b, "  data-api-key=\"%s\"\n", plaintext)
	fmt.Fprintf(b, "  data-project-name=\"%s\"\n", slug)
	fmt.Fprintf(b, "  defer></script>\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "For module-based projects, the SDK source lives at `sdks/js/` in the funnelbarn repo. **It is not yet published to npm** — vendor the built `dist/esm/index.js` or import directly from a git URL until publishing is wired in:\n\n")
	fmt.Fprintf(b, "```typescript\n")
	fmt.Fprintf(b, "// after vendoring sdks/js/dist/esm/index.js into your project:\n")
	fmt.Fprintf(b, "import { FunnelBarnClient } from './vendor/funnelbarn'\n\n")
	fmt.Fprintf(b, "const fb = new FunnelBarnClient({\n")
	fmt.Fprintf(b, "  endpoint:    '%s',\n", endpoint)
	fmt.Fprintf(b, "  projectName: '%s',\n", slug)
	fmt.Fprintf(b, "  apiKey:      '%s',\n", plaintext)
	fmt.Fprintf(b, "})\n\n")
	fmt.Fprintf(b, "// Track a page view (call on every route change)\n")
	fmt.Fprintf(b, "fb.page()\n\n")
	fmt.Fprintf(b, "// Track a custom event\n")
	fmt.Fprintf(b, "fb.track('signup-button-clicked', { plan: 'pro' })\n\n")
	fmt.Fprintf(b, "// Track a funnel step\n")
	fmt.Fprintf(b, "fb.track('checkout-started', { cart_value: 49.99 })\n")
	fmt.Fprintf(b, "fb.track('checkout-completed', { cart_value: 49.99 })\n")
	fmt.Fprintf(b, "```\n\n---\n\n")

	fmt.Fprintf(b, "## Python\n\n")
	fmt.Fprintf(b, "```python\n")
	fmt.Fprintf(b, "import httpx, time, uuid\n\n")
	fmt.Fprintf(b, "ENDPOINT = '%s'\n", endpoint)
	fmt.Fprintf(b, "API_KEY  = '%s'\n", plaintext)
	fmt.Fprintf(b, "PROJECT  = '%s'\n\n", slug)
	fmt.Fprintf(b, "def track(event_name: str, properties: dict = None):\n")
	fmt.Fprintf(b, "    httpx.post(ENDPOINT, json={\n")
	fmt.Fprintf(b, "        \"name\":        event_name,\n")
	fmt.Fprintf(b, "        \"session_id\":  str(uuid.uuid4()),  # persist per-user session\n")
	fmt.Fprintf(b, "        \"timestamp\":   time.strftime(\"%%Y-%%m-%%dT%%H:%%M:%%SZ\", time.gmtime()),\n")
	fmt.Fprintf(b, "        \"properties\":  properties or {},\n")
	fmt.Fprintf(b, "    }, headers={\n")
	fmt.Fprintf(b, "        \"%s\": API_KEY,\n", keyHeader)
	fmt.Fprintf(b, "        \"%s\": PROJECT,\n", projHeader)
	fmt.Fprintf(b, "    })\n\n")
	fmt.Fprintf(b, "track(\"page-view\", {\"url\": \"/home\"})\n")
	fmt.Fprintf(b, "track(\"signup-completed\")\n")
	fmt.Fprintf(b, "```\n\n---\n\n")

	fmt.Fprintf(b, "## Go\n\n")
	fmt.Fprintf(b, "```go\n")
	fmt.Fprintf(b, "package main\n\n")
	fmt.Fprintf(b, "import (\n")
	fmt.Fprintf(b, "    \"bytes\"\n")
	fmt.Fprintf(b, "    \"encoding/json\"\n")
	fmt.Fprintf(b, "    \"net/http\"\n")
	fmt.Fprintf(b, "    \"time\"\n")
	fmt.Fprintf(b, ")\n\n")
	fmt.Fprintf(b, "const (\n")
	fmt.Fprintf(b, "    endpoint = \"%s\"\n", endpoint)
	fmt.Fprintf(b, "    apiKey   = \"%s\"\n", plaintext)
	fmt.Fprintf(b, "    project  = \"%s\"\n", slug)
	fmt.Fprintf(b, ")\n\n")
	fmt.Fprintf(b, "type Event struct {\n")
	fmt.Fprintf(b, "    Name       string         `json:\"name\"`\n")
	fmt.Fprintf(b, "    SessionID  string         `json:\"session_id\"`\n")
	fmt.Fprintf(b, "    Timestamp  time.Time      `json:\"timestamp\"`\n")
	fmt.Fprintf(b, "    Properties map[string]any `json:\"properties,omitempty\"`\n")
	fmt.Fprintf(b, "}\n\n")
	fmt.Fprintf(b, "func track(name string, props map[string]any) error {\n")
	fmt.Fprintf(b, "    body, _ := json.Marshal(Event{\n")
	fmt.Fprintf(b, "        Name:       name,\n")
	fmt.Fprintf(b, "        SessionID:  \"user-session-id\",\n")
	fmt.Fprintf(b, "        Timestamp:  time.Now().UTC(),\n")
	fmt.Fprintf(b, "        Properties: props,\n")
	fmt.Fprintf(b, "    })\n")
	fmt.Fprintf(b, "    req, _ := http.NewRequest(\"POST\", endpoint, bytes.NewReader(body))\n")
	fmt.Fprintf(b, "    req.Header.Set(\"Content-Type\", \"application/json\")\n")
	fmt.Fprintf(b, "    req.Header.Set(\"%s\", apiKey)\n", keyHeader)
	fmt.Fprintf(b, "    req.Header.Set(\"%s\", project)\n", projHeader)
	fmt.Fprintf(b, "    _, err := http.DefaultClient.Do(req)\n")
	fmt.Fprintf(b, "    return err\n")
	fmt.Fprintf(b, "}\n")
	fmt.Fprintf(b, "```\n\n---\n\n")

	fmt.Fprintf(b, "## HTTP API (curl)\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "curl -s -X POST '%s' \\\n", endpoint)
	fmt.Fprintf(b, "  -H 'Content-Type: application/json' \\\n")
	fmt.Fprintf(b, "  -H '%s: %s' \\\n", keyHeader, plaintext)
	fmt.Fprintf(b, "  -H '%s: %s' \\\n", projHeader, slug)
	fmt.Fprintf(b, "  -d '{\n")
	fmt.Fprintf(b, "    \"name\":       \"page-view\",\n")
	fmt.Fprintf(b, "    \"session_id\": \"abc123\",\n")
	fmt.Fprintf(b, "    \"timestamp\":  \"2024-01-01T00:00:00Z\",\n")
	fmt.Fprintf(b, "    \"properties\": {\"url\": \"/home\"}\n")
	fmt.Fprintf(b, "  }'\n")
	fmt.Fprintf(b, "```\n\n---\n\n")

	fmt.Fprintf(b, "## Feature Flags\n\n")
	fmt.Fprintf(b, "FunnelBarn doubles as a feature-flag service. Create flags in the admin\n")
	fmt.Fprintf(b, "UI (release / experiment, with optional targeting rules and gradual\n")
	fmt.Fprintf(b, "rollouts), then evaluate them at runtime from your app.\n\n")
	fmt.Fprintf(b, "### `POST /api/v1/evaluate`\n\n")
	fmt.Fprintf(b, "| Header | Value |\n")
	fmt.Fprintf(b, "|--------|-------|\n")
	fmt.Fprintf(b, "| `%s` | `%s` |\n", keyHeader, plaintext)
	fmt.Fprintf(b, "| `%s` | `%s` |\n\n", projHeader, slug)
	fmt.Fprintf(b, "Request body:\n\n")
	fmt.Fprintf(b, "| Field | Type | Required | Notes |\n")
	fmt.Fprintf(b, "|-------|------|----------|-------|\n")
	fmt.Fprintf(b, "| `flag_key` | string | **yes** | The flag's key as configured in the dashboard |\n")
	fmt.Fprintf(b, "| `default_value` | any | no | Returned when the flag is missing or eval errors |\n")
	fmt.Fprintf(b, "| `context` | object | no | User/session attributes for targeting + bucketing |\n\n")
	fmt.Fprintf(b, "> **Important**: the `context` object is what targeting rules and the\n")
	fmt.Fprintf(b, "> split-based bucketing read. Use the key `targeting_key` (or `session_id`)\n")
	fmt.Fprintf(b, "> for the value that should stay sticky per user. `user_id` is **not**\n")
	fmt.Fprintf(b, "> the bucket key — if you only send `user_id`, every call buckets the same\n")
	fmt.Fprintf(b, "> way regardless of who the user is.\n\n")
	fmt.Fprintf(b, "Response:\n\n")
	fmt.Fprintf(b, "```json\n")
	fmt.Fprintf(b, "{\n")
	fmt.Fprintf(b, "  \"flag_key\":  \"checkout_redesign\",\n")
	fmt.Fprintf(b, "  \"variant\":   \"on\",\n")
	fmt.Fprintf(b, "  \"value\":     true,\n")
	fmt.Fprintf(b, "  \"reason\":    \"TARGETING_MATCH\",\n")
	fmt.Fprintf(b, "  \"error_code\": \"\",\n")
	fmt.Fprintf(b, "  \"flag_metadata\": {\"evaluated_rule_name\": \"beta-users\"}\n")
	fmt.Fprintf(b, "}\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "`reason` is one of:\n\n")
	fmt.Fprintf(b, "- `SPLIT` — no targeting rule matched, bucketed by `targeting_key` against the split percentages.\n")
	fmt.Fprintf(b, "- `TARGETING_MATCH` — a rule matched. `flag_metadata.evaluated_rule_name` tells you which one.\n")
	fmt.Fprintf(b, "- `DISABLED` — the flag is paused. You get the default variant; targeting rules and split are bypassed.\n")
	fmt.Fprintf(b, "- `ERROR` — something went wrong; check `error_code` (e.g. `FLAG_NOT_FOUND`). `value` echoes back the `default_value` you supplied.\n\n")
	fmt.Fprintf(b, "Errors are returned as HTTP 200 with `reason: ERROR` and the supplied\n")
	fmt.Fprintf(b, "`default_value` echoed back — never throw on missing flags, fall back.\n\n")
	fmt.Fprintf(b, "**Bucketing is deterministic.** A given `(flag_key, targeting_key)` pair\n")
	fmt.Fprintf(b, "always resolves to the same variant — across calls, across processes,\n")
	fmt.Fprintf(b, "across deploys. That's what makes A/B tests sticky and rollouts\n")
	fmt.Fprintf(b, "consistent. Hash inputs: `sha256(targeting_key + \":\" + flag_key)`,\n")
	fmt.Fprintf(b, "modulo 10000, walked against split percentages in sorted variant order.\n\n")
	fmt.Fprintf(b, "### curl\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "curl -s -X POST '%s/api/v1/evaluate' \\\n", publicURL)
	fmt.Fprintf(b, "  -H 'Content-Type: application/json' \\\n")
	fmt.Fprintf(b, "  -H '%s: %s' \\\n", keyHeader, plaintext)
	fmt.Fprintf(b, "  -H '%s: %s' \\\n", projHeader, slug)
	fmt.Fprintf(b, "  -d '{\n")
	fmt.Fprintf(b, "    \"flag_key\":      \"checkout_redesign\",\n")
	fmt.Fprintf(b, "    \"default_value\": false,\n")
	fmt.Fprintf(b, "    \"context\":       {\"targeting_key\": \"user-42\", \"plan\": \"pro\"}\n")
	fmt.Fprintf(b, "  }'\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "### Python\n\n")
	fmt.Fprintf(b, "```python\n")
	fmt.Fprintf(b, "def evaluate(flag_key: str, default, context: dict):\n")
	fmt.Fprintf(b, "    r = httpx.post(f\"{ENDPOINT.rsplit('/', 2)[0]}/api/v1/evaluate\",\n")
	fmt.Fprintf(b, "        json={\"flag_key\": flag_key, \"default_value\": default, \"context\": context},\n")
	fmt.Fprintf(b, "        headers={\"%s\": API_KEY, \"%s\": PROJECT})\n", keyHeader, projHeader)
	fmt.Fprintf(b, "    body = r.json()\n")
	fmt.Fprintf(b, "    return body.get(\"value\", default)\n\n")
	fmt.Fprintf(b, "if evaluate(\"checkout_redesign\", False, {\"targeting_key\": user_id}):\n")
	fmt.Fprintf(b, "    show_new_checkout()\n")
	fmt.Fprintf(b, "```\n\n---\n\n")

	fmt.Fprintf(b, "## Recommended Funnel Definitions\n\n")
	fmt.Fprintf(b, "Create these in the admin UI once the project is approved:\n\n")
	fmt.Fprintf(b, "1. **Acquisition → Activation**\n")
	fmt.Fprintf(b, "   - Steps: `page-view` → `signup-started` → `signup-completed`\n\n")
	fmt.Fprintf(b, "2. **Checkout**\n")
	fmt.Fprintf(b, "   - Steps: `cart-viewed` → `checkout-started` → `payment-entered` → `checkout-completed`\n\n")
	fmt.Fprintf(b, "3. **Engagement**\n")
	fmt.Fprintf(b, "   - Steps: `page-view` → `feature-used` → `return-visit`\n\n---\n\n")

	fmt.Fprintf(b, "## Next Steps\n\n")
	fmt.Fprintf(b, "1. Instrument your app using the SDK examples above\n")
	fmt.Fprintf(b, "2. Ask your FunnelBarn admin to approve this project at: **%s**\n", setupURL)
	fmt.Fprintf(b, "3. Once approved, visit the dashboard to see live data, define funnels, and create feature flags\n")
	fmt.Fprintf(b, "4. (Optional) Wire flag evaluation into your app — see the Feature Flags section above\n")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}
