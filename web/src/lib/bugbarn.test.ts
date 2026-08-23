import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { ApiError } from './api-error'

/**
 * bugbarn.ts fetches its config on import, so each case re-imports the module
 * with a stubbed fetch and lets that settle before asserting.
 */
async function loadReporter(): Promise<typeof import('./bugbarn')> {
  vi.resetModules()
  const mod = await import('./bugbarn')
  // Let the config fetch resolve so reports are sent rather than queued.
  await new Promise((r) => setTimeout(r, 0))
  return mod
}

let posts: { url: string; body: Record<string, unknown> }[]
let realFetch: typeof fetch

beforeEach(() => {
  posts = []
  realFetch = globalThis.fetch
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    if (url.includes('/api/v1/client-config')) {
      return new Response(JSON.stringify({
        bugbarn_endpoint: 'https://bugbarn.test',
        bugbarn_ingest_key: 'k',
        bugbarn_project: 'funnelbarn',
      }), { status: 200 })
    }
    posts.push({ url, body: JSON.parse(String(init?.body ?? '{}')) })
    return new Response('', { status: 202 })
  }) as unknown as typeof fetch
})

afterEach(() => {
  globalThis.fetch = realFetch
  vi.restoreAllMocks()
})

describe('reportError', () => {
  it('reports a real application error', async () => {
    const { reportError } = await loadReporter()
    reportError(new TypeError("Cannot read properties of null (reading 'map')"), {
      source: 'Insights',
    })
    expect(posts).toHaveLength(1)
    expect(posts[0].body).toMatchObject({ name: 'error' })
  })

  it('does not report a dropped connection — the user’s network, not our code', async () => {
    const { reportError } = await loadReporter()
    // FUN-6: Dashboard.getDashboard reported ApiError(0) because the noise
    // filter lived in ProjectProvider instead of at the reporting boundary.
    reportError(new ApiError(0, 'Failed to fetch'), { source: 'Dashboard.getDashboard' })
    expect(posts).toHaveLength(0)
  })

  it('does not report an expired session', async () => {
    const { reportError } = await loadReporter()
    reportError(new ApiError(401, 'unauthorized'), { source: 'anything' })
    expect(posts).toHaveLength(0)
  })

  it('still reports a server fault', async () => {
    const { reportError } = await loadReporter()
    reportError(new ApiError(500, 'internal'), { source: 'anything' })
    expect(posts).toHaveLength(1)
  })
})
