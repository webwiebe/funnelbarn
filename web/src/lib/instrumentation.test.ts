import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import {
  initInstrumentation, shutdownInstrumentation, shouldSampleTrace, traceparent,
  requestURL, currentTraceId, markTraceError, __resetForTests, __bufferedSpans,
  type SpanPayload,
} from './instrumentation'

type Sent = { url: string; body: { spans?: SpanPayload[]; message?: string; trace_id?: string } }

let sent: Sent[]
let realFetch: typeof fetch

/** installFetch replaces window.fetch with a recorder and returns it. */
function installFetch(responder: (url: string) => Response = () => new Response('{}', { status: 200 })) {
  const impl = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = requestURL(input)
    if (url.startsWith('/api/v1/telemetry') || url.startsWith('/api/v1/client-errors')) {
      sent.push({ url, body: JSON.parse(String(init?.body ?? '{}')) })
      return new Response('', { status: 202 })
    }
    return responder(url)
  })
  window.fetch = impl as unknown as typeof fetch
  return impl
}

beforeEach(() => {
  sent = []
  realFetch = window.fetch
  document.cookie = 'funnelbarn_csrf=tok'
  window.history.replaceState({}, '', '/overview')
  __resetForTests()
})

afterEach(() => {
  shutdownInstrumentation()
  window.fetch = realFetch
})

describe('traceparent', () => {
  it('builds a sampled W3C header', () => {
    expect(traceparent('4bf92f3577b34da6a3ce929d0e0e4736', '00f067aa0ba902b7'))
      .toBe('00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01')
  })
})

describe('shouldSampleTrace', () => {
  it('is deterministic — the same trace ID always decides the same way', () => {
    const id = 'ff'.repeat(16)
    expect(shouldSampleTrace(id, 50)).toBe(shouldSampleTrace(id, 50))
  })

  it('keeps everything at 100 and nothing at 0', () => {
    const id = '0123456789abcdef' + '0'.repeat(16)
    expect(shouldSampleTrace(id, 100)).toBe(true)
    expect(shouldSampleTrace(id, 0)).toBe(false)
  })

  it('drops rather than throwing on a malformed trace ID', () => {
    expect(shouldSampleTrace('', 100)).toBe(false)
    expect(shouldSampleTrace('zzzz', 100)).toBe(false)
  })
})

describe('fetch instrumentation', () => {
  it('injects traceparent so the server span joins the browser trace', async () => {
    let seen: string | null = null
    installFetch()
    const inner = window.fetch
    window.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input)
      if (!url.startsWith('/api/v1/telemetry') && !url.startsWith('/api/v1/client-errors')) {
        seen = new Headers(init?.headers).get('traceparent')
      }
      return inner(input, init)
    }) as typeof fetch

    initInstrumentation()
    await window.fetch('/api/v1/funnels')

    expect(seen).toMatch(/^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/)
    expect(seen).toContain(currentTraceId())
  })

  it('records a CLIENT span parented to the page span', async () => {
    installFetch()
    initInstrumentation()
    await window.fetch('/api/v1/funnels')

    const spans = __bufferedSpans()
    const client = spans.find((s) => s.kind === 'CLIENT')
    expect(client).toBeDefined()
    expect(client!.name).toBe('GET /api/v1/funnels')
    expect(client!.service).toBe('funnelbarn-web')
    expect(client!.status).toBe('OK')
    expect(client!.parentSpanId).toBeTruthy()
    expect(client!.traceId).toBe(currentTraceId())
    expect(client!.attributes['http.status_code']).toBe(200)
  })

  it('never traces its own telemetry delivery', async () => {
    installFetch()
    initInstrumentation()
    markTraceError() // force a flush
    await window.fetch('/api/v1/funnels')

    for (const s of __bufferedSpans()) {
      expect(s.name).not.toContain('/api/v1/telemetry')
      expect(s.name).not.toContain('/api/v1/client-errors')
    }
  })

  it('marks a failed response ERROR and reports a 5xx nothing else would see', async () => {
    installFetch((url) =>
      url === '/api/v1/boom' ? new Response('', { status: 503 }) : new Response('{}', { status: 200 }))
    initInstrumentation()
    await window.fetch('/api/v1/boom')

    const errors = sent.filter((s) => s.url === '/api/v1/client-errors')
    expect(errors).toHaveLength(1)
    expect(errors[0].body.message).toContain('HTTP 503')
    expect(errors[0].body.trace_id).toBe(currentTraceId())
  })

  it('re-throws a network failure after recording it', async () => {
    installFetch()
    const inner = window.fetch
    window.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input)
      if (url === '/api/v1/down') throw new TypeError('Failed to fetch')
      return inner(input, init)
    }) as typeof fetch

    initInstrumentation()
    await expect(window.fetch('/api/v1/down')).rejects.toThrow('Failed to fetch')

    // The error flushes the trace immediately so it survives the page closing.
    const batches = sent.filter((s) => s.url === '/api/v1/telemetry')
    const spans = batches.flatMap((b) => b.body.spans ?? [])
    const failed = spans.find((s) => s.name === 'GET /api/v1/down')
    expect(failed?.status).toBe('ERROR')
    expect(failed?.attributes['error.message']).toBe('Failed to fetch')
  })

  it('sends the CSRF header the session-authed endpoint requires', async () => {
    const impl = installFetch()
    initInstrumentation()
    markTraceError()
    await window.fetch('/api/v1/funnels')

    const call = impl.mock.calls.find(([input]) => requestURL(input as RequestInfo) === '/api/v1/telemetry')
    expect(call).toBeDefined()
    expect((call![1] as RequestInit).headers).toMatchObject({ 'X-FunnelBarn-CSRF': 'tok' })
  })
})

describe('navigation', () => {
  it('opens a new trace per navigation', async () => {
    installFetch()
    initInstrumentation()
    const first = currentTraceId()
    expect(first).toMatch(/^[0-9a-f]{32}$/)

    window.history.pushState({}, '', '/funnels')
    expect(currentTraceId()).not.toBe(first)
  })

  it('ignores a pushState that does not change the path', () => {
    installFetch()
    initInstrumentation()
    const first = currentTraceId()
    window.history.pushState({}, '', '/overview?tab=2')
    expect(currentTraceId()).toBe(first)
  })
})

describe('sampling', () => {
  it('drops an unsampled, error-free trace whole rather than in part', async () => {
    installFetch()
    initInstrumentation()
    await window.fetch('/api/v1/funnels')

    // Force the commit path without an error. Whether this trace was sampled
    // is decided by its ID, so assert the invariant: a batch is either sent
    // in full or not at all — never a partial trace.
    const before = sent.length
    shutdownInstrumentation()
    const batches = sent.slice(before).filter((s) => s.url === '/api/v1/telemetry')
    if (batches.length > 0) {
      const spans = batches.flatMap((b) => b.body.spans ?? [])
      expect(spans.some((s) => s.kind === 'INTERNAL')).toBe(true) // the page span came too
    }
    expect(__bufferedSpans()).toHaveLength(0)
  })

  it('always keeps a trace that hit an error', async () => {
    installFetch((url) =>
      url === '/api/v1/boom' ? new Response('', { status: 500 }) : new Response('{}', { status: 200 }))
    initInstrumentation()
    await window.fetch('/api/v1/boom')

    const spans = sent.filter((s) => s.url === '/api/v1/telemetry').flatMap((b) => b.body.spans ?? [])
    expect(spans.some((s) => s.status === 'ERROR')).toBe(true)
  })
})

describe('requestURL', () => {
  it('reads the URL from every fetch input shape', () => {
    expect(requestURL('/a')).toBe('/a')
    expect(requestURL(new URL('https://x.test/b'))).toBe('https://x.test/b')
    expect(requestURL(new Request('https://x.test/c'))).toBe('https://x.test/c')
  })
})
