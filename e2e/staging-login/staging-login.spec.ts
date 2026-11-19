import { test, expect } from '@playwright/test'

// Live check of the OIDC sign-in path on staging: an IAMBarn account that is
// organization_admin in the organization FunnelBarn trusts must end up with a
// FunnelBarn session. The main E2E suite signs in through POST /api/v1/login,
// so a regression in the IAMBarn login path (client config, callback, role
// check) would pass it unnoticed.
//
// Runs from .github/workflows/staging-login-e2e.yml (nightly and on dispatch),
// never per PR, with its own config (e2e/staging-login/playwright.config.ts).
// The workflow decrypts the account from e2e/secrets/staging-user.sops.yaml.
// The main E2E config ignores this directory.

const email = process.env.E2E_STAGING_EMAIL
const password = process.env.E2E_STAGING_PASSWORD

test.skip(!email || !password, 'Set E2E_STAGING_EMAIL and E2E_STAGING_PASSWORD')

test('an organization admin signs in through IAMBarn and gets a session', async ({ page }) => {
  test.setTimeout(60_000)
  await page.context().clearCookies()

  const callback = page.waitForResponse(
    (r) => new URL(r.url()).pathname === '/api/v1/oidc/callback',
    { timeout: 45_000 },
  )

  await page.goto('/api/v1/oidc/login')
  await page.waitForURL(/iam\./, { waitUntil: 'commit', timeout: 15_000 })

  // The sign-in page offers a magic link by default. "Use password" reveals the
  // password form, which posts to /auth/password-login-form. The page also
  // carries a hidden twin of the email field for password managers, so the
  // visible field is addressed by its id.
  await page.locator('#f-email').fill(email!)
  await page.getByRole('button', { name: /use password/i }).click()
  await page.locator('#_pw_input').fill(password!)
  await page.getByRole('button', { name: /^sign in$/i }).click()

  const allow = page.getByRole('button', { name: /allow/i })
  if (await allow.isVisible({ timeout: 3_000 }).catch(() => false)) {
    await allow.click()
  }

  const resp = await callback
  // The callback redirects into the app on success; 403 would be the role check.
  expect(resp.status(), 'OIDC callback status').toBeLessThan(400)

  const cookies = await page.context().cookies()
  expect(cookies.map((c) => c.name)).toContain('funnelbarn_session')
  const me = await page.request.get('/api/v1/me')
  expect(me.status()).toBe(200)
})
