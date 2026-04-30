import { chromium, FullConfig } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'

export default async function globalSetup(config: FullConfig) {
  // Only run if we have a base URL (i.e., in CI against a real environment)
  const baseURL = process.env.E2E_BASE_URL
  if (!baseURL) return

  const clusterIP = process.env.E2E_CLUSTER_IP || ''
  const originalHostname = new URL(baseURL).hostname
  const apiBaseURL = clusterIP
    ? baseURL.replace(/^(https?:\/\/)[^/:]+/, `$1${clusterIP}`)
    : baseURL

  const browser = await chromium.launch()
  const context = await browser.newContext({
    baseURL: apiBaseURL,
    ignoreHTTPSErrors: true,
    extraHTTPHeaders: clusterIP ? { host: originalHostname } : {},
  })
  const page = await context.newPage()

  // Navigate to login page and authenticate
  const loginURL = clusterIP
    ? apiBaseURL + '/login'
    : baseURL + '/login'

  await page.goto(loginURL)
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
