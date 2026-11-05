import { test, expect } from '@playwright/test'

test.describe('auth', () => {
  test('landing page shows hero text when logged out', async ({ page }) => {
    await page.context().clearCookies()
    await page.goto('/')
    await expect(page.getByText('Own Your')).toBeVisible()
    await expect(page.getByText('Analytics', { exact: true })).toBeVisible()
  })

  test('login and redirect to dashboard', async ({ page }) => {
    await page.context().clearCookies()
    // Authenticate via the API — works regardless of which auth mode the login
    // UI shows (local form, IAMBarn redirect, or other OIDC provider).
    const res = await page.request.post('/api/v1/login', {
      data: { username: process.env.E2E_USERNAME, password: process.env.E2E_PASSWORD },
    })
    expect(res.ok()).toBe(true)
    await page.goto('/dashboard')
    await expect(page).toHaveURL(/\/dashboard/)
  })

  test('logout redirects to landing', async ({ page }) => {
    await page.context().clearCookies()
    const res = await page.request.post('/api/v1/login', {
      data: { username: process.env.E2E_USERNAME, password: process.env.E2E_PASSWORD },
    })
    expect(res.ok()).toBe(true)
    await page.goto('/dashboard')
    await expect(page).toHaveURL(/\/dashboard/)

    // Open user menu and logout. The menu button holds the logged-in username;
    // it is found by its markup so no locator or assertion carries the value.
    expect(Boolean(process.env.E2E_USERNAME), 'E2E_USERNAME must be set').toBe(true)
    await page.locator('button:has(.user-name)').click()
    await page.getByRole('button', { name: /logout/i }).click()
    await expect(page).toHaveURL('/')
  })
})
