/**
 * Verify the session replay player renders content and is not clipped.
 *
 * Run locally against the test env:
 *   FUNNELBARN_API_URL=https://funnelbarn-test.nijmegen.wiebe.xyz \
 *   npx playwright test e2e/recording-verify.spec.ts --project=chromium --headed
 */
import { test, expect } from '@playwright/test'

const PROJECT_ID = 'e7af9de5-04d3-4e00-a552-5fa49baca192'

test.beforeEach(async ({ page }) => {
  const res = await page.request.post('/api/v1/login', {
    data: { username: 'wiebe', password: 'wiebe' },
  })
  expect(res.ok(), `Login failed: ${res.status()}`).toBeTruthy()
})

test('sessions list shows recordings', async ({ page }) => {
  await page.goto(`/sessions/${PROJECT_ID}`)

  // Wait for the "X shown" counter to appear (replaces "Loading…")
  await expect(page.getByText(/\d+ shown/)).toBeVisible({ timeout: 15000 })

  // At least one recording row must be present — each row has cursor:pointer
  const row = page.locator('div[style*="cursor: pointer"]').first()
  await expect(row).toBeVisible({ timeout: 5000 })
})

test('sessions list exposes replay status and a delete control', async ({ page }) => {
  await page.goto(`/sessions/${PROJECT_ID}`)
  await expect(page.getByText(/\d+ shown/)).toBeVisible({ timeout: 15000 })

  // The Replay column header and a per-row delete control must be present.
  await expect(page.getByText('Replay', { exact: true })).toBeVisible({ timeout: 5000 })
  await expect(page.locator('button[title="Delete recording"]').first()).toBeVisible({ timeout: 5000 })
})

test('replay player is scaled and visible — not a black screen', async ({ page }) => {
  await page.goto(`/sessions/${PROJECT_ID}`)

  // Wait for recordings to load
  await expect(page.getByText(/\d+ shown/)).toBeVisible({ timeout: 15000 })

  // Click the first playable recording row (cursor:pointer is only set when a
  // snapshot exists, so this also skips unplayable rows).
  const row = page.locator('div[style*="cursor: pointer"]').first()
  await expect(row).toBeVisible({ timeout: 5000 })
  await row.click()

  // The "snapshot missing" failure must never appear for a playable recording —
  // this guards the chunk-span fix (first..last inclusive).
  await expect(page.getByText(/initial page snapshot is missing/)).toHaveCount(0)

  // The modal + rrweb wrapper must appear
  const wrapper = page.locator('.replayer-wrapper').first()
  await expect(wrapper).toBeVisible({ timeout: 20000 })

  // 1. CSS scaling must be applied (transform should be a matrix, not 'none')
  const transform = await wrapper.evaluate((el) => getComputedStyle(el).transform)
  console.log('transform:', transform)
  expect(transform, 'replayer-wrapper should have a CSS scale transform applied').not.toBe('none')

  // 2. The iframe must be within the player container bounds (not pushed below by overflow)
  const container = wrapper.locator('..')
  const containerBox = await container.boundingBox()
  const iframe = wrapper.locator('iframe').first()
  const iframeBox = await iframe.boundingBox()
  console.log('container:', containerBox)
  console.log('iframe:', iframeBox)
  if (containerBox && iframeBox) {
    expect(
      iframeBox.y,
      'iframe top must be within the container (not clipped below)',
    ).toBeLessThan(containerBox.y + containerBox.height)
  }

  // 3. The iframe body must contain child elements (actual recorded DOM)
  const hasContent = await iframe.evaluate((el) => {
    const body = (el as HTMLIFrameElement).contentDocument?.body
    return (body?.children.length ?? 0) > 0
  })
  console.log('iframe has content:', hasContent)
  expect(hasContent, 'iframe body should have child elements from the recording').toBeTruthy()
})
