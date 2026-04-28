import { test, expect } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('Username').fill('wiebe')
  await page.getByLabel('Password').fill('wiebe')
  await page.getByRole('button', { name: /sign in/i }).click()
  await expect(page).toHaveURL(/\/dashboard/)
}

test.describe('dashboard', () => {
  test('shows stat cards after login', async ({ page }) => {
    await login(page)
    // Expect the overview heading
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
    // Expect at least the stat card labels
    await expect(page.getByText('Total Events')).toBeVisible()
    await expect(page.getByText('Unique Sessions')).toBeVisible()
    await expect(page.getByText('Bounce Rate')).toBeVisible()
  })

  test('time range buttons are visible', async ({ page }) => {
    await login(page)
    await expect(page.getByRole('button', { name: '24h' })).toBeVisible()
    await expect(page.getByRole('button', { name: '7d' })).toBeVisible()
    await expect(page.getByRole('button', { name: '30d' })).toBeVisible()
  })
})
