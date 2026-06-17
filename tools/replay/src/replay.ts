/**
 * Playwright-driven rendering of an assembled replay page.
 *
 * Playwright and the rrweb-player assets are loaded lazily so the data layer
 * (client + assemble) stays usable — and testable — without a browser install.
 */

import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";

export interface PlayerAssets {
  js: string;
  css: string;
}

/**
 * Load the rrweb-player UMD JS + CSS from the installed package so the replay
 * page is fully offline. Throws a clear, actionable error if the dependency is
 * missing.
 */
export function loadPlayerAssets(): PlayerAssets {
  const require = createRequire(import.meta.url);
  let pkgJsonPath: string;
  try {
    pkgJsonPath = require.resolve("rrweb-player/package.json");
  } catch {
    throw new Error(
      "rrweb-player is not installed. Run `npm install` in tools/replay before replaying."
    );
  }
  const root = dirname(pkgJsonPath);
  // rrweb-player ships a UMD bundle + stylesheet under dist/.
  const jsCandidates = ["dist/index.js", "dist/rrweb-player.umd.cjs", "dist/index.umd.js"];
  const cssCandidates = ["dist/style.css", "dist/index.css"];
  const js = readFirst(root, jsCandidates, "rrweb-player JS bundle");
  const css = readFirst(root, cssCandidates, "rrweb-player CSS", true);
  return { js, css };
}

function readFirst(root: string, candidates: string[], label: string, optional = false): string {
  for (const c of candidates) {
    try {
      return readFileSync(join(root, c), "utf8");
    } catch {
      // try next
    }
  }
  if (optional) return "";
  throw new Error(`could not locate ${label} in rrweb-player (looked for: ${candidates.join(", ")})`);
}

export interface RenderOptions {
  html: string;
  /** Run a visible browser window so a developer can watch/scrub. */
  headed?: boolean;
  /** If set, capture a PNG at the seek point to this path. */
  screenshotPath?: string;
  /** How long to keep a headed window open before exiting (ms). 0 = until closed. */
  keepOpenMs?: number;
}

/**
 * Render the replay HTML in a Playwright Chromium. Returns once the screenshot
 * (if requested) is taken and the keep-open window has elapsed.
 */
export async function renderReplay(opts: RenderOptions): Promise<void> {
  let chromium;
  try {
    ({ chromium } = await import("playwright"));
  } catch {
    throw new Error(
      "playwright is not installed. Run `npm install && npx playwright install chromium` in tools/replay."
    );
  }

  const browser = await chromium.launch({ headless: !opts.headed });
  try {
    const page = await browser.newPage({ viewport: { width: 1100, height: 720 } });
    page.on("console", (msg) => {
      if (msg.type() === "error") console.error(`[replay page] ${msg.text()}`);
    });
    await page.setContent(opts.html, { waitUntil: "load" });
    // Wait for the player to mount and seek.
    await page.waitForFunction("window.__fbReplayReady === true", { timeout: 15000 }).catch(() => {
      console.error("warning: replay player did not signal ready within 15s");
    });
    // Give the player a beat to paint the seeked frame before snapshotting.
    await page.waitForTimeout(1000);

    if (opts.screenshotPath) {
      await page.screenshot({ path: opts.screenshotPath, fullPage: false });
      console.error(`screenshot written to ${opts.screenshotPath}`);
    }

    if (opts.headed) {
      const keep = opts.keepOpenMs ?? 0;
      if (keep > 0) {
        await page.waitForTimeout(keep);
      } else {
        // Stay open until the user closes the window.
        await page.waitForEvent("close", { timeout: 0 }).catch(() => {});
      }
    }
  } finally {
    await browser.close();
  }
}
