import { defineConfig } from '@playwright/test'

const E2E_BASE_URL = process.env.E2E_BASE_URL || 'http://localhost:5173'
const isCIEnv = !!process.env.E2E_BASE_URL

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  use: {
    baseURL: E2E_BASE_URL,
    ignoreHTTPSErrors: true,
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
  // Only start web server for local dev (not in CI against live env)
  ...(isCIEnv ? {} : {
    webServer: {
      command: 'cd web && npm run dev',
      url: 'http://localhost:5173',
      reuseExistingServer: true,
    },
  }),
})
