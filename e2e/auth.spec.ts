import { test, expect } from '@playwright/test'

test.describe('auth', () => {
  test('landing page shows hero text when logged out', async ({ page }) => {
    await page.context().clearCookies()
    await page.goto('/')
    await expect(page.getByText('Own Your')).toBeVisible()
    await expect(page.getByText('Analytics')).toBeVisible()
  })

  test('login and redirect to dashboard', async ({ page }) => {
    // Clear storage state to simulate logged-out user
    await page.context().clearCookies()
    await page.goto('/login')
    await page.getByLabel('Username').fill('wiebe')
    await page.getByLabel('Password').fill('wiebe')
    await page.getByRole('button', { name: /sign in/i }).click()
    await expect(page).toHaveURL(/\/dashboard/)
  })

  test('logout redirects to landing', async ({ page }) => {
    // Clear storage state to simulate logged-out user, then log in
    await page.context().clearCookies()
    await page.goto('/login')
    await page.getByLabel('Username').fill('wiebe')
    await page.getByLabel('Password').fill('wiebe')
    await page.getByRole('button', { name: /sign in/i }).click()
    await expect(page).toHaveURL(/\/dashboard/)

    // Open user menu and logout
    await page.getByRole('button', { name: /wiebe/i }).click()
    await page.getByRole('button', { name: /logout/i }).click()
    await expect(page).toHaveURL('/')
  })
})
