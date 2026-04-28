import { test, expect } from '@playwright/test'

async function loginAndNavigateToDashboard(page: import('@playwright/test').Page) {
  // Login via browser
  await page.goto('/login')
  await page.getByLabel('Username').fill('wiebe')
  await page.getByLabel('Password').fill('wiebe')
  await page.getByRole('button', { name: /sign in/i }).click()
  await expect(page).toHaveURL(/\/dashboard/)

  // Fetch the first project ID from within the browser context (cookies are set)
  const projectId = await page.evaluate(async () => {
    try {
      const res = await fetch('/api/v1/projects')
      const data = await res.json()
      const projects = data.projects ?? []
      if (projects.length === 0) return null
      return projects[0].ID ?? projects[0].id ?? null
    } catch {
      return null
    }
  })

  if (projectId) {
    await page.goto(`/dashboard/${projectId}`)
  }
}

test.describe('dashboard', () => {
  test('shows stat cards after login', async ({ page }) => {
    await loginAndNavigateToDashboard(page)
    // Expect the overview heading
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
    // Expect at least the stat card labels
    await expect(page.getByText('Total Events')).toBeVisible()
    await expect(page.getByText('Unique Sessions')).toBeVisible()
    await expect(page.getByText('Bounce Rate')).toBeVisible()
  })

  test('time range buttons are visible', async ({ page }) => {
    await loginAndNavigateToDashboard(page)
    await expect(page.getByRole('button', { name: '24h' })).toBeVisible()
    await expect(page.getByRole('button', { name: '7d' })).toBeVisible()
    await expect(page.getByRole('button', { name: '30d' })).toBeVisible()
  })
})
