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
	fmt.Fprintf(b, "> The API key above is scoped to **ingest** only (event tracking). It is deterministic and will be identical on every visit. No plaintext is stored server-side.\n\n")
	fmt.Fprintf(b, "> **One key, all environments.** Use the same API key and project across development, staging, and production. Tag each event with `\"environment\": \"dev\"` (or `\"staging\"`, `\"prod\"`, etc.) — aliases are normalised server-side. The dashboard lets you filter to a single environment so your dev traffic never pollutes production numbers.\n\n---\n\n")

	fmt.Fprintf(b, "## What FunnelBarn Tracks\n\n")
	fmt.Fprintf(b, "- **Page views** — call `fb.page()` on route changes\n")
	fmt.Fprintf(b, "- **Custom events** — call `fb.track(\"event-name\", { key: \"value\" })` for any user action\n")
	fmt.Fprintf(b, "- **Funnel steps** — define funnels in the admin UI after approval; FunnelBarn will analyse conversion rates and drop-off\n")
	fmt.Fprintf(b, "- **Session recordings** — enable `recording: true` in the JS SDK; rrweb captures full DOM mutations and replays them in the dashboard\n")
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
	fmt.Fprintf(b, "| `utm_source` / `utm_medium` / `utm_campaign` / `utm_term` / `utm_content` | string | no | Explicit override of values auto-extracted from `url`. Send these only when the URL doesn't already carry them. |\n")
	fmt.Fprintf(b, "| `environment` | string | no | Deployment environment. Aliases are normalized: `prod`/`prd`/`live` → `production`, `stg`/`stage`/`acc`/`acceptance` → `staging`, `testing`/`tst`/`qa`/`uat`/`pr` → `test`, `dev`/`local`/`develop` → `development`. Omit for production (default). |\n\n")
	fmt.Fprintf(b, "> Fields not listed above (`event`, `type`, etc.) are silently discarded.\n\n---\n\n")

	fmt.Fprintf(b, "## JavaScript / TypeScript (recommended)\n\n")
	fmt.Fprintf(b, "Two paths — pick whichever fits.\n\n")
	fmt.Fprintf(b, "**Drop-in CDN script** — auto-tracks page views on init, no install step:\n\n")
	fmt.Fprintf(b, "```html\n")
	fmt.Fprintf(b, "<script src=\"%s\"\n", sdkURL)
	fmt.Fprintf(b, "  data-api-key=\"%s\"\n", plaintext)
	fmt.Fprintf(b, "  data-project-name=\"%s\"\n", slug)
	fmt.Fprintf(b, "  data-recording=\"true\"\n")
	fmt.Fprintf(b, "  defer></script>\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "> Remove `data-recording=\"true\"` to disable session recording. The SDK fetches server-side config regardless, so an admin can also toggle recording without a code deploy.\n\n")
	fmt.Fprintf(b, "**Module-based projects**:\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "npm install @funnelbarn/js\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "```typescript\n")
	fmt.Fprintf(b, "import { FunnelBarnClient } from '@funnelbarn/js'\n\n")
	fmt.Fprintf(b, "const fb = new FunnelBarnClient({\n")
	fmt.Fprintf(b, "  endpoint:    '%s',\n", endpoint)
	fmt.Fprintf(b, "  projectName: '%s',\n", slug)
	fmt.Fprintf(b, "  apiKey:      '%s',\n", plaintext)
	fmt.Fprintf(b, "  // Tag every event with the deployment environment.\n")
	fmt.Fprintf(b, "  // Use the same key+project in dev/staging/prod — filter by env in the dashboard.\n")
	fmt.Fprintf(b, "  environment: process.env.NODE_ENV === 'production' ? 'production' : 'development',\n\n")
	fmt.Fprintf(b, "  // Session recording (browser only). Server config can also enable/disable this.\n")
	fmt.Fprintf(b, "  recording:        true,\n")
	fmt.Fprintf(b, "  recordingChunkMs: 10_000,           // flush every 10 s (default)\n")
	fmt.Fprintf(b, "  recordInclude:    ['/checkout/**'],  // always capture these paths\n")
	fmt.Fprintf(b, "  recordExclude:    ['/account/**'],   // never capture these paths\n")
	fmt.Fprintf(b, "})\n\n")
	fmt.Fprintf(b, "// Track a page view (call on every route change)\n")
	fmt.Fprintf(b, "fb.page()\n\n")
	fmt.Fprintf(b, "// Track a custom event\n")
	fmt.Fprintf(b, "fb.track('signup-button-clicked', { plan: 'pro' })\n\n")
	fmt.Fprintf(b, "// Track a funnel step\n")
	fmt.Fprintf(b, "fb.track('checkout-started', { cart_value: 49.99 })\n")
	fmt.Fprintf(b, "fb.track('checkout-completed', { cart_value: 49.99 })\n")
	fmt.Fprintf(b, "```\n\n---\n\n")

	fmt.Fprintf(b, "## Session Recording\n\n")
	fmt.Fprintf(b, "Session recording captures full DOM snapshots and mutations using [rrweb](https://github.com/rrweb-io/rrweb).\n")
	fmt.Fprintf(b, "Recordings are streamed in compressed chunks and can be replayed frame-by-frame in the dashboard.\n")
	fmt.Fprintf(b, "Recording is **browser-only** — server-side SDKs do not generate recording data.\n\n")

	fmt.Fprintf(b, "### Enabling recording\n\n")
	fmt.Fprintf(b, "Two independent switches control whether a session is recorded:\n\n")
	fmt.Fprintf(b, "1. **Client-side**: `recording: true` in the JS SDK (or `data-recording=\"true\"` on the script tag)\n")
	fmt.Fprintf(b, "2. **Server-side**: the admin dashboard can override the client setting — set the project's recording toggle to on/off, or adjust the sample rate\n\n")
	fmt.Fprintf(b, "On every SDK init the browser fetches `GET /api/v1/recording-config` (no auth required for the call, the API key is used to scope the response to your project).\n")
	fmt.Fprintf(b, "The server response:\n\n")
	fmt.Fprintf(b, "```json\n")
	fmt.Fprintf(b, "{ \"enabled\": true, \"sample_rate\": 1.0, \"rules\": [] }\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "- If `enabled` is `false` the SDK stops any active recording, regardless of the client-side setting.\n")
	fmt.Fprintf(b, "- `sample_rate` is a float in `[0, 1]`. The SDK rolls `Math.random()` once per page load and skips recording if the roll exceeds the rate. At `1.0` every session is captured.\n")
	fmt.Fprintf(b, "- `rules` is an ordered list of URL-path glob rules checked for each chunk flush (first match wins, default is capture).\n\n")

	fmt.Fprintf(b, "### URL-path rules\n\n")
	fmt.Fprintf(b, "Rules let you restrict recording to specific paths or exclude sensitive pages without a code deploy.\n\n")
	fmt.Fprintf(b, "Priority order (highest first):\n\n")
	fmt.Fprintf(b, "1. SDK `recordExclude[]` — always suppresses recording for matching paths\n")
	fmt.Fprintf(b, "2. SDK `recordInclude[]` — always captures matching paths\n")
	fmt.Fprintf(b, "3. Server rules (first match wins) — set in the admin dashboard\n")
	fmt.Fprintf(b, "4. Default: capture\n\n")
	fmt.Fprintf(b, "Pattern syntax: `*` matches any single path segment, `**` matches any number of segments.\n\n")
	fmt.Fprintf(b, "```typescript\n")
	fmt.Fprintf(b, "// Capture checkout flows, never capture account or billing pages:\n")
	fmt.Fprintf(b, "recordInclude: ['/checkout/**', '/onboarding/**'],\n")
	fmt.Fprintf(b, "recordExclude: ['/account/**', '/billing/**'],\n")
	fmt.Fprintf(b, "```\n\n")

	fmt.Fprintf(b, "### Privacy controls\n\n")
	fmt.Fprintf(b, "- **Password fields** are automatically masked as `***` — no plaintext credentials captured.\n")
	fmt.Fprintf(b, "- Add the CSS class `fb-block` to any element you want fully redacted from recordings:\n\n")
	fmt.Fprintf(b, "```html\n")
	fmt.Fprintf(b, "<div class=\"fb-block\">This content is never captured</div>\n")
	fmt.Fprintf(b, "```\n\n")

	fmt.Fprintf(b, "### Chunk ingest endpoint (advanced)\n\n")
	fmt.Fprintf(b, "The JS SDK posts chunks automatically. If you are building a custom recorder or replaying your own rrweb stream, post directly:\n\n")
	fmt.Fprintf(b, "**`POST /api/v1/recordings/chunk`**\n\n")
	fmt.Fprintf(b, "| Header | Value |\n")
	fmt.Fprintf(b, "|--------|-------|\n")
	fmt.Fprintf(b, "| `%s` | `%s` |\n\n", keyHeader, plaintext)
	fmt.Fprintf(b, "Request body:\n\n")
	fmt.Fprintf(b, "| Field | Type | Notes |\n")
	fmt.Fprintf(b, "|-------|------|-------|\n")
	fmt.Fprintf(b, "| `recording_id` | string | Stable 32-hex-char ID for the recording. Generate once per page load; a new recording starts on each navigation. |\n")
	fmt.Fprintf(b, "| `session_id` | string | Same session ID used for event tracking. Ties the recording to the user's session. |\n")
	fmt.Fprintf(b, "| `chunk_index` | int | Zero-based, increments per flush within the same `recording_id`. The server uses this to track the first chunk index and total chunk count. |\n")
	fmt.Fprintf(b, "| `events` | array | Array of `rrweb.eventWithTime` objects captured since the last flush. Must not be empty. |\n")
	fmt.Fprintf(b, "| `started_at` | ISO 8601 string | Timestamp of when this recording started (i.e. when rrweb `record()` was called). |\n")
	fmt.Fprintf(b, "| `duration_ms` | int | Elapsed milliseconds since `started_at` — used to compute `ended_at` and total session length. |\n\n")
	fmt.Fprintf(b, "Response: `202 {\"accepted\": true}`\n\n")
	fmt.Fprintf(b, "> **Note**: `page_url` is not in the request body. The server reads the `Referer` header and user-agent for metadata. Chunks are stored as gzip-compressed JSON in object storage; metadata (session, device type, duration) is stored in the database.\n\n---\n\n")

	fmt.Fprintf(b, "## Python\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "pip install funnelbarn\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "```python\n")
	fmt.Fprintf(b, "import os\n")
	fmt.Fprintf(b, "from funnelbarn import FunnelBarnClient\n\n")
	fmt.Fprintf(b, "analytics = FunnelBarnClient(\n")
	fmt.Fprintf(b, "    api_key=\"%s\",\n", plaintext)
	fmt.Fprintf(b, "    endpoint=\"%s\",\n", publicURL)
	fmt.Fprintf(b, "    project_name=\"%s\",\n", slug)
	fmt.Fprintf(b, "    # Same key across envs — tag events so the dashboard can filter them.\n")
	fmt.Fprintf(b, "    environment=os.getenv(\"APP_ENV\", \"production\"),  # e.g. \"staging\", \"dev\"\n")
	fmt.Fprintf(b, ")\n\n")
	fmt.Fprintf(b, "analytics.page(\"/pricing\")\n")
	fmt.Fprintf(b, "analytics.track(\"signup\", {\"plan\": \"pro\"})\n")
	fmt.Fprintf(b, "analytics.identify(\"user_123\")  # hashed server-side\n\n")
	fmt.Fprintf(b, "# Optional: flush on shutdown so queued events aren't lost.\n")
	fmt.Fprintf(b, "analytics.shutdown()\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "The Python SDK queues events on a background worker and batches them. If you need raw control instead, hit `POST /api/v1/events` directly with the headers above — see the curl example below.\n\n---\n\n")

	fmt.Fprintf(b, "## Go\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "go get github.com/webwiebe/funnelbarn/sdks/go\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "```go\n")
	fmt.Fprintf(b, "package main\n\n")
	fmt.Fprintf(b, "import (\n")
	fmt.Fprintf(b, "    \"os\"\n")
	fmt.Fprintf(b, "    \"time\"\n")
	fmt.Fprintf(b, "    funnelbarn \"github.com/webwiebe/funnelbarn/sdks/go\"\n")
	fmt.Fprintf(b, ")\n\n")
	fmt.Fprintf(b, "func main() {\n")
	fmt.Fprintf(b, "    funnelbarn.Init(funnelbarn.Options{\n")
	fmt.Fprintf(b, "        APIKey:      \"%s\",\n", plaintext)
	fmt.Fprintf(b, "        Endpoint:    \"%s\",\n", publicURL)
	fmt.Fprintf(b, "        ProjectName: \"%s\",\n", slug)
	fmt.Fprintf(b, "        // Same key across envs — tag events so the dashboard can filter them.\n")
	fmt.Fprintf(b, "        Environment: os.Getenv(\"APP_ENV\"), // e.g. \"staging\", \"dev\", \"production\"\n")
	fmt.Fprintf(b, "    })\n")
	fmt.Fprintf(b, "    defer funnelbarn.Shutdown(5 * time.Second)\n\n")
	fmt.Fprintf(b, "    funnelbarn.Track(\"signup\", map[string]any{\"plan\": \"pro\"})\n")
	fmt.Fprintf(b, "    funnelbarn.Page(\"/pricing\", \"\")\n")
	fmt.Fprintf(b, "}\n")
	fmt.Fprintf(b, "```\n\n---\n\n")

	fmt.Fprintf(b, "## HTTP API (curl)\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "curl -s -X POST '%s' \\\n", endpoint)
	fmt.Fprintf(b, "  -H 'Content-Type: application/json' \\\n")
	fmt.Fprintf(b, "  -H '%s: %s' \\\n", keyHeader, plaintext)
	fmt.Fprintf(b, "  -H '%s: %s' \\\n", projHeader, slug)
	fmt.Fprintf(b, "  -d '{\n")
	fmt.Fprintf(b, "    \"name\":        \"page-view\",\n")
	fmt.Fprintf(b, "    \"session_id\":  \"abc123\",\n")
	fmt.Fprintf(b, "    \"timestamp\":   \"2024-01-01T00:00:00Z\",\n")
	fmt.Fprintf(b, "    \"environment\": \"staging\",\n")
	fmt.Fprintf(b, "    \"properties\":  {\"url\": \"/home\"}\n")
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

	fmt.Fprintf(b, "### Evaluation context — send more, target better\n\n")
	fmt.Fprintf(b, "The `context` object is a plain key/value map. There is no fixed schema —\n")
	fmt.Fprintf(b, "pass whatever is meaningful at the call site. The richer the context, the\n")
	fmt.Fprintf(b, "more specific the targeting rules you can write in the dashboard.\n\n")
	fmt.Fprintf(b, "Common keys to include:\n\n")
	fmt.Fprintf(b, "| Key | Example value | Why it helps |\n")
	fmt.Fprintf(b, "|-----|---------------|-------------|\n")
	fmt.Fprintf(b, "| `targeting_key` | `\"user-42\"` | **Required for sticky bucketing.** Omitting it makes every call land in the same bucket. |\n")
	fmt.Fprintf(b, "| `plan` | `\"pro\"` | Gate features by subscription tier |\n")
	fmt.Fprintf(b, "| `role` | `\"admin\"` | Internal vs. external rollouts |\n")
	fmt.Fprintf(b, "| `country` | `\"NL\"` | Regional rollouts and compliance |\n")
	fmt.Fprintf(b, "| `organization_id` | `\"org-acme\"` | Tenant-level targeting |\n")
	fmt.Fprintf(b, "| `version` | `\"2.4.1\"` | Client version gating |\n")
	fmt.Fprintf(b, "| `beta` | `\"true\"` | Opt-in cohorts |\n\n")
	fmt.Fprintf(b, "> **Consistency is what matters.** A key you send on 60%% of calls can\n")
	fmt.Fprintf(b, "> only be matched by a targeting rule for that 60%%. Send the keys you\n")
	fmt.Fprintf(b, "> care about on every call — even as empty string if the value is unknown.\n\n")
	fmt.Fprintf(b, "> **Feedback loop.** FunnelBarn records which context keys appear in\n")
	fmt.Fprintf(b, "> evaluations. In the targeting-rule editor, the context key field\n")
	fmt.Fprintf(b, "> autocompletes from your real traffic and shows each key's inclusion\n")
	fmt.Fprintf(b, "> rate (e.g. `plan — 94%%`). The more consistently you send a key, the\n")
	fmt.Fprintf(b, "> more prominently it surfaces. This makes the setup doc and the rule\n")
	fmt.Fprintf(b, "> editor self-reinforcing: what you document here, you'll see suggested\n")
	fmt.Fprintf(b, "> there.\n\n")

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
	fmt.Fprintf(b, "4. (Optional) Enable session recording — add `recording: true` to the SDK and ask your admin to turn on recording for this project in the dashboard\n")
	fmt.Fprintf(b, "5. (Optional) Wire flag evaluation into your app — see the Feature Flags section above\n")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}
