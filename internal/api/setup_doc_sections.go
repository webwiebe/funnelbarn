package api

import (
	"fmt"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/mcp"
)

// writeFlagKindsSection documents the two things flags get used for, and the
// behaviour around them that the doc used to leave out entirely:
// auto-registration, non-boolean variant values, and the config-flag pattern.
func writeFlagKindsSection(b *strings.Builder, maxAutoFlags int) {
	fmt.Fprintf(b, "### Two kinds of flag\n\n")
	fmt.Fprintf(b, "A flag is either an **experiment** or a **config** value. Pick the one that\n")
	fmt.Fprintf(b, "matches how it is read — the kind decides the evaluation semantics, not just\n")
	fmt.Fprintf(b, "the label.\n\n")
	fmt.Fprintf(b, "| | `experiment` (default) | `config` |\n")
	fmt.Fprintf(b, "|---|---|---|\n")
	fmt.Fprintf(b, "| Read by | a user request | a server, often on a loop |\n")
	fmt.Fprintf(b, "| Resolution | bucketed by `targeting_key` | one value for everyone |\n")
	fmt.Fprintf(b, "| `reason` | `SPLIT` / `TARGETING_MATCH` | `STATIC` |\n")
	fmt.Fprintf(b, "| Evaluation rows | one per read | none |\n")
	fmt.Fprintf(b, "| Variant / conversion report | yes | not shown |\n")
	fmt.Fprintf(b, "| `cache_max_age_seconds` | `0` (don't cache) | the sanctioned polling interval |\n\n")
	fmt.Fprintf(b, "Use `config` for a rate limit, a batch size, a daily cap — anything with no\n")
	fmt.Fprintf(b, "user, no bucket and no conversion. A single value polled every 60s from three\n")
	fmt.Fprintf(b, "pods writes ~4.3k evaluation rows a day as an experiment, all of them junk,\n")
	fmt.Fprintf(b, "and they drown the flag's own analytics in machine reads.\n\n")
	fmt.Fprintf(b, "### Values are arbitrary JSON, not just booleans\n\n")
	fmt.Fprintf(b, "A variant's value can be a boolean, a number, a string, or a whole object.\n")
	fmt.Fprintf(b, "The dashboard infers `flag_type` from it (`boolean` / `number` / `string` /\n")
	fmt.Fprintf(b, "`json`), so an integer-valued flag — a daily cap, say — needs nothing\n")
	fmt.Fprintf(b, "special. Note that a numeric value arrives as a JSON number, which is a\n")
	fmt.Fprintf(b, "float in most languages: convert at the call site, or use a typed SDK\n")
	fmt.Fprintf(b, "helper that does it for you.\n\n")
	fmt.Fprintf(b, "### Auto-registration — you don't pre-create flags\n\n")
	fmt.Fprintf(b, "Evaluating a key that doesn't exist yet **creates it**, inert, carrying the\n")
	fmt.Fprintf(b, "`default_value` you sent (`status: inactive`, `origin: auto`, single variant\n")
	fmt.Fprintf(b, "named `default`). You get your own default back with `reason: DISABLED`, so\n")
	fmt.Fprintf(b, "nothing changes for the caller — but the flag now appears in the dashboard\n")
	fmt.Fprintf(b, "ready to configure, already holding a sensible value.\n\n")
	fmt.Fprintf(b, "Send `\"kind\": \"config\"` on that first call and it registers as a config\n")
	fmt.Fprintf(b, "flag; the kind is read only at registration, so an existing flag keeps\n")
	fmt.Fprintf(b, "whatever the dashboard says.\n\n")
	fmt.Fprintf(b, "Auto-registration is capped per project (`FUNNELBARN_FLAG_AUTOREGISTER_MAX`,\n")
	fmt.Fprintf(b, "currently **%d** on this instance). Past the cap, evaluation returns\n", maxAutoFlags)
	fmt.Fprintf(b, "`reason: ERROR` with `error_code: AUTO_REGISTER_LIMIT` and your\n")
	fmt.Fprintf(b, "`default_value` — the flag is simply not created. Stale auto flags that are\n")
	fmt.Fprintf(b, "never configured are pruned; evaluating one keeps it alive.\n\n")
	fmt.Fprintf(b, "### The config-flag pattern\n\n")
	fmt.Fprintf(b, "```json\n")
	fmt.Fprintf(b, "{\n")
	fmt.Fprintf(b, "  \"flag_key\":      \"cold_email_daily_cap\",\n")
	fmt.Fprintf(b, "  \"default_value\": 250,\n")
	fmt.Fprintf(b, "  \"kind\":          \"config\"\n")
	fmt.Fprintf(b, "}\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "First call registers `cold_email_daily_cap` holding `250`. Open the flag in\n")
	fmt.Fprintf(b, "the dashboard, change the number, activate it — the fleet picks the new value\n")
	fmt.Fprintf(b, "up within `cache_max_age_seconds`, with no deploy.\n\n")
	fmt.Fprintf(b, "> A config flag needs **one variant at 100%%**. If you give it several and\n")
	fmt.Fprintf(b, "> a split, the default variant still wins — a config value is the same for\n")
	fmt.Fprintf(b, "> everyone by definition — but the extra variants are just noise in the UI.\n\n")
}

// writeFlagAPISection documents the server-to-server path for reading and
// toggling a flag. Without it the only way to flip a switch is a browser
// session, so an operator surface has to send people to a second dashboard —
// and splitting one deliberate action across two systems is how "we thought it
// was off" happens.
func writeFlagAPISection(b *strings.Builder, publicURL, slug string) {
	const keyHeader = "X-FunnelBarn-Api-Key"

	fmt.Fprintf(b, "### Reading and toggling flags from a service\n\n")
	fmt.Fprintf(b, "Evaluation (above) tells a client what *it* resolves. To report what the\n")
	fmt.Fprintf(b, "project actually holds — and to change it — use a **project-scoped flag\n")
	fmt.Fprintf(b, "token**. That distinction matters: \"the flag is off\" and \"the flag does not\n")
	fmt.Fprintf(b, "exist and you are seeing your own default\" are very different answers.\n\n")
	fmt.Fprintf(b, "Create one in the dashboard under **Settings → API Keys**, with scope:\n\n")
	fmt.Fprintf(b, "| Scope | May |\n")
	fmt.Fprintf(b, "|-------|-----|\n")
	fmt.Fprintf(b, "| `flags:read` | list and read this project's flags |\n")
	fmt.Fprintf(b, "| `flags:write` | the above, plus update an existing flag (enable / disable / change its value) |\n\n")
	fmt.Fprintf(b, "Tokens are bound to one project, hashed at rest, revocable, and carry a\n")
	fmt.Fprintf(b, "last-used timestamp. Neither scope can **create or delete** a flag:\n")
	fmt.Fprintf(b, "auto-registration already covers creation, and a token that can turn a gate\n")
	fmt.Fprintf(b, "off should not also be able to delete the gate.\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "# Read the project's real flag state\n")
	fmt.Fprintf(b, "curl -s '%s/api/v1/projects/<project-id>/flags' \\\n", publicURL)
	fmt.Fprintf(b, "  -H '%s: <flags-token>'\n\n", keyHeader)
	fmt.Fprintf(b, "# Turn a gate off. Fields you omit keep their current values, so a toggle\n")
	fmt.Fprintf(b, "# is exactly this and nothing else.\n")
	fmt.Fprintf(b, "curl -s -X PUT '%s/api/v1/projects/<project-id>/flags/<flag-id>' \\\n", publicURL)
	fmt.Fprintf(b, "  -H '%s: <flags-token>' \\\n", keyHeader)
	fmt.Fprintf(b, "  -H 'Content-Type: application/json' \\\n")
	fmt.Fprintf(b, "  -d '{\"status\": \"paused\"}'\n")
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "The instance-wide `FUNNELBARN_API_KEY` is refused here: it resolves with no\n")
	fmt.Fprintf(b, "project, which would make it a master key for every project's flags. Use a\n")
	fmt.Fprintf(b, "key issued for `%s`.\n\n", slug)
}

// writeMCPCallout is the short pitch near the top of the guide, addressed to
// the AI assistant that is most often the one reading it. It asks the
// assistant to connect the MCP server before instrumenting, since every later
// step (checking events arrive, defining funnels from real event names,
// creating flags) goes faster through the tools than through the dashboard.
// Only called when the instance serves MCP.
func writeMCPCallout(b *strings.Builder, publicURL, slug string) {
	mcpURL := publicURL + mcpPath

	fmt.Fprintf(b, "## Using an AI assistant? Connect the FunnelBarn MCP server first\n\n")
	fmt.Fprintf(b, "> **If you are an AI assistant reading this guide:** add the FunnelBarn MCP\n")
	fmt.Fprintf(b, "> server before you start instrumenting. With it you can:\n")
	fmt.Fprintf(b, ">\n")
	fmt.Fprintf(b, "> - confirm your events arrive (`get_project`, `list_event_names`) the moment you ship them;\n")
	fmt.Fprintf(b, "> - create the funnels and segments below from the real event names (`create_funnel`, `create_segment`);\n")
	fmt.Fprintf(b, "> - create and check feature flags (`create_flag`, `evaluate_flag`);\n")
	fmt.Fprintf(b, "> - read stats and funnel conversion (`get_stats`, `analyze_funnel`).\n")
	fmt.Fprintf(b, ">\n")
	fmt.Fprintf(b, "> The user then never has to open the dashboard to set this project up. If you\n")
	fmt.Fprintf(b, "> can add MCP servers, run this now, or ask the user to approve it:\n")
	fmt.Fprintf(b, ">\n")
	fmt.Fprintf(b, "> ```bash\n")
	fmt.Fprintf(b, "> claude mcp add --transport http funnelbarn %s\n", mcpURL)
	fmt.Fprintf(b, "> ```\n")
	fmt.Fprintf(b, ">\n")
	fmt.Fprintf(b, "> Other clients: it is a streamable HTTP server at `%s`. The first call opens a\n", mcpURL)
	fmt.Fprintf(b, "> browser sign-in through IAMBarn; there is no key to paste. This project's slug\n")
	fmt.Fprintf(b, "> is `%s`: pass `\"project\": \"%s\"` on tool calls, and write the slug into this\n", slug, slug)
	fmt.Fprintf(b, "> repository's assistant notes (`CLAUDE.md`, `AGENTS.md` or similar) so later\n")
	fmt.Fprintf(b, "> sessions use it too. Details: [Connect an AI assistant (MCP)](#connect-an-ai-assistant-mcp).\n\n---\n\n")
}

// writeMCPSection documents the MCP endpoint, next to writeFlagAPISection:
// both are "here is a second way in, for a program rather than a browser"
// sections. Only called when the instance actually serves MCP (s.mcpEnabled),
// since the config it prints (endpoint, scopes) doesn't exist otherwise.
func writeMCPSection(b *strings.Builder, publicURL, slug string) {
	mcpURL := publicURL + mcpPath

	fmt.Fprintf(b, "## Connect an AI assistant (MCP)\n\n")
	fmt.Fprintf(b, "This project is also reachable over the [Model Context Protocol](https://modelcontextprotocol.io),\n")
	fmt.Fprintf(b, "so an assistant working in this repository can read its analytics and manage\n")
	fmt.Fprintf(b, "its funnels, segments and feature flags directly.\n\n")
	fmt.Fprintf(b, "| Key | Value |\n")
	fmt.Fprintf(b, "|-----|-------|\n")
	fmt.Fprintf(b, "| Endpoint | `%s` |\n", mcpURL)
	fmt.Fprintf(b, "| Sign-in | through IAMBarn, in the browser, on first use |\n")
	fmt.Fprintf(b, "| Scopes | `mcp:read`, `mcp:write` |\n")
	fmt.Fprintf(b, "| Project | `%s`, passed as the `project` argument |\n\n", slug)
	fmt.Fprintf(b, "There is no API key to paste: the first call opens a browser sign-in, and the\n")
	fmt.Fprintf(b, "assistant holds the resulting token from then on.\n\n")
	fmt.Fprintf(b, "**CLI**\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "claude mcp add --transport http funnelbarn %s\n", mcpURL)
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "**`.mcp.json`**\n\n")
	fmt.Fprintf(b, "```json\n")
	fmt.Fprintf(b, "{\"mcpServers\":{\"funnelbarn\":{\"type\":\"http\",\"url\":\"%s\"}}}\n", mcpURL)
	fmt.Fprintf(b, "```\n\n")
	fmt.Fprintf(b, "One server covers every project on this instance. Each project tool takes a\n")
	fmt.Fprintf(b, "`project` argument (slug or ID); on an instance with a single project it can\n")
	fmt.Fprintf(b, "be left out, and `list_projects` shows the slugs otherwise.\n\n")
	fmt.Fprintf(b, "**Optional: pin a default project.** Add the `%s` header and tools\n", mcp.ProjectHeader)
	fmt.Fprintf(b, "use this project whenever `project` is left out. Commit it in `.mcp.json` to\n")
	fmt.Fprintf(b, "give every contributor's assistant the same default:\n\n")
	fmt.Fprintf(b, "```bash\n")
	fmt.Fprintf(b, "claude mcp add --transport http funnelbarn %s --header \"%s: %s\"\n", mcpURL, mcp.ProjectHeader, slug)
	fmt.Fprintf(b, "```\n\n---\n\n")
}

// writeClosingSections renders the recommended funnels and the next steps.
// With MCP enabled, both point at the tools as the quicker route.
func writeClosingSections(b *strings.Builder, setupURL string, mcpEnabled bool) {
	fmt.Fprintf(b, "## Recommended Funnel Definitions\n\n")
	if mcpEnabled {
		fmt.Fprintf(b, "Create these in the admin UI once the project is approved, or with the\n")
		fmt.Fprintf(b, "`create_funnel` MCP tool after checking the real names with `list_event_names`:\n\n")
	} else {
		fmt.Fprintf(b, "Create these in the admin UI once the project is approved:\n\n")
	}
	fmt.Fprintf(b, "1. **Acquisition → Activation**\n")
	fmt.Fprintf(b, "   - Steps: `page-view` → `signup-started` → `signup-completed`\n\n")
	fmt.Fprintf(b, "2. **Checkout**\n")
	fmt.Fprintf(b, "   - Steps: `cart-viewed` → `checkout-started` → `payment-entered` → `checkout-completed`\n\n")
	fmt.Fprintf(b, "3. **Engagement**\n")
	fmt.Fprintf(b, "   - Steps: `page-view` → `feature-used` → `return-visit`\n\n---\n\n")

	fmt.Fprintf(b, "## Next Steps\n\n")
	step := 1
	if mcpEnabled {
		fmt.Fprintf(b, "%d. Connect the FunnelBarn MCP server (see the top of this guide) so you can verify and configure everything below from your assistant\n", step)
		step++
	}
	fmt.Fprintf(b, "%d. Instrument your app using the SDK examples above\n", step)
	fmt.Fprintf(b, "%d. Ask your FunnelBarn admin to approve this project at: **%s**\n", step+1, setupURL)
	if mcpEnabled {
		fmt.Fprintf(b, "%d. Once approved, confirm events arrive with `get_project`, then define funnels and flags with the MCP tools or in the dashboard\n", step+2)
	} else {
		fmt.Fprintf(b, "%d. Once approved, visit the dashboard to see live data, define funnels, and create feature flags\n", step+2)
	}
	fmt.Fprintf(b, "%d. (Optional) Enable session recording — add `recording: true` to the SDK and ask your admin to turn on recording for this project in the dashboard\n", step+3)
	fmt.Fprintf(b, "%d. (Optional) Wire flag evaluation into your app — see the Feature Flags section above\n", step+4)
}
