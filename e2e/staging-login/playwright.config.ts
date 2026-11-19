import { defineConfig, devices } from '@playwright/test'

// Config for the staging IAMBarn browser login check only. It has no global
// setup: the main config's setup signs in as the local E2E admin, and this
// check must reach a session through IAMBarn instead.
//
// E2E_CLUSTER_IP: when the runner cannot reach the staging hostnames through
// public DNS, map both the FunnelBarn and IAMBarn hosts to the cluster IP.
const baseURL = process.env.E2E_BASE_URL || 'https://funnelbarn.staging.wiebe.xyz'
const clusterIP = process.env.E2E_CLUSTER_IP || ''
const hosts = (process.env.E2E_HOSTS || '').split(',').map((h) => h.trim()).filter(Boolean)
const args = clusterIP
  ? hosts.map((h) => `--host-resolver-rules=MAP ${h} ${clusterIP}`)
  : []

export default defineConfig({
  testDir: '.',
  testMatch: /staging-login\.spec\.ts/,
  timeout: 60_000,
  retries: 0,
  workers: 1,
  reporter: [['list']],
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
    // Non-production hosts use the internal CA, as in the main E2E config.
    ignoreHTTPSErrors: true,
    trace: 'off',
    screenshot: 'off',
    video: 'off',
    launchOptions: { args },
  },
  projects: [{ name: 'chromium' }],
})
