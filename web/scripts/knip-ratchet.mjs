#!/usr/bin/env node
// knip (dead code in the web app), as a ratchet. Unused files, exports, types
// and dependencies are counted per item and compared with knip-baseline.json.
//
// Same shape as oxlint-ratchet.mjs: the gate is symmetric. An item that is not
// in the baseline fails (new dead code), and so does a baseline item that is
// gone (stale allowance: it leaves room for that item to come back with the
// gate still green). The failure prints the refresh command.
//
// `--write` refreshes the baseline after a deliberate change.
// `--report-only` prints the same report and exits 0 when the tree and the
// baseline disagree; a run below the file floor still exits 1. CI runs this mode
// while the gate soaks, until 2026-10-31; drop the flag to enforce.
//
// Items are keyed `type:file:name`, without line numbers, so moving code inside
// a file does not read as one finding gone and another new.

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const BASELINE_FILE = "knip-baseline.json";

// knip exits 0 with nothing to report when it cannot read the sources (a TypeScript
// major it does not support, a project glob that matches no file). A run that
// analysed fewer files than this is an error in every mode, report-only included.
// The tree analyses 136 today.
export const MIN_FILES = 100;

// The issue types knip reports that mean "this is unused". `duplicates` and
// `enumMembers` are left out on purpose: they are style findings, not dead code.
export const ITEM_TYPES = [
  "dependencies",
  "devDependencies",
  "optionalPeerDependencies",
  "unlisted",
  "binaries",
  "unresolved",
  "exports",
  "types",
];

/** The command that refreshes the baseline, printed verbatim on every failure. */
export function refreshCommand() {
  return "node scripts/knip-ratchet.mjs --write";
}

/**
 * Flatten knip's JSON report to a sorted list of unique item keys.
 * Pure, so the gate can be tested without running knip.
 */
export function flatten(report) {
  const keys = new Set();
  for (const file of report.files ?? []) keys.add(`files:${file}:`);
  for (const issue of report.issues ?? []) {
    // knip's json reporter lists an unused file as an issue of its own.
    if ((issue.files ?? []).length > 0) keys.add(`files:${issue.file}:`);
    for (const type of ITEM_TYPES) {
      for (const item of issue[type] ?? []) {
        keys.add(`${type}:${issue.file}:${item.name}`);
      }
    }
  }
  return [...keys].sort();
}

/**
 * Compare the current items with the baseline.
 *
 *   - an item not in the baseline  -> added (new dead code)
 *   - a baseline item no longer found -> removed (stale allowance)
 *
 * Both fail, so `ok` requires neither.
 */
export function compare(baselineItems, currentItems) {
  const was = new Set(baselineItems);
  const now = new Set(currentItems);
  const added = [...now].filter((k) => !was.has(k)).sort();
  const removed = [...was].filter((k) => !now.has(k)).sort();
  return {
    ok: added.length === 0 && removed.length === 0,
    added,
    removed,
    total: now.size,
    baselineTotal: was.size,
  };
}

/** Files knip parsed, from the `getImportsAndExports` row of its --performance table. */
export function analysedFiles(performanceOutput) {
  const row = performanceOutput.split("\n").find((l) => l.trimStart().startsWith("getImportsAndExports"));
  const count = row ? Number(row.trim().split(/\s+/)[1]) : 0;
  return Number.isInteger(count) ? count : 0;
}

/** The floor's verdict: an error message, or null when knip saw enough files. */
export function checkFloor(performanceOutput, minFiles = MIN_FILES) {
  const analysed = analysedFiles(performanceOutput);
  return analysed < minFiles
    ? `knip analysed ${analysed} file(s), expected at least ${minFiles}: nothing was measured`
    : null;
}

// The web package's own knip, pinned in package.json. `npx knip` could fetch a
// different version at run time.
const WEB_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const KNIP_BIN = path.join(WEB_DIR, "node_modules", ".bin", "knip");

export function runKnipPerformance(dir) {
  try {
    return execFileSync(KNIP_BIN, ["--performance", "--no-exit-code"], {
      cwd: dir,
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
    });
  } catch (err) {
    return err.stdout ?? "";
  }
}

export function runKnip(dir) {
  let raw = "";
  try {
    raw = execFileSync(KNIP_BIN, ["--reporter", "json", "--no-exit-code"], {
      cwd: dir,
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
    });
  } catch (err) {
    if (!err.stdout) throw new Error(`knip did not run: ${err.message}`);
    raw = err.stdout;
  }
  return JSON.parse(raw);
}

/**
 * The gate over the package in `dir`; returns the exit code. The floor fails in
 * every mode. `--report-only` turns a disagreement with the baseline into exit 0;
 * it never touches the floor.
 */
export function run({ dir = WEB_DIR, args = [], minFiles = MIN_FILES, out = console.log, err = console.error } = {}) {
  const baselinePath = path.join(dir, BASELINE_FILE);
  const write = args.includes("--write");
  const reportOnly = args.includes("--report-only");

  const floorProblem = checkFloor(runKnipPerformance(dir), minFiles);
  if (floorProblem) {
    err(`knip ratchet FAILED: ${floorProblem}`);
    return 1;
  }

  const items = flatten(runKnip(dir));

  if (write) {
    fs.writeFileSync(baselinePath, `${JSON.stringify({ total: items.length, items }, null, 2)}\n`);
    out(`knip baseline written: ${items.length} items`);
    return 0;
  }

  const baseline = fs.existsSync(baselinePath)
    ? JSON.parse(fs.readFileSync(baselinePath, "utf8"))
    : { total: 0, items: [] };
  const res = compare(baseline.items, items);

  if (res.ok && res.total === 0) {
    out("knip: clean");
    return 0;
  }
  out(`knip: ${res.total} items (baseline ${res.baselineTotal})`);
  if (res.ok) {
    out("knip: on the baseline, exactly.");
    return 0;
  }

  const say = reportOnly ? out : err;
  say("");
  say(
    reportOnly
      ? "knip ratchet: the tree and the baseline disagree, but this run is report-only."
      : "knip ratchet FAILED: the tree and the baseline disagree.",
  );
  if (res.added.length) {
    say("");
    say("Dead code that is not in the baseline:");
    for (const k of res.added) say(`  ${k}`);
    say("");
    say("Delete it, or stop exporting it if only its own file uses it.");
  }
  if (res.removed.length) {
    say("");
    say("Baseline items that are gone, so the baseline is now stale:");
    for (const k of res.removed) say(`  ${k}`);
    say("");
    say("The baseline has to follow the work down. Lock the improvement in:");
    say(`  ${refreshCommand()}`);
  }

  if (reportOnly) {
    out("");
    out("knip ratchet: report-only run, not gating.");
    return 0;
  }
  return 1;
}

// Only run when invoked directly, so the test can import the pure helpers.
if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  const code = run({ args: process.argv.slice(2) });
  if (code !== 0) process.exit(code);
}
