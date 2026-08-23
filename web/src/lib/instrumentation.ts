// Browser-side tracing for the dashboard.
//
// Each navigation opens a trace. Every fetch made during that navigation
// carries its `traceparent`, so the Go server's spans become children of the
// browser's — one trace in SpanBarn from the click through the API call to the
// database query, instead of two disconnected halves.
//
// Spans are posted to this origin's own /api/v1/telemetry, which relays them
// with credentials that never reach the page. A browser must never hold a
// telemetry key.

const TELEMETRY_ENDPOINT = '/api/v1/telemetry'
const CLIENT_ERRORS_ENDPOINT = '/api/v1/client-errors'
const SERVICE_NAME = 'funnelbarn-web'

// No client-side sampling. SpanBarn samples again at ingest — it buffers each
// trace and keeps it only if the trace errored or its ID falls in
// 1-in-ingest.sample_ratio (default 1000, i.e. 0.1%). Sampling here too made
// the two multiply: a 5% client rate against that 0.1% left roughly 1 trace in
// 20,000, which on a dashboard with a handful of daily visits means you never
// see one. SpanBarn is the single sampling authority; adjust
// `ingest.sample_ratio.project.<id>` there to see more.
const FLUSH_INTERVAL_MS = 5000
/** Flush threshold, to keep batches small. */
const FLUSH_AT_SPANS = 25
/** Hard ceiling so a long-lived page can't grow the buffer without bound. */
const MAX_BUFFERED_SPANS = 100

export type SpanKind = 'INTERNAL' | 'CLIENT' | 'SERVER'
export type SpanStatus = 'OK' | 'ERROR'

export interface SpanPayload {
  traceId: string
  spanId: string
  parentSpanId?: string
  name: string
  service: string
  kind: SpanKind
  status: SpanStatus
  /** Microseconds since the epoch. */
  startTime: number
  /** Microseconds. */
  duration: number
  attributes: Record<string, string | number | boolean>
}

const spanQueue: SpanPayload[] = []
let flushTimer: ReturnType<typeof setInterval> | null = null

let pageTraceId = ''
let pageSpanId = ''
let pendingPageSpan: SpanPayload | null = null
let traceHasError = false

// ---------------------------------------------------------------------------
// Primitives
// ---------------------------------------------------------------------------

function hex(bytes: number): string {
  const arr = crypto.getRandomValues(new Uint8Array(bytes))
  return Array.from(arr, (b) => b.toString(16).padStart(2, '0')).join('')
}

function nowUs(): number {
  return Math.round(performance.timeOrigin * 1000 + performance.now() * 1000)
}

export function traceparent(traceId: string, spanId: string): string {
  return `00-${traceId}-${spanId}-01`
}

/** The current page trace's ID, for correlating an error report with the trace. */
export function currentTraceId(): string {
  return pageTraceId
}

/**
 * Marks the current trace as containing an error. SpanBarn keeps error traces
 * unconditionally, and this also flushes the buffer immediately so the trace
 * survives the page closing.
 */
export function markTraceError(): void {
  traceHasError = true
}

// ---------------------------------------------------------------------------
// Buffering
// ---------------------------------------------------------------------------

function getCookie(name: string): string {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'))
  return match ? decodeURIComponent(match[2]) : ''
}

function sendBatch(spans: SpanPayload[]): void {
  if (spans.length === 0) return
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  // The endpoint is session-authed, so mutating requests carry CSRF like the
  // rest of the API client.
  const csrf = getCookie('funnelbarn_csrf')
  if (csrf) headers['X-FunnelBarn-CSRF'] = csrf

  // originalFetch, not window.fetch: the instrumented wrapper would trace its
  // own delivery and never settle.
  originalFetch(TELEMETRY_ENDPOINT, {
    method: 'POST',
    headers,
    body: JSON.stringify({ spans }),
    credentials: 'same-origin',
    keepalive: true,
  }).catch(() => {})
}

function finalizePageSpan(): void {
  if (!pendingPageSpan) return
  pendingPageSpan.duration = nowUs() - pendingPageSpan.startTime
  spanQueue.push(pendingPageSpan)
  pendingPageSpan = null
}

/** Ends the current page trace and sends whatever it collected. */
function commitTrace(): void {
  finalizePageSpan()
  if (spanQueue.length === 0) return
  sendBatch(spanQueue.splice(0))
}

function startPageTrace(path: string, fromPath?: string): void {
  commitTrace()

  pageTraceId = hex(16)
  pageSpanId = hex(8)
  traceHasError = false

  const attributes: Record<string, string | number | boolean> = { 'navigation.to': path }
  if (fromPath) attributes['navigation.from'] = fromPath

  pendingPageSpan = {
    traceId: pageTraceId,
    spanId: pageSpanId,
    name: `page ${path}`,
    service: SERVICE_NAME,
    kind: 'INTERNAL',
    status: 'OK',
    startTime: nowUs(),
    duration: 0,
    attributes,
  }
}

function enqueueSpan(span: SpanPayload): void {
  if (span.status === 'ERROR') traceHasError = true
  spanQueue.push(span)

  // Send immediately on an error so the whole trace — including the spans
  // buffered before it — reaches SpanBarn while the page may still be closing.
  if (traceHasError) {
    finalizePageSpan()
    sendBatch(spanQueue.splice(0))
    return
  }
  if (spanQueue.length >= FLUSH_AT_SPANS) {
    flushSpans()
    return
  }
  if (spanQueue.length >= MAX_BUFFERED_SPANS) spanQueue.length = 0
}

function flushSpans(): void {
  finalizePageSpan()
  if (spanQueue.length === 0) return
  sendBatch(spanQueue.splice(0, 50))
}

// ---------------------------------------------------------------------------
// Error reporting
// ---------------------------------------------------------------------------

/**
 * Reports a failure the browser saw, tagged with the current trace so the
 * BugBarn issue and the SpanBarn trace point at each other.
 */
export function reportClientError(error: Error, extra?: Record<string, string>): void {
  markTraceError()
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const csrf = getCookie('funnelbarn_csrf')
  if (csrf) headers['X-FunnelBarn-CSRF'] = csrf

  originalFetch(CLIENT_ERRORS_ENDPOINT, {
    method: 'POST',
    headers,
    credentials: 'same-origin',
    keepalive: true,
    body: JSON.stringify({
      message: error.message,
      type: extra?.type ?? error.name ?? 'Error',
      stack: error.stack ?? '',
      url: location.href,
      trace_id: pageTraceId,
    }),
  }).catch(() => {})
}

// ---------------------------------------------------------------------------
// Fetch instrumentation
// ---------------------------------------------------------------------------

// Captured before wrapping so telemetry delivery is never itself traced —
// an instrumented delivery would trace its own delivery and never settle.
let originalFetch: typeof fetch = () =>
  Promise.reject(new Error('instrumentation: fetch unavailable'))

function isInstrumentationUrl(url: string): boolean {
  return url.includes(TELEMETRY_ENDPOINT) || url.includes(CLIENT_ERRORS_ENDPOINT)
}

export function requestURL(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

function spanName(method: string, url: string): string {
  try {
    return `${method} ${new URL(url, location.origin).pathname}`
  } catch {
    return `${method} ${url}`
  }
}

function instrumentFetch(): void {
  originalFetch = window.fetch.bind(window)

  window.fetch = function instrumentedFetch(
    input: RequestInfo | URL,
    init?: RequestInit,
  ): Promise<Response> {
    const url = requestURL(input)
    if (isInstrumentationUrl(url)) return originalFetch(input, init)

    const method = (init?.method ?? 'GET').toUpperCase()
    const spanId = hex(8)
    const startTime = nowUs()

    // Injecting traceparent is what joins the halves: the Go middleware
    // extracts it and its server span becomes a child of this one.
    const headers = new Headers(init?.headers)
    if (pageTraceId) headers.set('traceparent', traceparent(pageTraceId, spanId))

    const record = (status: SpanStatus, attributes: Record<string, string | number | boolean>) => {
      enqueueSpan({
        traceId: pageTraceId,
        spanId,
        parentSpanId: pageSpanId,
        name: spanName(method, url),
        service: SERVICE_NAME,
        kind: 'CLIENT',
        status,
        startTime,
        duration: nowUs() - startTime,
        attributes: { 'http.method': method, 'http.url': url, ...attributes },
      })
    }

    return originalFetch(input, { ...init, headers }).then(
      (response) => {
        record(response.ok ? 'OK' : 'ERROR', { 'http.status_code': response.status })
        // A 5xx is a server defect the user just hit. The app may handle it
        // gracefully, which is exactly why nothing else would report it.
        if (response.status >= 500) {
          reportClientError(new Error(`HTTP ${response.status} ${method} ${url}`), {
            type: 'http_error',
          })
        }
        return response
      },
      (error: unknown) => {
        const message = error instanceof Error ? error.message : String(error)
        record('ERROR', { 'error.message': message })
        throw error
      },
    )
  }
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

function instrumentNavigation(): void {
  let lastPath = location.pathname
  startPageTrace(lastPath)

  const originalPushState = history.pushState.bind(history)
  const originalReplaceState = history.replaceState.bind(history)

  const onNav = () => {
    if (location.pathname === lastPath) return
    const fromPath = lastPath
    lastPath = location.pathname
    startPageTrace(lastPath, fromPath)
  }

  history.pushState = function (...args: Parameters<typeof history.pushState>) {
    originalPushState(...args)
    onNav()
  }
  history.replaceState = function (...args: Parameters<typeof history.replaceState>) {
    originalReplaceState(...args)
    onNav()
  }
  window.addEventListener('popstate', onNav)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

export function initInstrumentation(): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return
  instrumentFetch()
  instrumentNavigation()
  flushTimer = setInterval(flushSpans, FLUSH_INTERVAL_MS)

  // A backgrounded tab may never come back, so commit what we have. This is
  // the only reliable "page is going away" signal across browsers.
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') commitTrace()
  })
}

export function shutdownInstrumentation(): void {
  if (flushTimer) {
    clearInterval(flushTimer)
    flushTimer = null
  }
  commitTrace()
}

/** Test seam: resets module state between cases. */
export function __resetForTests(fetchImpl?: typeof fetch): void {
  spanQueue.length = 0
  pendingPageSpan = null
  pageTraceId = ''
  pageSpanId = ''
  traceHasError = false
  if (flushTimer) {
    clearInterval(flushTimer)
    flushTimer = null
  }
  if (fetchImpl) originalFetch = fetchImpl
}

/** Test seam: the spans currently buffered. */
export function __bufferedSpans(): SpanPayload[] {
  return [...spanQueue]
}
