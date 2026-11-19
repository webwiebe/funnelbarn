/**
 * FunnelBarn browser + Node.js SDK.
 *
 * Tracks page views and custom events. Batches events and flushes every
 * 5 seconds or on beforeunload. Generates a session ID in localStorage
 * with a 30-minute idle timeout (browser only).
 */

import { record } from "rrweb";
import { isAutomatedBrowser, observePerformance } from "./browser.js";
import { RECORDING_CHUNK_BUDGET_BYTES, sendInOrder, splitByBudget } from "./recording-chunks.js";
import { SESSION_MAX_AGE_DEFAULT, SESSION_TIMEOUT_DEFAULT } from "./types.js";
import type { EventPayload, EventProperties, FunnelBarnOptions, RecordingRule, SessionSignals, TraceLink } from "./types.js";
import {
  collectSessionSignals, detectReferrer, detectURL, extractUTMs, generateSessionID, generateUUID,
  normaliseEndpoint, shouldRecordPath,
} from "./helpers.js";
import { clearRecordingState, resumeRecordingState, saveRecordingState } from "./recording-state.js";
import { installTraceCapture } from "./trace-capture.js";
import { postJSON } from "./http-fallback.js";
import { EngagementTracker } from "./engagement.js";
import { getOrCreateSessionID } from "./session.js";

export type { EventProperties, FunnelBarnOptions, RecordingRule, TraceLink } from "./types.js";
export { buildTraceparent, parseTraceparent } from "./trace-context.js";

export class FunnelBarnClient {
  private readonly apiKey: string;
  private readonly endpoint: string;
  private readonly projectName?: string;
  private readonly environment?: string;
  private readonly flushInterval: number;
  private readonly sessionTimeout: number;
  private readonly sessionMaxAge: number;

  private queue: EventPayload[] = [];
  private flushTimer?: ReturnType<typeof setInterval>;
  private userId?: string;

  // page_view_id
  private currentPageViewId: string | undefined;

  // session signals
  private sessionSignalsSent = false;

  // engagement tracking
  private readonly engagement = new EngagementTracker((url, pageViewId) => {
    this.enqueue({
      name: "page_engaged",
      url,
      page_view_id: pageViewId,
      session_id: this.getOrCreateSessionID(),
      timestamp: new Date().toISOString(),
    });
  });

  // web vitals
  private pendingVitals: Record<string, number> = {};
  private vitalsPageViewId: string | undefined;
  // Captured at page(): vitals can flush after an SPA navigation (#296).
  private vitalsPageURL: string | undefined;
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
  private uninstallTrace?: () => void;

  constructor(options: FunnelBarnOptions) {
    this.apiKey = options.apiKey;
    this.endpoint = normaliseEndpoint(options.endpoint);
    this.projectName = options.projectName;
    this.environment = isAutomatedBrowser() ? "test" : options.environment;
    this.flushInterval = options.flushInterval ?? 5000;
    this.sessionTimeout = options.sessionTimeout ?? SESSION_TIMEOUT_DEFAULT;
    this.sessionMaxAge = options.sessionMaxAge ?? SESSION_MAX_AGE_DEFAULT;
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
        this.enqueue({
          name: "web_vitals",
          url: this.vitalsPageURL,
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

      observePerformance("largest-contentful-paint", (list) => {
        const entries = list.getEntries();
        if (entries.length) this.pendingVitals.lcp = Math.round(entries[entries.length - 1].startTime);
      });
      observePerformance("paint", (list) => {
        const e = list.getEntriesByName("first-contentful-paint")[0];
        if (e) this.pendingVitals.fcp = Math.round(e.startTime);
      });
    }
  }

  /**
   * Track a page view. Auto-detects URL and referrer from window.location
   * when running in a browser.
   */
  page(properties?: EventProperties): void {
    // Generate a new page_view_id for this page view.
    this.currentPageViewId = generateUUID();

    // Clean up engagement tracking from any previous page view.
    this.engagement.cleanup();

    const url = detectURL();

    // Reset web vitals state for this page view.
    this.pendingVitals = {};
    this.vitalsPageViewId = this.currentPageViewId;
    this.vitalsPageURL = url;
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
      sessionSignals = collectSessionSignals();
    }

    const referrer = detectReferrer();
    const utms = extractUTMs(url);

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

    this.enqueue(payload);

    // Start engagement tracking for this page view.
    this.engagement.start(url, this.currentPageViewId);
  }

  /**
   * Queue an event, stamping the configured environment on it.
   *
   * Every enqueue goes through here so no event can escape untagged — an
   * option that only reaches some events is worse than no option at all,
   * because the dashboard filter then silently under-counts.
   */
  private enqueue(payload: EventPayload): void {
    if (this.environment) payload.environment = this.environment;
    this.queue.push(payload);
  }

  /**
   * Track a custom event.
   */
  track(name: string, properties?: EventProperties): void {
    if (!name) return;
    const url = detectURL();
    const utms = extractUTMs(url);

    this.enqueue({
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
    await postJSON(`${this.endpoint}/api/v1/events`, this.requestHeaders(), JSON.stringify(event));
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
    return getOrCreateSessionID(this.sessionTimeout, this.sessionMaxAge);
  }


  private startRecording(chunkMs?: number): void {
    if (this.rrwebStop) return; // already running

    // Resume the same recording across a full-page navigation when a recent
    // state was persisted on the previous page's unload. This keeps a visitor's
    // journey as one continuous replay (same recording_id, monotonically
    // increasing chunk_index). The rrweb full snapshot emitted on the new page
    // becomes a later chunk of the same recording, which the player re-anchors
    // on. When there is nothing to resume we start fresh at chunk 0.
    const resumed = resumeRecordingState(this.sessionTimeout);
    if (resumed) {
      this.recordingId = resumed.id;
      this.recordingChunkIndex = resumed.chunkIndex;
      this.recordingStartedAt = resumed.startedAt;
      this.recordingStartMs = Date.now() - resumed.elapsedMs;
    } else {
      this.recordingId = generateSessionID();
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
    saveRecordingState({
      id: this.recordingId,
      chunkIndex: this.recordingChunkIndex,
      startedAt: this.recordingStartedAt,
      elapsedMs: Date.now() - this.recordingStartMs,
      savedAt: Date.now(),
    });
    this.flushRecordingChunk(true).catch(() => {});
  };


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
    clearRecordingState();
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

  // shouldRecordCurrentPage checks SDK-local and server rules to decide
  // whether to flush recording events for the current page.
  private shouldRecordCurrentPage(): boolean {
    if (typeof window === 'undefined') return true;
    return shouldRecordPath(window.location.pathname, this.recordingOptions, this.serverRecordingRules);
  }


  // installTraceCapture starts observing outgoing requests' trace context
  // while a recording is active (see ./trace-capture.ts).
  private installTraceCapture(): void {
    if (this.traceCaptureInstalled) return;
    if (typeof window === 'undefined') return;
    if (this.recordingOptions.traceCapture === false) return;
    this.traceCaptureInstalled = true;
    this.uninstallTrace = installTraceCapture({
      options: this.recordingOptions,
      isRecording: () => !!this.rrwebStop,
      recordingId: () => this.recordingId,
      record: (traceId, spanId, url) => this.recordTrace(traceId, spanId, url),
    });
  }

  private uninstallTraceCapture(): void {
    if (!this.traceCaptureInstalled) return;
    this.traceCaptureInstalled = false;
    this.uninstallTrace?.();
    this.uninstallTrace = undefined;
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

  private requestHeaders(): Record<string, string> { // events and recording chunks
    const headers: Record<string, string> = { "Content-Type": "application/json", "x-funnelbarn-api-key": this.apiKey };
    if (this.projectName) headers["x-funnelbarn-project"] = this.projectName;
    return headers;
  }

  private async flushRecordingChunk(useKeepalive = false): Promise<void> {
    if (!this.shouldRecordCurrentPage()) {
      this.rrwebBuffer = [];
      return;
    }
    const events = this.rrwebBuffer.splice(0);
    if (!events.length) return;
    const traces = this.traceBuffer.splice(0);
    const groups = splitByBudget(events, RECORDING_CHUNK_BUDGET_BYTES);
    // Claim every index before the first await, so a flush that overlaps this
    // one cannot slot its chunk between these.
    const first = this.recordingChunkIndex;
    this.recordingChunkIndex += groups.length;
    const sent = await sendInOrder(groups, (chunk, g) => fetch(`${this.endpoint}/api/v1/recordings/chunk`, {
      method: 'POST',
      headers: this.requestHeaders(),
      body: JSON.stringify({
        recording_id: this.recordingId, environment: this.environment,
        session_id: this.getOrCreateSessionID(),
        chunk_index: first + g,
        events: chunk,
        started_at: this.recordingStartedAt,
        duration_ms: Date.now() - this.recordingStartMs,
        page_url: typeof window !== 'undefined' ? window.location.href : undefined,
        traces: g === 0 && traces.length ? traces : undefined,
      }),
      // keepalive has a 64 KB body limit per the Fetch spec — rrweb full
      // snapshots are 1-5 MB so we only use it for the final unload flush
      // where the body is a small incremental tail.
      keepalive: useKeepalive,
    }));
    if (sent === groups.length) return;
    // On failure, push the unsent events back to the front so the next flush
    // retries them — most importantly this preserves the full snapshot (chunk 0).
    this.rrwebBuffer.unshift(...groups.slice(sent).flat());
    if (sent === 0 && traces.length) this.traceBuffer.unshift(...traces);
    if (this.recordingChunkIndex === first + groups.length) this.recordingChunkIndex = first + sent;
  }

}
