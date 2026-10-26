import { describe, it, expect, vi, afterEach } from 'vitest'
import { getFunnelBreakdown } from './api.funnelbreakdown'

const breakdown = { property: 'plan', groups: [{ value: 'pro', label: 'pro', entries: 3, results: [] }] }

function mockFetch() {
  const fetch = vi.fn().mockResolvedValue({
    status: 200,
    ok: true,
    text: () => Promise.resolve(JSON.stringify({ breakdown })),
    json: () => Promise.resolve({ breakdown }),
    headers: new Headers(),
  })
  globalThis.fetch = fetch
  return fetch
}

// The query string of the URL the first fetch call went to.
function requestedParams(fetch: ReturnType<typeof mockFetch>) {
  const url = new URL(fetch.mock.calls[0][0] as string, 'http://x')
  return { path: url.pathname, params: url.searchParams }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('getFunnelBreakdown', () => {
  it('requests the funnel analysis with the breakdown property and returns the breakdown', async () => {
    const fetch = mockFetch()
    const result = await getFunnelBreakdown('p1', 'f1', 'plan')
    const { path, params } = requestedParams(fetch)
    expect(path).toBe('/api/v1/projects/p1/funnels/f1/analysis')
    expect(params.toString()).toBe('breakdown=plan')
    expect(result).toEqual(breakdown)
  })

  it('leaves out the all segment', async () => {
    const fetch = mockFetch()
    await getFunnelBreakdown('p1', 'f1', 'plan', { segment: 'all' })
    expect(requestedParams(fetch).params.has('segment')).toBe(false)
  })

  it('passes segment, stored segment and environment', async () => {
    const fetch = mockFetch()
    await getFunnelBreakdown('p1', 'f1', 'plan', { segment: 'mobile', segmentId: 's9', environment: 'production' })
    const { params } = requestedParams(fetch)
    expect(params.get('segment')).toBe('mobile')
    expect(params.get('segment_id')).toBe('s9')
    expect(params.get('environment')).toBe('production')
  })
})
