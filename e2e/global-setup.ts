import { request as playwrightRequest, chromium, FullConfig } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'

export default async function globalSetup(_config: FullConfig) {
  // Only run if we have a base URL (i.e., in CI against a real environment)
  const baseURL = process.env.E2E_BASE_URL
  if (!baseURL) return

  const clusterIP = process.env.E2E_CLUSTER_IP || ''
  const originalHostname = new URL(baseURL).hostname

  // Authenticate via the API directly rather than through the login UI.
  // This is independent of which auth mode is configured on the frontend
  // (IAMBarn, OIDC, or local) — the password endpoint is always available
  // for the test credentials regardless of what the login page renders.
  const apiContext = await playwrightRequest.newContext({
    baseURL,
    ignoreHTTPSErrors: true,
  })
  const res = await apiContext.post('/api/v1/login', {
    data: { username: 'wiebe', password: 'wiebe' },
  })
  if (!res.ok()) {
    throw new Error(`E2E login failed: HTTP ${res.status()} — ${await res.text()}`)
  }

  // Dump the session cookies so all Playwright workers can reuse the session.
  const cookies = await apiContext.storageState()
  await apiContext.dispose()

  // Spin up a browser briefly to stamp the storage-state file with the correct
  // browser origin, which some Playwright fixtures expect.
  const browser = await chromium.launch({
    // Use --host-resolver-rules to point the hostname at the cluster IP without
    // touching the Host header (which is a restricted header Chromium refuses to set).
    args: clusterIP ? [`--host-resolver-rules=MAP ${originalHostname} ${clusterIP}`] : [],
  })
  const context = await browser.newContext({
    baseURL,
    ignoreHTTPSErrors: true,
    storageState: cookies,
  })

  const authFile = path.join(__dirname, '.auth', 'user.json')
  fs.mkdirSync(path.dirname(authFile), { recursive: true })
  await context.storageState({ path: authFile })

  await browser.close()
}
