/**
 * Verify the session replay player renders content and is not clipped.
 *
 * Each test creates its own project, ingests one recording with a known
 * viewport through the public chunk endpoint, and deletes the project
 * afterwards, so the result does not depend on whatever recording last
 * arrived on the shared environment.
 *
 * Run locally against the test env:
 *   E2E_BASE_URL=https://funnelbarn.test.wiebe.xyz \
 *   pnpm exec playwright test e2e/recording-verify.spec.ts --project=chromium --headed
 */
import { test, expect, type Page } from '@playwright/test'

// The Sessions page shows human traffic by default, and DetectBot marks a
// HeadlessChrome User-Agent as a bot, so the fixture chunk says it came from
// a desktop browser.
const DESKTOP_UA =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36'

// The fixture's viewport. It differs from the player's 1280x720 fallback, so
// a passing test shows the Meta event was read.
const RECORDED = { width: 1000, height: 600 }

interface Fixture {
  projectId: string
  cleanup: () => Promise<void>
}

function fixtureEvents(start: number) {
  return [
    { type: 4, timestamp: start, data: { href: 'https://e2e.invalid/replay', ...RECORDED } },
    {
      type: 2,
      timestamp: start + 1,
      data: {
        initialOffset: { top: 0, left: 0 },
        node: {
          type: 0, id: 1, childNodes: [
            { type: 1, id: 2, name: 'html', publicId: '', systemId: '' },
            {
              type: 2, id: 3, tagName: 'html', attributes: {}, childNodes: [
                { type: 2, id: 4, tagName: 'head', attributes: {}, childNodes: [] },
                {
                  type: 2, id: 5, tagName: 'body', attributes: {}, childNodes: [
                    {
                      type: 2, id: 6, tagName: 'div', attributes: { id: 'fixture' }, childNodes: [
                        { type: 3, id: 7, textContent: 'E2E replay fixture' },
                      ],
                    },
                  ],
                },
              ],
            },
          ],
        },
      },
    },
    { type: 3, timestamp: start + 1000, data: { source: 1, positions: [{ x: 10, y: 10, id: 6, timeOffset: 0 }] } },
  ]
}

async function seedRecording(page: Page): Promise<Fixture> {
  const login = await page.request.post('/api/v1/login', {
    data: { username: process.env.E2E_USERNAME, password: process.env.E2E_PASSWORD },
  })
  expect(login.ok(), `Login failed: ${login.status()}`).toBeTruthy()
  const csrfCookie = login
    .headersArray()
    .filter((h) => h.name.toLowerCase() === 'set-cookie')
    .map((h) => h.value.match(/funnelbarn_csrf=([^;]+)/))
    .find(Boolean)
  expect(csrfCookie, 'login did not set funnelbarn_csrf cookie').toBeTruthy()
  const headers = { 'X-FunnelBarn-CSRF': decodeURIComponent(csrfCookie![1]) }

  const created = await page.request.post('/api/v1/projects', {
    headers,
    data: { name: 'E2E replay', slug: `e2e-replay-${Date.now()}-${Math.floor(Math.random() * 1e6)}` },
  })
  expect(created.status()).toBe(201)
  const project = await created.json()
  const projectId: string = project.ID ?? project.id
  const cleanup = async () => {
    await page.request.delete(`/api/v1/projects/${projectId}`, { headers })
  }

  try {
    const keyResp = await page.request.post('/api/v1/apikeys', {
      headers,
      data: { project_id: projectId, name: 'e2e-replay' },
    })
    expect(keyResp.status()).toBe(201)
    const { key } = await keyResp.json()

    const start = Date.now() - 5000
    const id = `e2e${start}`
    const chunk = await page.request.post('/api/v1/recordings/chunk', {
      headers: { 'x-funnelbarn-api-key': key, 'User-Agent': DESKTOP_UA },
      data: {
        recording_id: id,
        session_id: id,
        chunk_index: 0,
        events: fixtureEvents(start),
        started_at: new Date(start).toISOString(),
        duration_ms: 1000,
        page_url: 'https://e2e.invalid/replay',
      },
    })
    expect(chunk.status(), `chunk ingest: ${await chunk.text()}`).toBe(202)

    await expect
      .poll(async () => {
        const resp = await page.request.get(`/api/v1/projects/${projectId}/recordings?human_only=true`)
        const body = await resp.json()
        return body.recordings?.length ?? 0
      }, { timeout: 10000 })
      .toBe(1)
  } catch (err) {
    await cleanup()
    throw err
  }
  return { projectId, cleanup }
}

let fixture: Fixture

test.beforeEach(async ({ page }) => {
  fixture = await seedRecording(page)
})

test.afterEach(async () => {
  await fixture?.cleanup()
})

test('sessions list shows recordings', async ({ page }) => {
  await page.goto(`/sessions/${fixture.projectId}`)

  // Wait for the "X shown" counter to appear (replaces "Loading…")
  await expect(page.getByText(/\d+ shown/)).toBeVisible({ timeout: 15000 })

  // At least one recording row must be present — each row has cursor:pointer
  const row = page.locator('div[style*="cursor: pointer"]').first()
  await expect(row).toBeVisible({ timeout: 5000 })
})

test('sessions list exposes replay status and a delete control', async ({ page }) => {
  await page.goto(`/sessions/${fixture.projectId}`)
  await expect(page.getByText(/\d+ shown/)).toBeVisible({ timeout: 15000 })

  // The Replay column header and a per-row delete control must be present.
  await expect(page.getByText('Replay', { exact: true })).toBeVisible({ timeout: 5000 })
  await expect(page.locator('button[title="Delete recording"]').first()).toBeVisible({ timeout: 5000 })
})

test('replay player is scaled and visible — not a black screen', async ({ page }) => {
  await page.goto(`/sessions/${fixture.projectId}`)
  await expect(page.getByText(/\d+ shown/)).toBeVisible({ timeout: 15000 })

  // Click the fixture's row (cursor:pointer is only set when a snapshot exists).
  const row = page.locator('div[style*="cursor: pointer"]').first()
  await expect(row).toBeVisible({ timeout: 5000 })
  await row.click()

  // The "snapshot missing" failure must never appear for a playable recording —
  // this guards the chunk-span fix (first..last inclusive).
  await expect(page.getByText(/initial page snapshot is missing/)).toHaveCount(0)

  // The modal + rrweb wrapper must appear
  const wrapper = page.locator('.replayer-wrapper').first()
  await expect(wrapper).toBeVisible({ timeout: 20000 })
  const container = wrapper.locator('..')

  // 1. The player scales the wrapper one frame after rrweb builds it, so wait
  // for that, with a bound. On a failure the message carries the computed
  // transform and the recorded size the player read.
  const scaleState = () =>
    wrapper.evaluate((el) => {
      const parent = el.parentElement as HTMLElement
      return {
        transform: getComputedStyle(el).transform,
        width: parent.dataset.recordedWidth ?? 'unset',
        height: parent.dataset.recordedHeight ?? 'unset',
        source: parent.dataset.recordedSize ?? 'unset',
      }
    })
  const scaled = await expect
    .poll(async () => (await scaleState()).transform !== 'none', { timeout: 10000 })
    .toBe(true)
    .then(() => true, () => false)
  const state = await scaleState()
  console.log('scale:', state)
  expect(scaled, `replayer-wrapper has no scale transform: ${JSON.stringify(state)}`).toBe(true)
  expect(state, 'the player read the recorded viewport from the Meta event').toMatchObject({
    width: String(RECORDED.width),
    height: String(RECORDED.height),
    source: 'meta',
  })

  // 2. The iframe must be within the player container bounds (not pushed below by overflow)
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
  await expect
    .poll(
      () => iframe.evaluate((el) => {
        const body = (el as HTMLIFrameElement).contentDocument?.body
        return (body?.children.length ?? 0) > 0
      }),
      { timeout: 3000 },
    )
    .toBe(true)
})
