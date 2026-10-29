import { test, expect, type Page } from '@playwright/test'

// The flags page with a long list. The flag endpoints are stubbed so the test
// does not depend on how many flags the environment happens to hold: 40 flags,
// the last 12 auto-detected with long keys.
const FLAG_COUNT = 40

function stubFlags(projectId: string) {
  const statuses = ['active', 'paused', 'inactive']
  return Array.from({ length: FLAG_COUNT }, (_, i) => {
    const auto = i >= FLAG_COUNT - 12
    return {
      id: `e2e-flag-${i}`,
      project_id: projectId,
      flag_key: auto ? `auto_detected_feature_with_a_long_key_${i}` : `e2e_flag_${i}`,
      name: auto ? `auto_detected_feature_with_a_long_key_${i}` : `E2E flag ${i}`,
      flag_type: 'boolean',
      variants: '{"on":true,"off":false}',
      default_variant: 'off',
      split: '{"on":0,"off":100}',
      targeting_rules: '[]',
      status: auto ? 'inactive' : statuses[i % 3],
      created_at: `2026-10-${String(1 + (i % 28)).padStart(2, '0')} 10:00:00`,
      origin: auto ? 'auto' : 'manual',
      flag_kind: auto ? 'config' : 'experiment',
    }
  })
}

async function openFlagsPage(page: Page) {
  await page.goto('/')
  const projectId = await page.evaluate(async () => {
    const res = await fetch('/api/v1/projects')
    const data = await res.json()
    const projects = data.projects ?? []
    return projects.length > 0 ? (projects[0].ID ?? projects[0].id ?? null) : null
  })
  test.skip(!projectId, 'no project to open the flags page for')
  await page.route(`**/api/v1/projects/${projectId}/flags`, (route) =>
    route.request().method() === 'GET' ? route.fulfill({ json: { flags: stubFlags(projectId) } }) : route.continue())
  await page.route(`**/api/v1/projects/${projectId}/flags/*/analysis`, (route) =>
    route.fulfill({ json: { results: [], significant: false, unavailable: 'config' } }))
  await page.goto(`/flags/${projectId}`)
  await expect(page.getByTestId('flag-count')).toHaveText(`${FLAG_COUNT} flags`)
}

test.describe('flags page with many flags', () => {
  test('a flag low in the list opens its detail in view', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await openFlagsPage(page)

    const rows = page.getByRole('list', { name: 'Flags' }).getByRole('listitem')
    await expect(rows).toHaveCount(FLAG_COUNT)

    const last = rows.last().getByRole('button')
    await last.scrollIntoViewIfNeeded()
    await last.click()

    await expect(page.getByRole('heading', { level: 2, name: /long_key/ })).toBeInViewport()
    // The search box stays reachable after scrolling the list.
    await expect(page.getByRole('searchbox', { name: 'Search flags' })).toBeInViewport()
  })

  test('search and the status filter narrow the list', async ({ page }) => {
    await openFlagsPage(page)
    await page.getByRole('searchbox', { name: 'Search flags' }).fill('long_key')
    await expect(page.getByTestId('flag-count')).toHaveText(`12 of ${FLAG_COUNT} flags`)
    await page.getByRole('searchbox', { name: 'Search flags' }).fill('')
    await page.getByRole('button', { name: /^Paused \d+$/ }).click()
    await expect(page.getByRole('list', { name: 'Flags' }).getByRole('listitem')).toHaveCount(9)
  })

  test('rows fit the phone width', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openFlagsPage(page)
    const box = await page.getByRole('list', { name: 'Flags' }).getByRole('listitem').last().boundingBox()
    expect(box).not.toBeNull()
    expect(box!.x + box!.width).toBeLessThanOrEqual(390)
  })
})
