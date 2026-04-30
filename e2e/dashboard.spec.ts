import { test, expect } from '@playwright/test'

async function navigateToDashboard(page: import('@playwright/test').Page) {
  // storageState provides auth; just navigate to dashboard
  const projectId = await page.evaluate(async () => {
    try {
      const res = await fetch('/api/v1/projects')
      const data = await res.json()
      const projects = data.projects ?? []
      return projects.length > 0 ? (projects[0].ID ?? projects[0].id ?? null) : null
    } catch { return null }
  })
  if (projectId) {
    await page.goto(`/dashboard/${projectId}`)
  } else {
    await page.goto('/dashboard')
  }
}

test.describe('dashboard', () => {
  test('shows stat cards after login', async ({ page }) => {
    await navigateToDashboard(page)
    // Expect the overview heading
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
    // Expect at least the stat card labels
    await expect(page.getByText('Total Events')).toBeVisible()
    await expect(page.getByText('Unique Sessions')).toBeVisible()
    await expect(page.getByText('Bounce Rate')).toBeVisible()
  })

  test('time range buttons are visible', async ({ page }) => {
    await navigateToDashboard(page)
    await expect(page.getByRole('button', { name: '24h' })).toBeVisible()
    await expect(page.getByRole('button', { name: '7d' })).toBeVisible()
    await expect(page.getByRole('button', { name: '30d' })).toBeVisible()
  })
})
