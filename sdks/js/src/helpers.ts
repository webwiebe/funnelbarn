import type { EventPayload, FunnelBarnOptions, RecordingRule, SessionSignals } from "./types.js";
import { randomHex } from "./trace-context.js";

/**
 * Reduces whatever was configured to a base URL. `endpoint` is the server root
 * (the SDK appends `/api/v1/events` itself) but the setup page used to hand
 * out the full ingest URL, so a config value copied from there produced
 * `/api/v1/events/api/v1/events` and 404'd every event. Accepting both
 * spellings costs one line and removes the whole failure mode. See #237.
 */
export function normaliseEndpoint(endpoint: string): string {
  const trimmed = endpoint.trim().replace(/\/+$/, "");
  return trimmed.replace(/\/api\/v1\/events$/, "");
}

export function generateSessionID(): string {
  return randomHex(16);
}

export function generateUUID(): string {
  const bytes = new Uint8Array(16);
  if (typeof crypto !== "undefined" && crypto.getRandomValues) {
    crypto.getRandomValues(bytes);
  } else {
    for (let i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256);
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export function collectSessionSignals(): SessionSignals {
  if (typeof window === "undefined") return {};
  const signals: SessionSignals = {};
  if (typeof screen !== "undefined") {
    signals.screen_width = screen.width;
    signals.screen_height = screen.height;
  }
  if (typeof window.devicePixelRatio !== "undefined") {
    signals.pixel_ratio = window.devicePixelRatio;
  }
  if (typeof navigator !== "undefined") {
    if (typeof navigator.maxTouchPoints !== "undefined") {
      signals.touch = navigator.maxTouchPoints > 0;
    }
    if (typeof navigator.hardwareConcurrency !== "undefined") {
      signals.cpu_cores = navigator.hardwareConcurrency;
    }
  }
  try {
    signals.dark_mode = window.matchMedia(
      "(prefers-color-scheme: dark)"
    ).matches;
  } catch {
    // ignore
  }
  try {
    signals.reduced_motion = window.matchMedia(
      "(prefers-reduced-motion: reduce)"
    ).matches;
  } catch {
    // ignore
  }
  try {
    signals.browser_timezone =
      Intl.DateTimeFormat().resolvedOptions().timeZone;
  } catch {
    // ignore
  }
  return signals;
}

export function detectURL(): string | undefined {
  if (typeof window !== "undefined" && window.location) {
    return window.location.href;
  }
  return undefined;
}

export function detectReferrer(): string | undefined {
  if (typeof document !== "undefined" && document.referrer) {
    return document.referrer;
  }
  return undefined;
}

export function extractUTMs(url?: string): Partial<EventPayload> {
  if (!url) return {};
  try {
    const u = new URL(url);
    const result: Partial<EventPayload> = {};
    if (u.searchParams.get("utm_source"))
      result.utm_source = u.searchParams.get("utm_source")!;
    if (u.searchParams.get("utm_medium"))
      result.utm_medium = u.searchParams.get("utm_medium")!;
    if (u.searchParams.get("utm_campaign"))
      result.utm_campaign = u.searchParams.get("utm_campaign")!;
    if (u.searchParams.get("utm_term"))
      result.utm_term = u.searchParams.get("utm_term")!;
    if (u.searchParams.get("utm_content"))
      result.utm_content = u.searchParams.get("utm_content")!;
    return result;
  } catch {
    return {};
  }
}

// matchesPattern checks a URL path against a glob pattern.
// * matches any single path segment, ** matches any number of segments.
function matchesPattern(pattern: string, path: string): boolean {
  const escaped = pattern
    .replace(/[.+^${}()|[\]\\]/g, '\\$&')
    .replace(/\*\*/g, '\x00')
    .replace(/\*/g, '[^/]*')
    .replace(/\x00/g, '.*');
  try {
    return new RegExp(`^${escaped}$`).test(path);
  } catch {
    return false;
  }
}

// shouldRecordPath decides whether a page is captured: SDK-local exclude and
// include first (highest priority), then the server rules (first match wins),
// and capture by default.
export function shouldRecordPath(path: string, opts: FunnelBarnOptions, serverRules: RecordingRule[]): boolean {
  for (const p of opts.recordExclude ?? []) {
    if (matchesPattern(p, path)) return false;
  }
  for (const p of opts.recordInclude ?? []) {
    if (matchesPattern(p, path)) return true;
  }
  for (const rule of serverRules) {
    if (matchesPattern(rule.pattern, path)) {
      return rule.action === 'capture';
    }
  }
  return true;
}
