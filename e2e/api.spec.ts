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
  // Helper: log in and return the session context
  async function loginContext(request: import('@playwright/test').APIRequestContext) {
    const loginResp = await request.post('/api/v1/login', {
      data: { username: 'wiebe', password: 'wiebe' },
    })
    expect(loginResp.status()).toBe(200)
    return loginResp
  }

  test('GET /api/v1/projects returns array after login', async ({ request }) => {
    await loginContext(request)
    const resp = await request.get('/api/v1/projects')
    expect(resp.status()).toBe(200)
    const body = await resp.json()
    expect(Array.isArray(body.projects)).toBe(true)
  })

  test('POST /api/v1/projects creates a new project', async ({ request }) => {
    await loginContext(request)
    const slug = `e2e-test-${Date.now()}`
    const resp = await request.post('/api/v1/projects', {
      data: { name: 'E2E Test Project', slug },
    })
    expect(resp.status()).toBe(201)
    const body = await resp.json()
    // The API serialises Go struct fields with their original casing (no json tags on Project)
    expect(body.Slug ?? body.slug).toBe(slug)
    expect(body.Name ?? body.name).toBe('E2E Test Project')
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
