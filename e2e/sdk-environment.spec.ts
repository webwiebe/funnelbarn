import { test, expect } from '@playwright/test'

/**
 * Playwright sets navigator.webdriver, so this run is the traffic the SDK must
 * keep out of production counts. The page configures production and the stored
 * row must still say test.
 */
test('SDK in an automated browser stores its events as test', async ({ page }) => {
  const username = process.env.E2E_USERNAME
  const password = process.env.E2E_PASSWORD
  expect(Boolean(username && password), 'E2E_USERNAME and E2E_PASSWORD must be set').toBe(true)

  const login = await page.request.post('/api/v1/login', {
    data: { username, password },
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
    data: { name: 'E2E SDK environment', slug: `e2e-sdk-env-${Date.now()}` },
  })
  expect(created.status()).toBe(201)
  const project = await created.json()
  const projectId = project.ID ?? project.id

  try {
    const keyResp = await page.request.post('/api/v1/apikeys', {
      headers,
      data: { project_id: projectId, name: 'e2e-sdk-env' },
    })
    expect(keyResp.status()).toBe(201)
    const { key } = await keyResp.json()

    await page.goto('/')
    await page.addScriptTag({ url: '/sdk.js' })
    await page.evaluate((apiKey) => {
      const sdk = (window as unknown as { funnelbarn: any }).funnelbarn
      sdk.init({ apiKey, endpoint: location.origin, environment: 'production', flushInterval: 200 })
      sdk.track('sdk_environment_check')
    }, key)

    await expect
      .poll(
        async () => {
          const resp = await page.request.get(
            `/api/v1/projects/${projectId}/events?name=sdk_environment_check`,
          )
          const body = await resp.json()
          return body.events?.[0]?.environment
        },
        // Under the 30s test timeout, so the project is still deleted on a failure.
        { timeout: 20000 },
      )
      .toBe('test')
  } finally {
    await page.request.delete(`/api/v1/projects/${projectId}`, { headers })
  }
})
