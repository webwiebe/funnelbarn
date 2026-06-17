#!/usr/bin/env node
/**
 * funnelbarn-replay — resolve a trace_id to a session recording and replay it.
 *
 *   funnelbarn-replay --trace <trace_id> [--endpoint URL] [--api-key KEY] [options]
 *
 * Endpoint/key default to FUNNELBARN_ENDPOINT / FUNNELBARN_API_KEY.
 */

import { parseArgs } from "node:util";
import { writeFileSync } from "node:fs";
import { ReplayClient } from "./client.js";
import { assembleEvents, buildReplayHTML } from "./assemble.js";

const USAGE = `funnelbarn-replay — replay the session recording behind a trace

Usage:
  funnelbarn-replay --trace <trace_id> [options]

Options:
  --trace <id>         W3C trace_id (from SpanBarn/BugBarn). Required.
  --endpoint <url>     FunnelBarn base URL (env: FUNNELBARN_ENDPOINT).
  --api-key <key>      FunnelBarn API key (env: FUNNELBARN_API_KEY).
  --headed             Open a visible browser window to watch/scrub (default).
  --headless           Render without a window (use with --screenshot).
  --screenshot <path>  Save a PNG at the trace moment.
  --out <path>         Write the self-contained replay HTML to a file.
  --keep-open <ms>     Headed window lifetime; 0 = until closed (default 0).
  --dry-run            Resolve + fetch + assemble only; no browser.
  -h, --help           Show this help.
`;

interface ParsedArgs {
  trace: string;
  endpoint: string;
  apiKey: string;
  headed: boolean;
  screenshot?: string;
  out?: string;
  keepOpen: number;
  dryRun: boolean;
}

export function parseCliArgs(argv: string[], env: Record<string, string | undefined>): ParsedArgs {
  const { values } = parseArgs({
    args: argv,
    options: {
      trace: { type: "string" },
      endpoint: { type: "string" },
      "api-key": { type: "string" },
      headed: { type: "boolean" },
      headless: { type: "boolean" },
      screenshot: { type: "string" },
      out: { type: "string" },
      "keep-open": { type: "string" },
      "dry-run": { type: "boolean" },
      help: { type: "boolean", short: "h" },
    },
    allowPositionals: false,
  });

  if (values.help) {
    process.stdout.write(USAGE);
    process.exit(0);
  }

  const trace = values.trace ?? "";
  const endpoint = values.endpoint ?? env.FUNNELBARN_ENDPOINT ?? "";
  const apiKey = values["api-key"] ?? env.FUNNELBARN_API_KEY ?? "";

  const missing: string[] = [];
  if (!trace) missing.push("--trace");
  if (!endpoint) missing.push("--endpoint (or FUNNELBARN_ENDPOINT)");
  if (!apiKey) missing.push("--api-key (or FUNNELBARN_API_KEY)");
  if (missing.length) {
    throw new Error(`missing required: ${missing.join(", ")}\n\n${USAGE}`);
  }

  return {
    trace,
    endpoint,
    apiKey,
    headed: values.headless ? false : true,
    screenshot: values.screenshot,
    out: values.out,
    keepOpen: values["keep-open"] ? Number(values["keep-open"]) : 0,
    dryRun: Boolean(values["dry-run"]),
  };
}

export async function run(args: ParsedArgs): Promise<void> {
  const client = new ReplayClient({ endpoint: args.endpoint, apiKey: args.apiKey });

  console.error(`resolving trace ${args.trace} …`);
  const lookup = await client.lookupTrace(args.trace);
  console.error(
    `→ recording ${lookup.recording_id} (session ${lookup.session_id}), ` +
      `seek +${lookup.offset_ms}ms of ${lookup.duration_ms}ms, ` +
      `chunks ${lookup.first_chunk_index}..${lookup.last_chunk_index}` +
      (lookup.page_url ? `, page ${lookup.page_url}` : "")
  );

  const raw = await client.fetchAllEvents(lookup, (w) => console.error(`  ${w}`));
  const assembled = assembleEvents(raw);
  console.error(
    `assembled ${assembled.events.length} events, ` +
      `${assembled.durationMs}ms, snapshot=${assembled.hasSnapshot}`
  );
  if (!assembled.hasSnapshot) {
    console.error("warning: no full snapshot in this recording — playback may be blank.");
  }

  let html = "";
  const needHtml = args.out || !args.dryRun;
  if (needHtml) {
    const { loadPlayerAssets } = await import("./replay.js");
    const assets = loadPlayerAssets();
    html = buildReplayHTML({
      events: assembled.events,
      seekMs: lookup.offset_ms,
      playerJs: assets.js,
      playerCss: assets.css,
      banner: `trace ${args.trace} · session ${lookup.session_id}` + (lookup.url ? ` · ${lookup.url}` : ""),
    });
  }

  if (args.out) {
    writeFileSync(args.out, html, "utf8");
    console.error(`replay HTML written to ${args.out}`);
  }

  if (args.dryRun) {
    process.stdout.write(
      JSON.stringify(
        {
          recording_id: lookup.recording_id,
          session_id: lookup.session_id,
          offset_ms: lookup.offset_ms,
          duration_ms: lookup.duration_ms,
          chunk_count: lookup.chunk_count,
          event_count: assembled.events.length,
          has_snapshot: assembled.hasSnapshot,
        },
        null,
        2
      ) + "\n"
    );
    return;
  }

  const { renderReplay } = await import("./replay.js");
  await renderReplay({
    html,
    headed: args.headed,
    screenshotPath: args.screenshot,
    keepOpenMs: args.keepOpen,
  });
}

// Entry point when run as a binary (not when imported by tests).
const isMain = import.meta.url === `file://${process.argv[1]}`;
if (isMain) {
  try {
    const args = parseCliArgs(process.argv.slice(2), process.env);
    await run(args);
  } catch (err) {
    console.error(`error: ${(err as Error).message}`);
    process.exit(1);
  }
}
