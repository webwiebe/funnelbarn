/**
 * FunnelBarn browser + Node.js SDK.
 *
 * Tracks page views and custom events. Batches events and flushes every
 * 5 seconds or on beforeunload. Generates a session ID in localStorage
 * with a 30-minute idle timeout (browser only).
 */

import { record } from "rrweb";

export interface RecordingRule {
  pattern: string;
  action: 'capture' | 'ignore';
}

export interface FunnelBarnOptions {
  apiKey: string;
  endpoint: string;
  /** Optional project name sent as x-funnelbarn-project header */
  projectName?: string;
  /** Flush interval in ms (default: 5000) */
  flushInterval?: number;
  /** Session idle timeout in ms (default: 30 minutes) */
  sessionTimeout?: number;
  /** Enable session recording by default (default: false). Server config can override. */
  recording?: boolean;
  /** Recording chunk flush interval in ms (default: 10000) */
  recordingChunkMs?: number;
  /** URL path patterns to always capture (checked before server rules) */
  recordInclude?: string[];
  /** URL path patterns to always ignore (checked before server rules) */
  recordExclude?: string[];
}

export interface EventProperties {
  [key: string]: unknown;
}

interface SessionSignals {
  screen_width?: number;
  screen_height?: number;
  pixel_ratio?: number;
  touch?: boolean;
  dark_mode?: boolean;
  reduced_motion?: boolean;
  browser_timezone?: string;
  cpu_cores?: number;
}

interface EventPayload {
  name: string;
  url?: string;
  referrer?: string;
  utm_source?: string;
  utm_medium?: string;
  utm_campaign?: string;
  utm_term?: string;
  utm_content?: string;
  properties?: EventProperties;
  session_id?: string;
  user_id?: string;
  timestamp: string;
  page_view_id?: string;
  session_signals?: Record<string, unknown>;
  vitals?: Record<string, number>;
}

const SESSION_KEY = "funnelbarn_sid";
const SESSION_EXPIRY_KEY = "funnelbarn_sid_exp";
const SESSION_TIMEOUT_DEFAULT = 30 * 60 * 1000; // 30 min

/**
 * FunnelBarnClient is the main analytics client.
 */
export class FunnelBarnClient {
  private readonly apiKey: string;
  private readonly endpoint: string;
  private readonly projectName?: string;
  private readonly flushInterval: number;
  private readonly sessionTimeout: number;

  private queue: EventPayload[] = [];
  private flushTimer?: ReturnType<typeof setInterval>;
  private userId?: string;

  // page_view_id
  private currentPageViewId: string | undefined;

  // session signals
  private sessionSignalsSent = false;

  // engagement tracking
  private engagementTimer?: ReturnType<typeof setTimeout>;
  private engagementScrollHandler?: () => void;
  private engagementFired = false;

  // web vitals
  private pendingVitals: Record<string, number> = {};
  private vitalsPageViewId: string | undefined;
  private vitalsFlushed = false;

  // session recording
  private rrwebStop?: () => void;
  private rrwebBuffer: unknown[] = [];
  private recordingId = '';
  private recordingChunkIndex = 0;
  private recordingStartedAt = '';
  private recordingStartMs = 0;
  private recordingTimer?: ReturnType<typeof setInterval>;
  private recordingOptions: FunnelBarnOptions;
  private serverRecordingRules: RecordingRule[] = [];

  constructor(options: FunnelBarnOptions) {
    this.apiKey = options.apiKey;
    this.endpoint = options.endpoint.replace(/\/$/, "");
    this.projectName = options.projectName;
    this.flushInterval = options.flushInterval ?? 5000;
    this.sessionTimeout = options.sessionTimeout ?? SESSION_TIMEOUT_DEFAULT;
    this.recordingOptions = options;

    this.startFlushTimer();

    if (typeof window !== "undefined") {
      // Auto-flush on page unload (browser only).
      window.addEventListener("beforeunload", () => this.flush());

      // Register vitals flush listeners once in the constructor.
      const flushVitals = () => {
        if (this.vitalsFlushed || !this.vitalsPageViewId) return;
        if (Object.keys(this.pendingVitals).length === 0) return;
        this.vitalsFlushed = true;
        this.queue.push({
          name: "web_vitals",
          page_view_id: this.vitalsPageViewId,
          session_id: this.getOrCreateSessionID(),
          properties: { ...this.pendingVitals },
          timestamp: new Date().toISOString(),
        });
        this.flush().catch(() => {});
      };

      if (typeof document !== "undefined") {
        document.addEventListener("visibilitychange", () => {
          if (document.hidden) flushVitals();
        });
      }
      window.addEventListener("beforeunload", flushVitals);

      // Session recording — start locally if requested, then apply server overrides async.
      if (options.recording) {
        this.startRecording(options.recordingChunkMs);
      }
      // Fetch server recording config and apply overrides (does not block init).
      this.applyServerRecordingConfig(options.recordingChunkMs).catch(() => {});

      // LCP observer.
      try {
        new PerformanceObserver((list) => {
          const entries = list.getEntries();
          if (entries.length) {
            this.pendingVitals.lcp = Math.round(
              entries[entries.length - 1].startTime
            );
          }
        }).observe({ type: "largest-contentful-paint", buffered: true });
      } catch {
        // Not supported in this environment.
      }

      // FCP observer.
      try {
        new PerformanceObserver((list) => {
          const e = list.getEntriesByName("first-contentful-paint")[0];
          if (e) this.pendingVitals.fcp = Math.round(e.startTime);
        }).observe({ type: "paint", buffered: true });
      } catch {
        // Not supported in this environment.
      }
    }
  }

  /**
   * Track a page view. Auto-detects URL and referrer from window.location
   * when running in a browser.
   */
  page(properties?: EventProperties): void {
    // Generate a new page_view_id for this page view.
    this.currentPageViewId = this.generateUUID();

    // Clean up engagement tracking from any previous page view.
    this.cleanupEngagement();

    // Reset web vitals state for this page view.
    this.pendingVitals = {};
    this.vitalsPageViewId = this.currentPageViewId;
    this.vitalsFlushed = false;

    // Collect TTFB synchronously.
    const vitalsForEvent: Record<string, number> = {};
    if (typeof performance !== "undefined") {
      const nav = performance.getEntriesByType(
        "navigation"
      )[0] as PerformanceNavigationTiming | undefined;
      if (nav) {
        const ttfb = Math.round(nav.responseStart - nav.requestStart);
        this.pendingVitals.ttfb = ttfb;
        vitalsForEvent.ttfb = ttfb;
      }
    }

    // Collect session signals on the very first page() call.
    let sessionSignals: SessionSignals | undefined;
    if (!this.sessionSignalsSent) {
      this.sessionSignalsSent = true;
      sessionSignals = this.collectSessionSignals();
    }

    const url = this.detectURL();
    const referrer = this.detectReferrer();
    const utms = this.extractUTMs(url);

    const payload: EventPayload = {
      name: "page_view",
      url,
      referrer,
      ...utms,
      properties,
      session_id: this.getOrCreateSessionID(),
      user_id: this.userId,
      timestamp: new Date().toISOString(),
      page_view_id: this.currentPageViewId,
    };

    if (sessionSignals && Object.keys(sessionSignals).length > 0) {
      payload.session_signals = sessionSignals as Record<string, unknown>;
    }

    if (Object.keys(vitalsForEvent).length > 0) {
      payload.vitals = vitalsForEvent;
    }

    this.queue.push(payload);

    // Start engagement tracking for this page view.
    this.startEngagementTracking();
  }

  /**
   * Track a custom event.
   */
  track(name: string, properties?: EventProperties): void {
    if (!name) return;
    const url = this.detectURL();
    const utms = this.extractUTMs(url);

    this.queue.push({
      name,
      url,
      ...utms,
      properties,
      session_id: this.getOrCreateSessionID(),
      user_id: this.userId,
      timestamp: new Date().toISOString(),
      page_view_id: this.currentPageViewId,
    });
  }

  /**
   * Associate a user ID with subsequent events.
   */
  identify(userId: string): void {
    this.userId = userId || undefined;
  }

  /**
   * Flush queued events to the server immediately.
   */
  async flush(): Promise<void> {
    if (this.queue.length === 0) return;

    const batch = this.queue.splice(0, this.queue.length);

    for (const event of batch) {
      try {
        await this.sendEvent(event);
      } catch {
        // Best-effort: silently discard on error.
      }
    }
  }

  // --------------------------------------------------------------------------

  private async sendEvent(event: EventPayload): Promise<void> {
    const url = `${this.endpoint}/api/v1/events`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      "x-funnelbarn-api-key": this.apiKey,
    };
    if (this.projectName) {
      headers["x-funnelbarn-project"] = this.projectName;
    }

    const body = JSON.stringify(event);

    if (typeof fetch !== "undefined") {
      // Use fetch (browser + Node 18+).
      const response = await fetch(url, {
        method: "POST",
        headers,
        body,
        keepalive: true,
      });
      if (!response.ok) {
        throw new Error(`funnelbarn: server returned ${response.status}`);
      }
    } else {
      // Node < 18: use http/https module.
      await sendWithHttp(url, headers, body);
    }
  }

  private startFlushTimer(): void {
    if (typeof setInterval !== "undefined") {
      this.flushTimer = setInterval(() => {
        this.flush().catch(() => {});
      }, this.flushInterval);

      // Allow Node.js process to exit even if timer is active.
      if (
        this.flushTimer &&
        typeof this.flushTimer === "object" &&
        "unref" in this.flushTimer
      ) {
        (this.flushTimer as NodeJS.Timeout).unref();
      }
    }
  }

  private getOrCreateSessionID(): string {
    if (typeof localStorage === "undefined") {
      return "";
    }

    const now = Date.now();
    const expiry = parseInt(
      localStorage.getItem(SESSION_EXPIRY_KEY) ?? "0",
      10
    );

    if (now < expiry) {
      // Extend session expiry on activity.
      localStorage.setItem(
        SESSION_EXPIRY_KEY,
        String(now + this.sessionTimeout)
      );
      return localStorage.getItem(SESSION_KEY) ?? this.generateSessionID();
    }

    // New session.
    const id = this.generateSessionID();
    localStorage.setItem(SESSION_KEY, id);
    localStorage.setItem(SESSION_EXPIRY_KEY, String(now + this.sessionTimeout));
    return id;
  }

  private generateSessionID(): string {
    const bytes = new Uint8Array(16);
    if (typeof crypto !== "undefined" && crypto.getRandomValues) {
      crypto.getRandomValues(bytes);
    } else {
      for (let i = 0; i < bytes.length; i++) {
        bytes[i] = Math.floor(Math.random() * 256);
      }
    }
    return Array.from(bytes)
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
  }

  private generateUUID(): string {
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

  private collectSessionSignals(): SessionSignals {
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

  private startEngagementTracking(): void {
    if (typeof window === "undefined") return;

    this.engagementFired = false;
    const pageViewId = this.currentPageViewId;

    const fireEngagement = () => {
      if (this.engagementFired) return;
      this.engagementFired = true;
      this.cleanupEngagement();
      this.queue.push({
        name: "page_engaged",
        page_view_id: pageViewId,
        session_id: this.getOrCreateSessionID(),
        timestamp: new Date().toISOString(),
      });
    };

    // 15-second wall-clock timer using 1s ticks.
    let ticks = 0;
    const tick = () => {
      if (this.engagementFired) return;
      ticks++;
      if (ticks >= 15) {
        fireEngagement();
      } else {
        this.engagementTimer = setTimeout(tick, 1000);
      }
    };
    this.engagementTimer = setTimeout(tick, 1000);

    // Passive scroll listener — fires when scrolled past 50%.
    const scrollHandler = () => {
      const scrollable =
        document.documentElement.scrollHeight -
        document.documentElement.clientHeight;
      if (scrollable <= 0) return;
      if (
        window.scrollY / scrollable >= 0.5
      ) {
        fireEngagement();
      }
    };
    this.engagementScrollHandler = scrollHandler;
    window.addEventListener("scroll", scrollHandler, { passive: true });
  }

  private cleanupEngagement(): void {
    if (this.engagementTimer !== undefined) {
      clearTimeout(this.engagementTimer);
      this.engagementTimer = undefined;
    }
    if (this.engagementScrollHandler !== undefined && typeof window !== "undefined") {
      window.removeEventListener("scroll", this.engagementScrollHandler);
      this.engagementScrollHandler = undefined;
    }
  }

  private startRecording(chunkMs?: number): void {
    if (this.rrwebStop) return; // already running
    this.recordingId = this.generateSessionID();
    this.recordingStartedAt = new Date().toISOString();
    this.recordingStartMs = Date.now();
    this.rrwebStop = record({
      emit: (event) => { this.rrwebBuffer.push(event); },
      maskInputOptions: { password: true },
      blockClass: 'fb-block',
    });
    const interval = chunkMs ?? 10_000;
    this.recordingTimer = setInterval(() => { this.flushRecordingChunk().catch(() => {}); }, interval);
    window.addEventListener('beforeunload', () => { this.flushRecordingChunk().catch(() => {}); });
  }

  private stopRecording(): void {
    if (this.rrwebStop) {
      this.rrwebStop();
      this.rrwebStop = undefined;
    }
    if (this.recordingTimer !== undefined) {
      clearInterval(this.recordingTimer);
      this.recordingTimer = undefined;
    }
    this.rrwebBuffer = [];
  }

  private async applyServerRecordingConfig(chunkMs?: number): Promise<void> {
    try {
      const resp = await fetch(`${this.endpoint}/api/v1/recording-config`, {
        headers: { 'x-funnelbarn-api-key': this.apiKey },
      });
      if (!resp.ok) return;
      const cfg = await resp.json() as { enabled: boolean; sample_rate: number; rules: RecordingRule[] };
      this.serverRecordingRules = cfg.rules ?? [];

      // Apply sample-rate decision: skip recording if random roll is above rate.
      const rate = cfg.sample_rate ?? 1;
      const sampled = Math.random() < rate;

      if (cfg.enabled && sampled && !this.rrwebStop) {
        this.startRecording(chunkMs);
      } else if (!cfg.enabled && this.rrwebStop) {
        this.stopRecording();
      }
    } catch {
      // best-effort — local config remains in effect
    }
  }

  // matchesPattern checks a URL path against a glob pattern.
  // * matches any single path segment, ** matches any number of segments.
  private matchesPattern(pattern: string, path: string): boolean {
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

  // shouldRecordCurrentPage checks SDK-local and server rules to decide
  // whether to flush recording events for the current page.
  private shouldRecordCurrentPage(): boolean {
    if (typeof window === 'undefined') return true;
    const path = window.location.pathname;
    const opts = this.recordingOptions;

    // SDK-local include/exclude (checked first, highest priority).
    for (const p of opts.recordExclude ?? []) {
      if (this.matchesPattern(p, path)) return false;
    }
    for (const p of opts.recordInclude ?? []) {
      if (this.matchesPattern(p, path)) return true;
    }

    // Server rules (first match wins).
    for (const rule of this.serverRecordingRules) {
      if (this.matchesPattern(rule.pattern, path)) {
        return rule.action === 'capture';
      }
    }

    return true; // default: capture
  }

  private async flushRecordingChunk(): Promise<void> {
    if (!this.shouldRecordCurrentPage()) {
      // Discard buffered events for this page — don't send them.
      this.rrwebBuffer = [];
      return;
    }
    const events = this.rrwebBuffer.splice(0);
    if (!events.length) return;
    try {
      await fetch(`${this.endpoint}/api/v1/recordings/chunk`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'x-funnelbarn-api-key': this.apiKey,
        },
        body: JSON.stringify({
          recording_id: this.recordingId,
          session_id: this.getOrCreateSessionID(),
          chunk_index: this.recordingChunkIndex++,
          events,
          started_at: this.recordingStartedAt,
          duration_ms: Date.now() - this.recordingStartMs,
          page_url: typeof window !== 'undefined' ? window.location.href : undefined,
        }),
        keepalive: true,
      });
    } catch {
      // best-effort
    }
  }

  private detectURL(): string | undefined {
    if (typeof window !== "undefined" && window.location) {
      return window.location.href;
    }
    return undefined;
  }

  private detectReferrer(): string | undefined {
    if (typeof document !== "undefined" && document.referrer) {
      return document.referrer;
    }
    return undefined;
  }

  private extractUTMs(url?: string): Partial<EventPayload> {
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
}

// --------------------------------------------------------------------------
// Node < 18 HTTP fallback
// --------------------------------------------------------------------------

function sendWithHttp(
  url: string,
  headers: Record<string, string>,
  body: string
): Promise<void> {
  return new Promise((resolve, reject) => {
    try {
      // eslint-disable-next-line @typescript-eslint/no-var-requires
      const protocol = url.startsWith("https") ? require("https") : require("http");
      const parsed = new URL(url);
      const req = protocol.request(
        {
          hostname: parsed.hostname,
          port: parsed.port,
          path: parsed.pathname,
          method: "POST",
          headers,
        },
        (res: { statusCode: number }) => {
          if (res.statusCode && res.statusCode >= 400) {
            reject(new Error(`funnelbarn: server returned ${res.statusCode}`));
          } else {
            resolve();
          }
        }
      );
      req.on("error", reject);
      req.write(body);
      req.end();
    } catch (err) {
      reject(err);
    }
  });
}
