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
  /**
   * Capture W3C trace context (traceparent) from outgoing fetch/XHR requests
   * during a recording and link it to the session. This is the cross-stack join
   * key: a trace_id seen in SpanBarn/BugBarn resolves back to this recording.
   * Read-only — never modifies requests. Default: true (only active while recording).
   */
  traceCapture?: boolean;
  /**
   * When an outgoing request carries no traceparent, generate one and inject it
   * so an un-instrumented backend's trace still reaches SpanBarn correlatable to
   * this recording. The injected trace_id is the recording_id (deterministic, so
   * it resolves with no lookup). Same-origin requests only, unless an origin is
   * listed in tracePropagateOrigins. Default: false (opt-in — injecting a header
   * cross-origin requires the server to allow it via CORS).
   */
  tracePropagate?: boolean;
  /** Extra origins (besides same-origin) eligible for traceparent injection. */
  tracePropagateOrigins?: string[];
}

/** A W3C trace observed in the browser during a recording. */
export interface TraceLink {
  trace_id: string;
  span_id?: string;
  url?: string;
  occurred_at: string;
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

// Per-tab recording continuity. Persisted to sessionStorage on unload so a
// full-page navigation resumes the same recording instead of starting a new
// one — a visitor's whole journey becomes a single, continuous replay.
const RECORDING_STATE_KEY = "funnelbarn_rec";

interface RecordingState {
  id: string;
  chunkIndex: number;
  startedAt: string;
  elapsedMs: number;
  savedAt: number;
}

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
  private recordingSnapshotFlushed = false;
  private recordingOptions: FunnelBarnOptions;
  private serverRecordingRules: RecordingRule[] = [];

  // trace capture (fetch/XHR instrumentation, active only while recording)
  private traceBuffer: TraceLink[] = [];
  private traceCaptureInstalled = false;
  private origFetch?: typeof fetch;
  private origXhrOpen?: typeof XMLHttpRequest.prototype.open;
  private origXhrSetHeader?: typeof XMLHttpRequest.prototype.setRequestHeader;
  private origXhrSend?: typeof XMLHttpRequest.prototype.send;

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

    // Resume the same recording across a full-page navigation when a recent
    // state was persisted on the previous page's unload. This keeps a visitor's
    // journey as one continuous replay (same recording_id, monotonically
    // increasing chunk_index). The rrweb full snapshot emitted on the new page
    // becomes a later chunk of the same recording, which the player re-anchors
    // on. When there is nothing to resume we start fresh at chunk 0.
    const resumed = this.resumeRecordingState();
    if (resumed) {
      this.recordingId = resumed.id;
      this.recordingChunkIndex = resumed.chunkIndex;
      this.recordingStartedAt = resumed.startedAt;
      this.recordingStartMs = Date.now() - resumed.elapsedMs;
    } else {
      this.recordingId = this.generateSessionID();
      this.recordingStartedAt = new Date().toISOString();
      this.recordingStartMs = Date.now();
      // A fresh recording always starts at chunk 0 with an empty buffer so the
      // rrweb full snapshot (the first emitted event) lands in chunk_index=0.
      this.recordingChunkIndex = 0;
      this.rrwebBuffer = [];
    }
    this.recordingSnapshotFlushed = false;

    this.rrwebStop = record({
      emit: (event) => {
        this.rrwebBuffer.push(event);
        // The full snapshot (type 2) is the one event the player can't replay
        // without. Flush it immediately via a normal fetch (no keepalive size
        // cap) so short visits — ones that never reach the periodic tick or
        // only fire beforeunload — don't silently drop it.
        if (!this.recordingSnapshotFlushed && (event as { type?: number }).type === 2) {
          this.recordingSnapshotFlushed = true;
          this.flushRecordingChunk().catch(() => {});
        }
      },
      maskInputOptions: { password: true },
      blockClass: 'fb-block',
    });
    const interval = chunkMs ?? 10_000;
    this.recordingTimer = setInterval(() => { this.flushRecordingChunk().catch(() => {}); }, interval);
    window.addEventListener('beforeunload', this.handleRecordingUnload);
    // Begin observing outgoing requests' trace context so it links to this recording.
    this.installTraceCapture();
  }

  private handleRecordingUnload = (): void => {
    // Persist continuity state first, then flush the incremental tail. The tail
    // is small, so keepalive (64 KB cap) is safe here; the snapshot already left
    // earlier via a normal fetch.
    this.saveRecordingState();
    this.flushRecordingChunk(true).catch(() => {});
  };

  private saveRecordingState(): void {
    if (typeof sessionStorage === "undefined" || !this.recordingId) return;
    const state: RecordingState = {
      id: this.recordingId,
      chunkIndex: this.recordingChunkIndex,
      startedAt: this.recordingStartedAt,
      elapsedMs: Date.now() - this.recordingStartMs,
      savedAt: Date.now(),
    };
    try {
      sessionStorage.setItem(RECORDING_STATE_KEY, JSON.stringify(state));
    } catch {
      // sessionStorage may be full or unavailable — continuity is best-effort.
    }
  }

  private resumeRecordingState(): RecordingState | null {
    if (typeof sessionStorage === "undefined") return null;
    let raw: string | null;
    try {
      raw = sessionStorage.getItem(RECORDING_STATE_KEY);
    } catch {
      return null;
    }
    if (!raw) return null;
    try {
      const state = JSON.parse(raw) as RecordingState;
      // Discard stale state so a tab left idle past the session window starts a
      // new recording rather than stitching onto an abandoned one.
      if (!state.id || Date.now() - state.savedAt > this.sessionTimeout) {
        return null;
      }
      return state;
    } catch {
      return null;
    }
  }

  private clearRecordingState(): void {
    if (typeof sessionStorage === "undefined") return;
    try {
      sessionStorage.removeItem(RECORDING_STATE_KEY);
    } catch {
      // ignore
    }
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
    if (typeof window !== "undefined") {
      window.removeEventListener('beforeunload', this.handleRecordingUnload);
    }
    this.uninstallTraceCapture();
    this.clearRecordingState();
    this.rrwebBuffer = [];
    this.traceBuffer = [];
    this.recordingChunkIndex = 0;
    this.recordingSnapshotFlushed = false;
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

  // installTraceCapture wraps fetch + XHR to observe the W3C traceparent on
  // outgoing requests while a recording is active. Harvesting is read-only;
  // injection (tracePropagate) only mutates same-origin / allow-listed requests.
  private installTraceCapture(): void {
    if (this.traceCaptureInstalled) return;
    if (typeof window === 'undefined') return;
    if (this.recordingOptions.traceCapture === false) return;
    this.traceCaptureInstalled = true;

    // fetch
    if (typeof window.fetch === 'function' && !(window.fetch as { __fb?: boolean }).__fb) {
      const orig = window.fetch.bind(window);
      this.origFetch = orig;
      const wrapped = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
        try {
          const url = this.resolveURL(input);
          let hdrs: Headers;
          if (init?.headers) hdrs = new Headers(init.headers as HeadersInit);
          else if (typeof Request !== 'undefined' && input instanceof Request) hdrs = new Headers(input.headers);
          else hdrs = new Headers();

          const existing = hdrs.get('traceparent');
          if (existing) {
            const p = parseTraceparent(existing);
            if (p) this.recordTrace(p.traceId, p.spanId, url);
          } else if (this.shouldPropagate(url)) {
            const traceId = this.recordingId || randomHex(16);
            const spanId = randomHex(8);
            hdrs.set('traceparent', buildTraceparent(traceId, spanId));
            this.recordTrace(traceId, spanId, url);
            if (typeof Request !== 'undefined' && input instanceof Request) {
              input = new Request(input, { headers: hdrs });
            } else {
              init = { ...(init || {}), headers: hdrs };
            }
          }
        } catch {
          // Never let trace capture break the app's own request.
        }
        return orig(input, init);
      };
      (wrapped as { __fb?: boolean }).__fb = true;
      window.fetch = wrapped;
    }

    // XMLHttpRequest
    if (typeof XMLHttpRequest !== 'undefined' && !(XMLHttpRequest.prototype.send as { __fb?: boolean }).__fb) {
      const proto = XMLHttpRequest.prototype;
      this.origXhrOpen = proto.open;
      this.origXhrSetHeader = proto.setRequestHeader;
      this.origXhrSend = proto.send;
      const self = this;
      proto.open = function (this: XMLHttpRequest, method: string, url: string | URL, ...rest: unknown[]) {
        (this as { __fbUrl?: string; __fbTp?: string }).__fbUrl = self.resolveURL(url);
        (this as { __fbTp?: string }).__fbTp = undefined;
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        return (self.origXhrOpen as any).call(this, method, url, ...rest);
      };
      proto.setRequestHeader = function (this: XMLHttpRequest, name: string, value: string) {
        if (String(name).toLowerCase() === 'traceparent') {
          (this as { __fbTp?: string }).__fbTp = value;
        }
        return (self.origXhrSetHeader as typeof proto.setRequestHeader).call(this, name, value);
      };
      const sendWrapped = function (this: XMLHttpRequest, body?: Document | XMLHttpRequestBodyInit | null) {
        try {
          const url = (this as { __fbUrl?: string }).__fbUrl ?? '';
          const tp = (this as { __fbTp?: string }).__fbTp;
          if (tp) {
            const p = parseTraceparent(tp);
            if (p) self.recordTrace(p.traceId, p.spanId, url);
          } else if (self.shouldPropagate(url)) {
            const traceId = self.recordingId || randomHex(16);
            const spanId = randomHex(8);
            (self.origXhrSetHeader as typeof proto.setRequestHeader).call(this, 'traceparent', buildTraceparent(traceId, spanId));
            self.recordTrace(traceId, spanId, url);
          }
        } catch {
          // ignore — never break the app's request
        }
        return (self.origXhrSend as typeof proto.send).call(this, body ?? null);
      };
      (sendWrapped as { __fb?: boolean }).__fb = true;
      proto.send = sendWrapped;
    }
  }

  private uninstallTraceCapture(): void {
    if (!this.traceCaptureInstalled) return;
    this.traceCaptureInstalled = false;
    if (this.origFetch && typeof window !== 'undefined') {
      window.fetch = this.origFetch;
      this.origFetch = undefined;
    }
    if (typeof XMLHttpRequest !== 'undefined' && this.origXhrSend) {
      const proto = XMLHttpRequest.prototype;
      if (this.origXhrOpen) proto.open = this.origXhrOpen;
      if (this.origXhrSetHeader) proto.setRequestHeader = this.origXhrSetHeader;
      proto.send = this.origXhrSend;
      this.origXhrOpen = this.origXhrSetHeader = this.origXhrSend = undefined;
    }
  }

  // recordTrace buffers a trace link, but only while a recording is active and
  // capped so a chatty page can't grow the buffer without bound.
  private recordTrace(traceId: string, spanId: string, url: string): void {
    if (!this.rrwebStop) return;
    if (this.traceBuffer.length >= 500) return;
    this.traceBuffer.push({
      trace_id: traceId,
      span_id: spanId || undefined,
      url: url || undefined,
      occurred_at: new Date().toISOString(),
    });
  }

  // shouldPropagate reports whether a traceparent may be injected on this URL.
  private shouldPropagate(url: string): boolean {
    if (!this.rrwebStop) return false;
    if (!this.recordingOptions.tracePropagate) return false;
    if (typeof window === 'undefined') return false;
    try {
      const u = new URL(url, window.location.href);
      if (u.origin === window.location.origin) return true;
      return (this.recordingOptions.tracePropagateOrigins ?? []).includes(u.origin);
    } catch {
      return false;
    }
  }

  private resolveURL(input: unknown): string {
    try {
      if (typeof input === 'string') return input;
      if (typeof URL !== 'undefined' && input instanceof URL) return input.href;
      if (typeof Request !== 'undefined' && input instanceof Request) return input.url;
      if (input && typeof (input as { url?: unknown }).url === 'string') return (input as { url: string }).url;
    } catch {
      // fall through
    }
    return '';
  }

  private async flushRecordingChunk(useKeepalive = false): Promise<void> {
    if (!this.shouldRecordCurrentPage()) {
      this.rrwebBuffer = [];
      return;
    }
    const events = this.rrwebBuffer.splice(0);
    if (!events.length) return;
    const traces = this.traceBuffer.splice(0);
    const chunkIndex = this.recordingChunkIndex++;
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
          chunk_index: chunkIndex,
          events,
          started_at: this.recordingStartedAt,
          duration_ms: Date.now() - this.recordingStartMs,
          page_url: typeof window !== 'undefined' ? window.location.href : undefined,
          traces: traces.length ? traces : undefined,
        }),
        // keepalive has a 64 KB body limit per the Fetch spec — rrweb full
        // snapshots are 1-5 MB so we only use it for the final unload flush
        // where the body is a small incremental tail.
        keepalive: useKeepalive,
      });
    } catch {
      // On failure, push events back to the front so the next flush retries
      // them — most importantly this preserves the full snapshot (chunk 0).
      this.rrwebBuffer.unshift(...events);
      if (traces.length) this.traceBuffer.unshift(...traces);
      this.recordingChunkIndex--;
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
// W3C trace context helpers
// --------------------------------------------------------------------------

/**
 * parseTraceparent validates a W3C `traceparent` header and extracts the
 * trace + span ids. Returns null for malformed or all-zero (invalid) ids.
 * Format: `00-<32 hex trace-id>-<16 hex span-id>-<2 hex flags>`.
 */
export function parseTraceparent(value: string): { traceId: string; spanId: string } | null {
  if (!value) return null;
  const m = /^00-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}$/i.exec(value.trim());
  if (!m) return null;
  const traceId = m[1].toLowerCase();
  const spanId = m[2].toLowerCase();
  if (traceId === '0'.repeat(32) || spanId === '0'.repeat(16)) return null;
  return { traceId, spanId };
}

/** buildTraceparent assembles a sampled (flags=01) W3C traceparent header. */
export function buildTraceparent(traceId: string, spanId: string): string {
  return `00-${traceId}-${spanId}-01`;
}

/** randomHex returns `bytes` cryptographically-random bytes as a lowercase hex string. */
function randomHex(bytes: number): string {
  const arr = new Uint8Array(bytes);
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(arr);
  } else {
    for (let i = 0; i < arr.length; i++) arr[i] = Math.floor(Math.random() * 256);
  }
  return Array.from(arr).map((b) => b.toString(16).padStart(2, '0')).join('');
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
