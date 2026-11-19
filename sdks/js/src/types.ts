// Public option and payload shapes for the SDK, plus the storage keys and
// defaults the session and recording code share.

export interface RecordingRule {
  pattern: string;
  action: 'capture' | 'ignore';
}

export interface FunnelBarnOptions {
  apiKey: string;
  endpoint: string;
  /** Optional project name sent as x-funnelbarn-project header */
  projectName?: string;
  /**
   * Deployment environment tagged on every event, so one API key and project
   * can be reused across dev / staging / production and filtered apart in the
   * dashboard. Aliases are normalised server-side ("prod" and "live" both mean
   * production); an unrecognised value is filed as development, so a typo is
   * worth avoiding. A browser under automation (navigator.webdriver) sends
   * "test" whatever this says.
   */
  environment?: string;
  /** Flush interval in ms (default: 5000) */
  flushInterval?: number;
  /** Session idle timeout in ms (default: 30 minutes) */
  sessionTimeout?: number;
  /**
   * Hard cap on how long one session ID may live, regardless of activity
   * (default: 24 hours). The idle timeout alone never fires for a tab that
   * stays busy — a kiosk, a dashboard left open, a page with a polling
   * heartbeat — so without this a single session ID can run for months.
   */
  sessionMaxAge?: number;
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

export interface SessionSignals {
  screen_width?: number;
  screen_height?: number;
  pixel_ratio?: number;
  touch?: boolean;
  dark_mode?: boolean;
  reduced_motion?: boolean;
  browser_timezone?: string;
  cpu_cores?: number;
}

export interface EventPayload {
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
  environment?: string;
}

export const SESSION_KEY = "funnelbarn_sid";
export const SESSION_EXPIRY_KEY = "funnelbarn_sid_exp";
export const SESSION_STARTED_KEY = "funnelbarn_sid_start";
export const SESSION_TIMEOUT_DEFAULT = 30 * 60 * 1000; // 30 min
export const SESSION_MAX_AGE_DEFAULT = 24 * 60 * 60 * 1000; // 24 h

// Per-tab recording continuity. Persisted to sessionStorage on unload so a
// full-page navigation resumes the same recording instead of starting a new
// one — a visitor's whole journey becomes a single, continuous replay.
export const RECORDING_STATE_KEY = "funnelbarn_rec";

export interface RecordingState {
  id: string;
  chunkIndex: number;
  startedAt: string;
  elapsedMs: number;
  savedAt: number;
}
