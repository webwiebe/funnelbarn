import { test, expect } from '@playwright/test'

const CHROME_UA =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36'
const NON_CHROME_UA =
  'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)'

// ── API-level tests (no browser, run in the "api" project) ──────────────────

test.describe('API: iambarn feature flag via client-config', () => {
  test('Chrome UA → iambarn_enabled true', async ({ request }) => {
    const resp = await request.get('/api/v1/client-config', {
      headers: { 'user-agent': CHROME_UA },
    })
    expect(resp.status()).toBe(200)
    const body = await resp.json()
    expect(body.iambarn_enabled).toBe(true)
  })

  test('non-Chrome UA → iambarn_enabled false', async ({ request }) => {
    const resp = await request.get('/api/v1/client-config', {
      headers: { 'user-agent': NON_CHROME_UA },
    })
    expect(resp.status()).toBe(200)
    const body = await resp.json()
    expect(body.iambarn_enabled).toBe(false)
  })
})

test.describe('API: OIDC login endpoint gating', () => {
  test('non-Chrome UA → 404 (flag off)', async ({ request }) => {
    const resp = await request.get('/api/v1/auth/oidc/login', {
      headers: { 'user-agent': NON_CHROME_UA },
    })
    // Flag evaluates to off for this UA; handler returns 404.
    expect(resp.status()).toBe(404)
  })
})

// ── Browser tests (Chromium UA contains "Chrome") ───────────────────────────

test.describe('iambarn login page', () => {
  test('shows Continue with IAMBarn button in Chrome', async ({ page }) => {
    await page.context().clearCookies()
    await page.goto('/login')
    await expect(page.getByText('Continue with IAMBarn')).toBeVisible({ timeout: 5000 })
  })

  test('IAMBarn button redirects to IAMBarn authorization endpoint', async ({ page }) => {
    await page.context().clearCookies()
    await page.goto('/login')
    await expect(page.getByText('Continue with IAMBarn')).toBeVisible()

    // Navigate and grab the redirect URL before the IAMBarn page loads.
    await page.getByText('Continue with IAMBarn').click()
    // waitUntil: 'commit' captures the URL as soon as the navigation is
    // committed (before the remote page fully loads), so this works even
    // if iam.wiebe.xyz isn't reachable from the runner.
    await page.waitForURL(/iam\.wiebe\.xyz/, { waitUntil: 'commit', timeout: 10000 })
    expect(page.url()).toContain('iam.wiebe.xyz/oauth2/authorize')
    expect(page.url()).toContain('code_challenge')
    expect(page.url()).toContain('client_id=ibc_')
  })
})

// ── OIDC callback error handling ─────────────────────────────────────────────

test.describe('OIDC callback error handling', () => {
  test('callback without state cookie → /login?error=auth_failed', async ({ page }) => {
    await page.context().clearCookies()
    await page.goto('/api/v1/auth/oidc/callback?code=fake&state=fake')
    await expect(page).toHaveURL(/\/login\?error=auth_failed/, { timeout: 6000 })
    await expect(page.getByText(/Login failed/i)).toBeVisible()
  })

  test('callback with mismatched state → /login?error=auth_failed', async ({ page }) => {
    await page.context().clearCookies()
    const baseURL = process.env.E2E_BASE_URL || 'http://localhost:5173'
    const hostname = new URL(baseURL).hostname
    await page.context().addCookies([{
      name: 'oidc_state',
      value: 'expected_state|some_verifier',
      domain: hostname,
      path: '/',
    }])
    await page.goto('/api/v1/auth/oidc/callback?code=fake&state=wrong_state')
    await expect(page).toHaveURL(/\/login\?error=auth_failed/, { timeout: 6000 })
  })

  test('callback with error param → /login?error=auth_failed', async ({ page }) => {
    await page.context().clearCookies()
    const baseURL = process.env.E2E_BASE_URL || 'http://localhost:5173'
    const hostname = new URL(baseURL).hostname
    await page.context().addCookies([{
      name: 'oidc_state',
      value: 'real_state|some_verifier',
      domain: hostname,
      path: '/',
    }])
    await page.goto('/api/v1/auth/oidc/callback?error=access_denied&state=real_state')
    await expect(page).toHaveURL(/\/login\?error=auth_failed/, { timeout: 6000 })
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

  test('logs in via IAMBarn and lands on /dashboard', async ({ page }) => {
    test.setTimeout(30_000)
    await page.context().clearCookies()
    await page.goto('/login')
    await page.getByText('Continue with IAMBarn').click()

    // Wait for IAMBarn login page.
    await page.waitForURL(/iam\.wiebe\.xyz/, { waitUntil: 'commit', timeout: 10_000 })

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
  })

  test('IAMBarn session persists across page reload', async ({ page }) => {
    test.setTimeout(30_000)
    await page.context().clearCookies()
    await page.goto('/login')
    await page.getByText('Continue with IAMBarn').click()
    await page.waitForURL(/iam\.wiebe\.xyz/, { waitUntil: 'commit', timeout: 10_000 })

    await page.locator('input[name="email"]').fill(email!)
    await page.getByRole('button', { name: /continue|next/i }).first().click()
    await page.locator('input[type="password"]').fill(password!)
    await page.locator('button[type="submit"]').click()

    const allowBtn = page.getByRole('button', { name: /allow/i })
    if (await allowBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await allowBtn.click()
    }

    await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 })

    await page.reload()
    await expect(page).toHaveURL(/\/dashboard/)
  })
})
