import type { FunnelBarnOptions } from "./types.js";
import { buildTraceparent, parseTraceparent, randomHex } from "./trace-context.js";

/** What trace capture needs from the client that owns the recording. */
export interface TraceHost {
  options: FunnelBarnOptions;
  isRecording(): boolean;
  recordingId(): string;
  record(traceId: string, spanId: string, url: string): void;
}

// shouldPropagate reports whether a traceparent may be injected on this URL.
function shouldPropagate(host: TraceHost, url: string): boolean {
  if (!host.isRecording()) return false;
  if (!host.options.tracePropagate) return false;
  if (typeof window === 'undefined') return false;
  try {
    const u = new URL(url, window.location.href);
    if (u.origin === window.location.origin) return true;
    return (host.options.tracePropagateOrigins ?? []).includes(u.origin);
  } catch {
    return false;
  }
}

function resolveURL(input: unknown): string {
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

function installFetch(host: TraceHost): (() => void) | undefined {
  if (typeof window.fetch !== 'function' || (window.fetch as { __fb?: boolean }).__fb) return undefined;
  const orig = window.fetch.bind(window);
  const wrapped = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    try {
      const url = resolveURL(input);
      let hdrs: Headers;
      if (init?.headers) hdrs = new Headers(init.headers as HeadersInit);
      else if (typeof Request !== 'undefined' && input instanceof Request) hdrs = new Headers(input.headers);
      else hdrs = new Headers();

      const existing = hdrs.get('traceparent');
      if (existing) {
        const p = parseTraceparent(existing);
        if (p) host.record(p.traceId, p.spanId, url);
      } else if (shouldPropagate(host, url)) {
        const traceId = host.recordingId() || randomHex(16);
        const spanId = randomHex(8);
        hdrs.set('traceparent', buildTraceparent(traceId, spanId));
        host.record(traceId, spanId, url);
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
  return () => { window.fetch = orig; };
}

function installXhr(host: TraceHost): (() => void) | undefined {
  if (typeof XMLHttpRequest === 'undefined' || (XMLHttpRequest.prototype.send as { __fb?: boolean }).__fb) return undefined;
  const proto = XMLHttpRequest.prototype;
  const origOpen = proto.open;
  const origSetHeader = proto.setRequestHeader;
  const origSend = proto.send;
  proto.open = function (this: XMLHttpRequest, method: string, url: string | URL, ...rest: unknown[]) {
    (this as { __fbUrl?: string; __fbTp?: string }).__fbUrl = resolveURL(url);
    (this as { __fbTp?: string }).__fbTp = undefined;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return (origOpen as any).call(this, method, url, ...rest);
  };
  proto.setRequestHeader = function (this: XMLHttpRequest, name: string, value: string) {
    if (String(name).toLowerCase() === 'traceparent') {
      (this as { __fbTp?: string }).__fbTp = value;
    }
    return (origSetHeader as typeof proto.setRequestHeader).call(this, name, value);
  };
  const sendWrapped = function (this: XMLHttpRequest, body?: Document | XMLHttpRequestBodyInit | null) {
    try {
      const url = (this as { __fbUrl?: string }).__fbUrl ?? '';
      const tp = (this as { __fbTp?: string }).__fbTp;
      if (tp) {
        const p = parseTraceparent(tp);
        if (p) host.record(p.traceId, p.spanId, url);
      } else if (shouldPropagate(host, url)) {
        const traceId = host.recordingId() || randomHex(16);
        const spanId = randomHex(8);
        (origSetHeader as typeof proto.setRequestHeader).call(this, 'traceparent', buildTraceparent(traceId, spanId));
        host.record(traceId, spanId, url);
      }
    } catch {
      // ignore: never break the app's request
    }
    return (origSend as typeof proto.send).call(this, body ?? null);
  };
  (sendWrapped as { __fb?: boolean }).__fb = true;
  proto.send = sendWrapped;
  return () => {
    proto.open = origOpen;
    proto.setRequestHeader = origSetHeader;
    proto.send = origSend;
  };
}

/**
 * installTraceCapture wraps fetch + XHR to observe the W3C traceparent on
 * outgoing requests while a recording is active. Harvesting is read-only;
 * injection (tracePropagate) only mutates same-origin / allow-listed requests.
 * Returns an uninstall function, or undefined when capture is off or
 * unavailable here.
 */
export function installTraceCapture(host: TraceHost): (() => void) | undefined {
  if (typeof window === 'undefined') return undefined;
  if (host.options.traceCapture === false) return undefined;
  const undoFetch = installFetch(host);
  const undoXhr = installXhr(host);
  return () => {
    undoFetch?.();
    undoXhr?.();
  };
}
