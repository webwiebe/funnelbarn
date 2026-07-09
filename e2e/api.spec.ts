import { test, expect } from '@playwright/test'

/**
 * API-level E2E tests — hit the backend directly via fetch/request.
 * These run against the live test environment (E2E_BASE_URL).
 */

test.describe('API: health', () => {
  test('GET /api/v1/health returns 200 and status ok', async ({ request }) => {
    const resp = await request.get('/api/v1/health')
    expect(resp.status()).toBe(200)
    const body = await resp.json()
    expect(body.status).toBe('ok')
    expect(body.time).toBeTruthy()
  })
})

test.describe('API: auth', () => {
  test('POST /api/v1/login with valid credentials returns session cookie', async ({ request }) => {
    const resp = await request.post('/api/v1/login', {
      data: { username: 'wiebe', password: 'wiebe' },
    })
    expect(resp.status()).toBe(200)
    const body = await resp.json()
    expect(body.username).toBe('wiebe')

    const setCookie = resp.headers()['set-cookie'] ?? ''
    expect(setCookie).toContain('funnelbarn_session')
  })

  test('POST /api/v1/login with bad credentials returns 401', async ({ request }) => {
    const resp = await request.post('/api/v1/login', {
      data: { username: 'wiebe', password: 'wrongpassword' },
    })
    expect(resp.status()).toBe(401)
    const body = await resp.json()
    expect(body.error).toBeTruthy()
  })

  test('GET /api/v1/me without session returns 401', async ({ request }) => {
    const resp = await request.get('/api/v1/me')
    expect(resp.status()).toBe(401)
  })
})

test.describe('API: projects', () => {
  // Helper: log in and return the CSRF token the server set on the response.
  // The server enforces double-submit CSRF on mutating methods (POST/PUT/DELETE/PATCH),
  // so callers need to pass this token as X-FunnelBarn-CSRF for those requests.
  async function loginContext(request: import('@playwright/test').APIRequestContext): Promise<string> {
    const loginResp = await request.post('/api/v1/login', {
      data: { username: 'wiebe', password: 'wiebe' },
    })
    expect(loginResp.status()).toBe(200)
    const setCookies = loginResp.headersArray().filter((h) => h.name.toLowerCase() === 'set-cookie')
    for (const c of setCookies) {
      const match = c.value.match(/funnelbarn_csrf=([^;]+)/)
      if (match) return decodeURIComponent(match[1])
    }
    throw new Error('login did not set funnelbarn_csrf cookie')
  }

  test('GET /api/v1/projects returns array after login', async ({ request }) => {
    await loginContext(request)
    const resp = await request.get('/api/v1/projects')
    expect(resp.status()).toBe(200)
    const body = await resp.json()
    expect(Array.isArray(body.projects)).toBe(true)
  })

  test('POST /api/v1/projects creates a new project', async ({ request }) => {
    const csrf = await loginContext(request)
    const slug = `e2e-test-${Date.now()}`
    const resp = await request.post('/api/v1/projects', {
      headers: { 'X-FunnelBarn-CSRF': csrf },
      data: { name: 'E2E Test Project', slug },
    })
    expect(resp.status()).toBe(201)
    const body = await resp.json()
    // The API serialises Go struct fields with their original casing (no json tags on Project)
    expect(body.Slug ?? body.slug).toBe(slug)
    expect(body.Name ?? body.name).toBe('E2E Test Project')

    // Clean up — delete the project so the dropdown doesn't accumulate duplicates.
    const projectId = body.ID ?? body.id
    if (projectId) {
      await request.delete(`/api/v1/projects/${projectId}`, {
        headers: { 'X-FunnelBarn-CSRF': csrf },
      })
    }
  })
})

test.describe('API: ingest', () => {
  test('POST /api/v1/events with missing API key returns 401', async ({ request }) => {
    const resp = await request.post('/api/v1/events', {
      data: {
        event: 'pageview',
        url: 'https://example.com',
        project: 'test',
      },
    })
    // No API key → should be rejected
    expect([401, 403]).toContain(resp.status())
  })
})

/**
 * CORS configuration guard — validates the deployed edge, not just the handler.
 *
 * This is the regression net for the custom-CNAME ingest break (CORS-1 in #195,
 * fixed in #198): a browser SDK preflights POST /api/v1/events carrying headers
 * we don't control (notably `traceparent` from distributed tracing), and the
 * non-credentialed ingest path MUST echo them back or the browser blocks the
 * real request. It only reproduces in a browser — curl skips preflight — so we
 * pin the actual OPTIONS response the browser would enforce on. Running against
 * the live deployment (E2E_BASE_URL) also catches an edge/proxy that strips or
 * rewrites the CORS headers, which a Go unit test cannot.
 */
test.describe('API: CORS config', () => {
  const CUSTOMER_ORIGIN = 'https://cors-e2e-customer.example'

  test('ingest preflight reflects the origin and echoes requested headers incl. traceparent', async ({
    request,
  }) => {
    const resp = await request.fetch('/api/v1/events', {
      method: 'OPTIONS',
      headers: {
        Origin: CUSTOMER_ORIGIN,
        'Access-Control-Request-Method': 'POST',
        'Access-Control-Request-Headers': 'content-type,traceparent,x-funnelbarn-api-key',
      },
    })
    expect(resp.status()).toBe(204)

    const h = resp.headers()
    // Origin reflected (browser SDKs live on arbitrary customer sites).
    expect(h['access-control-allow-origin']).toBe(CUSTOMER_ORIGIN)
    // The header that regressed — must be echoed back, or the POST is blocked.
    expect((h['access-control-allow-headers'] ?? '').toLowerCase()).toContain('traceparent')
    // Non-credentialed: these auth with an API-key header, never cookies.
    expect(h['access-control-allow-credentials']).toBeUndefined()
  })

  test('credentialed dashboard endpoint does not open CORS to an arbitrary origin', async ({
    request,
  }) => {
    const resp = await request.fetch('/api/v1/projects', {
      method: 'OPTIONS',
      headers: {
        Origin: 'https://cors-e2e-evil.example',
        'Access-Control-Request-Method': 'GET',
        'Access-Control-Request-Headers': 'x-evil-injected',
      },
    })
    const h = resp.headers()
    // An off-allowlist origin must get NO Allow-Origin (credentialed API is not
    // reflected cross-origin) and certainly no reflected injected header. This
    // is the strict posture CORS-1 established and #198 preserved.
    expect(h['access-control-allow-origin']).toBeUndefined()
    expect((h['access-control-allow-headers'] ?? '').toLowerCase()).not.toContain('x-evil-injected')
  })
})
