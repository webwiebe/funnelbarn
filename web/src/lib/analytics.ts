// Self-tracking analytics — dogfooding FunnelBarn with the real SDK.
// Config is fetched from /api/v1/client-config at runtime.
// Falls back to no-op when unconfigured.

import { FunnelBarnClient } from '@funnelbarn/js'

let client: FunnelBarnClient | null = null
let configLoaded = false

async function loadConfig(): Promise<void> {
  if (configLoaded) return
  configLoaded = true
  try {
    const res = await fetch('/api/v1/client-config')
    if (!res.ok) return
    const cfg = await res.json()
    const endpoint: string = cfg.funnelbarn_endpoint ?? ''
    const apiKey: string = cfg.funnelbarn_api_key ?? ''
    const projectName: string = cfg.funnelbarn_project ?? ''
    if (!endpoint || !apiKey) return
    client = new FunnelBarnClient({ endpoint, apiKey, projectName })
  } catch {
    // Config fetch failed — tracking disabled.
  }
}

const configPromise = loadConfig()

export function trackPageView(): void {
  configPromise.then(() => client?.page()).catch(() => {})
}

export function trackEvent(name: string, properties?: Record<string, unknown>): void {
  configPromise.then(() => client?.track(name, properties)).catch(() => {})
}

export function identifyUser(userId: string): void {
  configPromise.then(() => client?.identify(userId)).catch(() => {})
}
