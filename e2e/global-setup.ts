import { chromium, FullConfig } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'

export default async function globalSetup(config: FullConfig) {
  // Only run if we have a base URL (i.e., in CI against a real environment)
  const baseURL = process.env.E2E_BASE_URL
  if (!baseURL) return

  const clusterIP = process.env.E2E_CLUSTER_IP || ''
  const originalHostname = new URL(baseURL).hostname

  // Use --host-resolver-rules to point the hostname at the cluster IP without
  // touching the Host header (which is a restricted header Chromium refuses to set).
  const browser = await chromium.launch({
    args: clusterIP ? [`--host-resolver-rules=MAP ${originalHostname} ${clusterIP}`] : [],
  })
  const context = await browser.newContext({
    baseURL,
    ignoreHTTPSErrors: true,
  })
  const page = await context.newPage()

  await page.goto(baseURL + '/login')
  await page.getByLabel('Username').fill('wiebe')
  await page.getByLabel('Password').fill('wiebe')
  await page.getByRole('button', { name: /sign in/i }).click()
  await page.waitForURL(/\/dashboard/)

  // Save auth state for all workers to reuse
  const authFile = path.join(__dirname, '.auth', 'user.json')
  fs.mkdirSync(path.dirname(authFile), { recursive: true })
  await context.storageState({ path: authFile })

  await browser.close()
}
