import { test, expect } from '@playwright/test'

// Live check that the OIDC callback refuses an IAMBarn account that holds only
// the member role in the organization FunnelBarn trusts (OIDCClient.Allowed in
// internal/auth/oidc.go). The refusal must come before any session exists.
//
// It runs from .github/workflows/member-signin-refusal.yml with its own config
// (e2e/member/playwright.config.ts). The workflow decrypts the account from
// e2e/secrets/production-member.sops.yaml in CI. The main E2E config ignores
// this directory.

const email = process.env.E2E_MEMBER_EMAIL
const password = process.env.E2E_MEMBER_PASSWORD

test.skip(!email || !password, 'Set E2E_MEMBER_EMAIL and E2E_MEMBER_PASSWORD')

test('the OIDC callback refuses a member with 403 and starts no session', async ({ page }) => {
  test.setTimeout(60_000)
  await page.context().clearCookies()

  const callback = page.waitForResponse(
    (r) => new URL(r.url()).pathname === '/api/v1/oidc/callback',
    { timeout: 45_000 },
  )

  await page.goto('/api/v1/oidc/login')
  await page.waitForURL(/iam\./, { waitUntil: 'commit', timeout: 15_000 })

  await page.locator('input[name="email"]').fill(email!)
  await page.getByRole('button', { name: /continue|next/i }).first().click()
  await page.locator('input[type="password"]').fill(password!)
  await page.locator('button[type="submit"]').click()

  const allow = page.getByRole('button', { name: /allow/i })
  if (await allow.isVisible({ timeout: 3_000 }).catch(() => false)) {
    await allow.click()
  }

  const resp = await callback
  expect(resp.status()).toBe(403)
  expect(await resp.text()).toContain('access denied')

  const cookies = await page.context().cookies()
  expect(cookies.map((c) => c.name)).not.toContain('funnelbarn_session')
  const me = await page.request.get('/api/v1/me')
  expect(me.status()).toBe(401)
})
