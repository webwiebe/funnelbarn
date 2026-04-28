import { test, expect } from '@playwright/test'

async function loginAndGetFunnelsUrl(page: import('@playwright/test').Page): Promise<string> {
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

  return projectId ? `/funnels/${projectId}` : '/funnels'
}

test.describe('funnels', () => {
  test('shows funnels page with create button', async ({ page }) => {
    const url = await loginAndGetFunnelsUrl(page)
    await page.goto(url)
    await expect(page.getByRole('heading', { name: 'Funnels' })).toBeVisible()
    await expect(page.getByRole('button', { name: /create funnel/i })).toBeVisible()
  })

  test('can open create funnel modal', async ({ page }) => {
    const url = await loginAndGetFunnelsUrl(page)
    await page.goto(url)
    await page.getByRole('button', { name: /create funnel/i }).click()
    await expect(page.getByRole('heading', { name: 'Create funnel' })).toBeVisible()
    await expect(page.getByPlaceholder(/signup flow/i)).toBeVisible()
  })

  test('create funnel modal has step inputs', async ({ page }) => {
    const url = await loginAndGetFunnelsUrl(page)
    await page.goto(url)
    await page.getByRole('button', { name: /create funnel/i }).click()
    // Should have at least 2 step inputs by default
    const stepInputs = page.getByPlaceholder(/event name/i)
    await expect(stepInputs.first()).toBeVisible()
  })
})
