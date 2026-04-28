import { defineConfig, devices } from '@playwright/test'

// E2E_BASE_URL: the URL to test against (must use the real hostname so TLS
//   SNI and cookies work correctly)
// E2E_CLUSTER_IP: when the public DNS for the test hostname doesn't route to
//   the cluster from this machine (e.g. private-network CI runner), set this
//   to the cluster's private IP.
//   - Chromium browser: DNS override via --host-resolver-rules
//   - Playwright request context: baseURL rewritten to IP + Host header
const E2E_BASE_URL = process.env.E2E_BASE_URL || 'http://localhost:5173'
const clusterIP = process.env.E2E_CLUSTER_IP || ''
const isCIEnv = !!process.env.E2E_BASE_URL

const originalHostname = E2E_BASE_URL ? (() => {
  try { return new URL(E2E_BASE_URL).hostname } catch { return '' }
})() : ''

// Chromium args: override DNS for the browser process
const chromiumArgs = clusterIP
  ? [`--host-resolver-rules=MAP ${originalHostname} ${clusterIP}, EXCLUDE localhost`]
  : []

// For the Playwright request context (used in api.spec.ts), override the IP
// via the global `use.baseURL` and add a Host header so Traefik can route.
// The browser project overrides this back to the hostname for page navigation.
const apiBaseURL = clusterIP
  ? E2E_BASE_URL.replace(/^(https?:\/\/)[^/:]+/, `$1${clusterIP}`)
  : E2E_BASE_URL

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  use: {
    baseURL: apiBaseURL,
    ignoreHTTPSErrors: true,
    extraHTTPHeaders: clusterIP ? { host: originalHostname } : {},
  },
  projects: [
    {
      name: 'api',
      testMatch: /api\.spec\.ts/,
      use: {
        // API tests use the IP + Host header approach (no browser)
        baseURL: apiBaseURL,
        extraHTTPHeaders: clusterIP ? { host: originalHostname } : {},
        ignoreHTTPSErrors: true,
      },
    },
    {
      name: 'chromium',
      testIgnore: /api\.spec\.ts/,
      use: {
        ...devices['Desktop Chrome'],
        // Browser tests use the real hostname; DNS is overridden via args
        baseURL: E2E_BASE_URL,
        extraHTTPHeaders: {},
        ignoreHTTPSErrors: true,
        launchOptions: {
          args: chromiumArgs,
        },
      },
    },
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
