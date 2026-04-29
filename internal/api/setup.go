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
	project, err := s.store.EnsureProjectPending(ctx, slug, slug)
	if err != nil {
		slog.Error("setup: ensure project", "slug", slug, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Derive deterministic ingest key.
	plaintext, keySHA256 := setupKey(s.sessionSecret, slug)

	// Upsert the setup key so it is valid for ingest.
	if err := s.store.EnsureSetupAPIKey(ctx, project.ID, keySHA256); err != nil {
		slog.Error("setup: ensure api key", "project_id", project.ID, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	publicURL := s.publicURL
	if publicURL == "" {
		publicURL = "https://funnelbarn.wiebe.xyz"
	}

	endpoint := publicURL + "/api/v1/events"
	sdkURL := publicURL + "/packages/js/latest"
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
	fmt.Fprintf(b, "- **Funnel steps** — define funnels in the admin UI after approval; FunnelBarn will analyse conversion rates and drop-off\n\n---\n\n")

	fmt.Fprintf(b, "## JavaScript / TypeScript (recommended)\n\n")
	fmt.Fprintf(b, "Install the SDK:\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "npm install @funnelbarn/js\n")
	fmt.Fprintf(b, "# or load from CDN (no install required):\n")
	fmt.Fprintf(b, "# <script src=\"%s\"></script>\n", sdkURL)
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "Usage:\n\n")
	fmt.Fprintf(b, "```typescript\n")
	fmt.Fprintf(b, "import { FunnelBarn } from '@funnelbarn/js'\n\n")
	fmt.Fprintf(b, "const fb = new FunnelBarn({\n")
	fmt.Fprintf(b, "  endpoint: '%s',\n", endpoint)
	fmt.Fprintf(b, "  project:  '%s',\n", slug)
	fmt.Fprintf(b, "  apiKey:   '%s',\n", plaintext)
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
	fmt.Fprintf(b, "        \"project\":     PROJECT,\n")
	fmt.Fprintf(b, "        \"name\":        event_name,\n")
	fmt.Fprintf(b, "        \"session_id\":  str(uuid.uuid4()),  # persist per-user session\n")
	fmt.Fprintf(b, "        \"occurred_at\": time.strftime(\"%%Y-%%m-%%dT%%H:%%M:%%SZ\", time.gmtime()),\n")
	fmt.Fprintf(b, "        \"properties\":  properties or {},\n")
	fmt.Fprintf(b, "    }, headers={\"X-API-Key\": API_KEY})\n\n")
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
	fmt.Fprintf(b, "    Project    string         `json:\"project\"`\n")
	fmt.Fprintf(b, "    Name       string         `json:\"name\"`\n")
	fmt.Fprintf(b, "    SessionID  string         `json:\"session_id\"`\n")
	fmt.Fprintf(b, "    OccurredAt string         `json:\"occurred_at\"`\n")
	fmt.Fprintf(b, "    Properties map[string]any `json:\"properties,omitempty\"`\n")
	fmt.Fprintf(b, "}\n\n")
	fmt.Fprintf(b, "func track(name string, props map[string]any) error {\n")
	fmt.Fprintf(b, "    body, _ := json.Marshal(Event{\n")
	fmt.Fprintf(b, "        Project:    project,\n")
	fmt.Fprintf(b, "        Name:       name,\n")
	fmt.Fprintf(b, "        SessionID:  \"user-session-id\",\n")
	fmt.Fprintf(b, "        OccurredAt: time.Now().UTC().Format(time.RFC3339),\n")
	fmt.Fprintf(b, "        Properties: props,\n")
	fmt.Fprintf(b, "    })\n")
	fmt.Fprintf(b, "    req, _ := http.NewRequest(\"POST\", endpoint, bytes.NewReader(body))\n")
	fmt.Fprintf(b, "    req.Header.Set(\"Content-Type\", \"application/json\")\n")
	fmt.Fprintf(b, "    req.Header.Set(\"X-API-Key\", apiKey)\n")
	fmt.Fprintf(b, "    _, err := http.DefaultClient.Do(req)\n")
	fmt.Fprintf(b, "    return err\n")
	fmt.Fprintf(b, "}\n")
	fmt.Fprintf(b, "```\n\n---\n\n")

	fmt.Fprintf(b, "## HTTP API (curl)\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "curl -s -X POST '%s' \\\n", endpoint)
	fmt.Fprintf(b, "  -H 'Content-Type: application/json' \\\n")
	fmt.Fprintf(b, "  -H 'X-API-Key: %s' \\\n", plaintext)
	fmt.Fprintf(b, "  -d '{\n")
	fmt.Fprintf(b, "    \"project\":     \"%s\",\n", slug)
	fmt.Fprintf(b, "    \"name\":        \"page-view\",\n")
	fmt.Fprintf(b, "    \"session_id\":  \"abc123\",\n")
	fmt.Fprintf(b, "    \"occurred_at\": \"2024-01-01T00:00:00Z\",\n")
	fmt.Fprintf(b, "    \"properties\":  {\"url\": \"/home\"}\n")
	fmt.Fprintf(b, "  }'\n")
	fmt.Fprintf(b, "```\n\n---\n\n")

	fmt.Fprintf(b, "## Release Markers\n\n")
	fmt.Fprintf(b, "Tag a deploy so FunnelBarn can overlay it on time-series charts:\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "curl -s -X POST '%s/api/v1/releases' \\\n", publicURL)
	fmt.Fprintf(b, "  -H 'Content-Type: application/json' \\\n")
	fmt.Fprintf(b, "  -H 'X-API-Key: %s' \\\n", plaintext)
	fmt.Fprintf(b, "  -d '{\n")
	fmt.Fprintf(b, "    \"project\": \"%s\",\n", slug)
	fmt.Fprintf(b, "    \"version\": \"v1.2.3\",\n")
	fmt.Fprintf(b, "    \"message\": \"Deploy to production\"\n")
	fmt.Fprintf(b, "  }'\n")
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
	fmt.Fprintf(b, "3. Once approved, visit the dashboard to see live data and define funnels\n")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}
