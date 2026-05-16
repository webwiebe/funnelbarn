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
