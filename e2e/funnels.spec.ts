import { test, expect } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('Username').fill('wiebe')
  await page.getByLabel('Password').fill('wiebe')
  await page.getByRole('button', { name: /sign in/i }).click()
  await expect(page).toHaveURL(/\/dashboard/)
}

test.describe('funnels', () => {
  test('shows funnels page with create button', async ({ page }) => {
    await login(page)
    await page.goto('/funnels')
    await expect(page.getByRole('heading', { name: 'Funnels' })).toBeVisible()
    await expect(page.getByRole('button', { name: /create funnel/i })).toBeVisible()
  })

  test('can open create funnel modal', async ({ page }) => {
    await login(page)
    await page.goto('/funnels')
    await page.getByRole('button', { name: /create funnel/i }).click()
    await expect(page.getByRole('heading', { name: 'Create funnel' })).toBeVisible()
    await expect(page.getByPlaceholder(/funnel name/i)).toBeVisible()
  })

  test('create funnel modal has step inputs', async ({ page }) => {
    await login(page)
    await page.goto('/funnels')
    await page.getByRole('button', { name: /create funnel/i }).click()
    // Should have at least 2 step inputs by default
    const stepInputs = page.getByPlaceholder(/event name/i)
    await expect(stepInputs.first()).toBeVisible()
  })
})
