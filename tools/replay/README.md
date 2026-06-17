# funnelbarn-replay

Resolve a W3C `trace_id` — the one you see on an error span in **SpanBarn** or an
issue in **BugBarn** — to the **FunnelBarn** session recording that captured it,
and replay that recording locally in a Playwright-driven browser, **seeking
straight to the moment the trace fired**.

This is the consumer end of the trace-correlation link:

```
SpanBarn / BugBarn (trace_id)
        │
        ▼
FunnelBarn  GET /api/v1/traces/{trace_id}     ← resolve to session + recording + seek offset
        │   GET /api/v1/recordings/{rid}/chunks/{i}   ← fetch rrweb chunks
        ▼
rrweb-player in Playwright Chromium, auto-seeked to offset_ms
```

## Install

```bash
cd tools/replay
npm install
npx playwright install chromium   # one-time browser download (needed for live replay)
npm run build
```

`npm install` alone (with `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1`) is enough for
`--dry-run`, which never launches a browser.

## Usage

```bash
export FUNNELBARN_ENDPOINT=https://funnelbarn.wiebe.xyz
export FUNNELBARN_API_KEY=fb_xxx          # a key scoped to the recording's project

# Watch the session, auto-seeked to the trace moment:
node dist/cli.js --trace 4bf92f3577b34da6a3ce929d0e0e4736

# Headless, just grab the frame at the trace moment:
node dist/cli.js --trace <id> --headless --screenshot ./moment.png

# Resolve + assemble only (no browser), for scripting/agents:
node dist/cli.js --trace <id> --dry-run

# Save a standalone, offline replay page you can open anywhere:
node dist/cli.js --trace <id> --out ./replay.html
```

| Flag | Meaning |
|------|---------|
| `--trace <id>` | trace_id from SpanBarn/BugBarn (required) |
| `--endpoint <url>` | FunnelBarn base URL (env `FUNNELBARN_ENDPOINT`) |
| `--api-key <key>` | FunnelBarn API key (env `FUNNELBARN_API_KEY`) |
| `--headed` / `--headless` | visible window (default) vs. offscreen |
| `--screenshot <path>` | PNG at the seek point |
| `--out <path>` | write the self-contained replay HTML |
| `--keep-open <ms>` | headed window lifetime, `0` = until closed |
| `--dry-run` | resolve + fetch + assemble, print JSON, no browser |

## How the link is formed

The FunnelBarn browser SDK observes the `traceparent` on outgoing requests while
recording and ships `(trace_id, span_id, url, occurred_at)` with each rrweb
chunk. FunnelBarn stores those in `recording_traces`, so a trace_id resolves back
to the recording and the **seek offset** (`occurred_at − recording.started_at`).

When the app isn't instrumented, the SDK can inject a **deterministic**
traceparent whose `trace_id` *is* the `recording_id` (opt-in `tracePropagate`),
so the SpanBarn trace correlates with no lookup at all.

## Reproducing application state (roadmap)

Replaying the recording reconstructs exactly what the **user saw** (the DOM), which
is usually enough to understand a bug. Getting the **backend** into the same state
— spinning the whole stack locally against a restored database snapshot so you can
re-drive the exact requests — is a larger, infra-specific step. The intended
workflow, to be automated next:

1. `funnelbarn-replay --trace <id> --dry-run` → identify the session, time window,
   and (via the harvested trace links) the exact backend traces involved.
2. Restore the relevant DB snapshot into a local stack (deploy scripts under
   `deploy/`).
3. Point the app at the local stack (`--base-url`, reserved) and step through the
   replay alongside the restored data.

Today the tool delivers steps that need no infra: resolve → fetch → replay →
seek. The `--base-url` hook is reserved for the stack-bound step.
