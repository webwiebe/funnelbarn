import { test, expect } from '@playwright/test'

// FunnelBarn is a standard OIDC relying party: ONE confidential-client flow
// with PKCE at /api/v1/oidc/{login,callback}, token-bound server-side
// sessions, and server-driven logout. These specs run against a deployed
// environment with FUNNELBARN_OIDC_* configured.

// ── API-level tests (no browser, run in the "api" project) ──────────────────

test.describe('API: OIDC client-config', () => {
  test('exposes oidc login + iambarn widget fields', async ({ request }) => {
    const resp = await request.get('/api/v1/client-config')
    expect(resp.status()).toBe(200)
    const body = await resp.json()
    expect(body.oidc?.enabled).toBe(true)
    expect(body.oidc?.loginURL).toBe('/api/v1/oidc/login')
    // The hosted IAMBarn widget needs issuer + client id + post-logout URI.
    expect(body.iambarn?.server_url).toContain('iam.')
    expect(body.iambarn?.client_id).toBeTruthy()
    expect(body.iambarn?.widget_url).toContain('/widget/iambarn-widget.iife.js')
    expect(body.iambarn?.post_logout_redirect_uri).toContain('/api/v1/auth/oidc/logged-out')
  })
})

test.describe('API: OIDC login redirect', () => {
  test('redirects to the issuer with PKCE + offline_access', async ({ request }) => {
    const resp = await request.get('/api/v1/oidc/login', { maxRedirects: 0 })
    expect(resp.status()).toBe(302)
    const location = resp.headers()['location'] ?? ''
    expect(location).toContain('/oauth2/authorize')
    expect(location).toContain('code_challenge=')
    expect(location).toContain('code_challenge_method=S256')
    expect(location).toContain('offline_access')
    // State cookie carries "state|verifier" for the callback.
    const setCookies = resp.headers()['set-cookie'] ?? ''
    expect(setCookies).toContain('funnelbarn_oidc_state')
    expect(setCookies).toContain('funnelbarn_oidc_nonce')
  })

  test('callback without state cookie → 400', async ({ request }) => {
    const resp = await request.get('/api/v1/oidc/callback?code=fake&state=fake', {
      maxRedirects: 0,
    })
    expect(resp.status()).toBe(400)
  })
})

test.describe('API: back-channel logout endpoint', () => {
  test('rejects garbage logout tokens with 400', async ({ request }) => {
    const resp = await request.post('/api/v1/oidc/backchannel-logout', {
      form: { logout_token: 'not.a.jwt' },
    })
    expect(resp.status()).toBe(400)
  })

  test('rejects requests without a token', async ({ request }) => {
    const resp = await request.post('/api/v1/oidc/backchannel-logout', { form: {} })
    expect(resp.status()).toBe(400)
  })
})

// ── Browser tests ────────────────────────────────────────────────────────────

test.describe('OIDC login page', () => {
  test('auto-redirects to the IAMBarn authorization endpoint', async ({ page }) => {
    await page.context().clearCookies()
    // When oidc.enabled is true, the login page redirects immediately.
    // waitUntil: 'commit' captures the URL as soon as the navigation commits
    // (before the remote IAMBarn page fully loads).
    await page.goto('/login')
    await page.waitForURL(/iam\./, { waitUntil: 'commit', timeout: 10000 })
    const url = page.url()
    // An unauthenticated visitor is bounced to IAMBarn's /auth/login with the
    // authorization request URL-encoded in redirect_uri, so decode before
    // asserting the PKCE authorize request is present (works whether IAMBarn
    // lands us directly on /oauth2/authorize or on the login page first).
    const decoded = decodeURIComponent(url)
    expect(decoded).toContain('/oauth2/authorize')
    expect(decoded).toContain('code_challenge')
  })
})

// ── Full credential flow ──────────────────────────────────────────────────────
// Set E2E_IAMBARN_EMAIL + E2E_IAMBARN_PASSWORD to run these.

test.describe('Full IAMBarn credential flow', () => {
  const email = process.env.E2E_IAMBARN_EMAIL
  const password = process.env.E2E_IAMBARN_PASSWORD

  test.beforeEach(async () => {
    if (!email || !password) {
      test.skip(true, 'Set E2E_IAMBARN_EMAIL and E2E_IAMBARN_PASSWORD to run credential tests')
    }
  })

  async function loginViaIAMBarn(page: import('@playwright/test').Page) {
    await page.context().clearCookies()
    // Login page auto-redirects to IAMBarn when OIDC is configured.
    await page.goto('/login')
    await page.waitForURL(/iam\./, { waitUntil: 'commit', timeout: 10_000 })

    // IAMBarn uses email + password form.
    await page.locator('input[name="email"]').fill(email!)
    await page.getByRole('button', { name: /continue|next/i }).first().click()

    // Password step (IAMBarn may use a two-step flow).
    await page.locator('input[type="password"]').fill(password!)
    await page.locator('button[type="submit"]').click()

    // Consent screen if present.
    const allowBtn = page.getByRole('button', { name: /allow/i })
    if (await allowBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await allowBtn.click()
    }

    // Should land back on FunnelBarn dashboard.
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 })
  }

  test('logs in via IAMBarn and lands on /dashboard', async ({ page }) => {
    test.setTimeout(30_000)
    await loginViaIAMBarn(page)

    // The session cookie is an opaque handle bound to a server-side row —
    // the API must recognise it.
    const me = await page.request.get('/api/v1/me')
    expect(me.status()).toBe(200)
  })

  test('IAMBarn session persists across page reload', async ({ page }) => {
    test.setTimeout(30_000)
    await loginViaIAMBarn(page)

    await page.reload()
    await expect(page).toHaveURL(/\/dashboard/)
  })

  test('server-driven logout kills the session and returns an end-session URL', async ({ page }) => {
    test.setTimeout(30_000)
    await loginViaIAMBarn(page)

    const resp = await page.request.post('/api/v1/logout')
    expect(resp.status()).toBe(200)
    const body = await resp.json()
    // OIDC sessions must carry the IdP end-session URL (server-driven logout).
    expect(body.logout_url).toContain('/oauth2/end-session')
    expect(body.logout_url).toContain('id_token_hint=')

    // The server-side row is gone: the old cookie no longer authenticates.
    const me = await page.request.get('/api/v1/me')
    expect(me.status()).toBe(401)
  })
})
