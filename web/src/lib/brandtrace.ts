// Loads the BrandTrace site SDK for the dashboard when the server hands out a
// site key through /api/v1/client-config. An empty key keeps the SDK off.

const SDK_URL = 'https://brandtrace.net/sdk.js'

export function loadBrandTrace(siteKey: unknown): void {
  if (typeof siteKey !== 'string' || siteKey === '') return
  if (document.querySelector(`script[src="${SDK_URL}"]`)) return
  const script = document.createElement('script')
  script.src = SDK_URL
  script.defer = true
  script.dataset.siteKey = siteKey
  document.head.appendChild(script)
}
