import { defineConfig, devices } from '@playwright/test'

// Config for the member sign-in refusal smoke only. It has no global setup:
// the main config's setup signs in as the local E2E admin, which this check
// must not carry.
const baseURL = process.env.E2E_BASE_URL || 'https://funnelbarn.wiebe.xyz'

export default defineConfig({
  testDir: '.',
  testMatch: /member-signin-refusal\.spec\.ts/,
  timeout: 60_000,
  retries: 0,
  workers: 1,
  reporter: [['list']],
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
    trace: 'off',
    screenshot: 'off',
    video: 'off',
  },
  projects: [{ name: 'chromium' }],
})
