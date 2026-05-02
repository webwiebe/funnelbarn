// Self-tracking analytics for dogfooding FunnelBarn.
// Config is fetched from /api/v1/client-config at runtime.
// Falls back to no-op when unconfigured.

let endpoint = ''
let apiKey = ''
let projectName = ''
let initialized = false

const SESSION_KEY = 'funnelbarn_dogfood_sid'
const SESSION_EXPIRY_KEY = 'funnelbarn_dogfood_sid_exp'
const SESSION_TIMEOUT = 30 * 60 * 1000

async function loadConfig(): Promise<void> {
  if (initialized) return
  initialized = true
  try {
    const res = await fetch('/api/v1/client-config')
    if (!res.ok) return
    const cfg = await res.json()
    endpoint = cfg.funnelbarn_endpoint ?? ''
    apiKey = cfg.funnelbarn_api_key ?? ''
    projectName = cfg.funnelbarn_project ?? ''
  } catch {
    // Config fetch failed — tracking disabled.
  }
}

loadConfig()

function getSessionId(): string {
  try {
    const now = Date.now()
    const expiry = parseInt(localStorage.getItem(SESSION_EXPIRY_KEY) ?? '0', 10)
    if (now < expiry) {
      localStorage.setItem(SESSION_EXPIRY_KEY, String(now + SESSION_TIMEOUT))
      return localStorage.getItem(SESSION_KEY) ?? newSessionId()
    }
    const id = newSessionId()
    localStorage.setItem(SESSION_KEY, id)
    localStorage.setItem(SESSION_EXPIRY_KEY, String(now + SESSION_TIMEOUT))
    return id
  } catch {
    return ''
  }
}

function newSessionId(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

function extractUTMs(): Record<string, string> {
  const params = new URLSearchParams(window.location.search)
  const utms: Record<string, string> = {}
  for (const key of ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content']) {
    const val = params.get(key)
    if (val) utms[key] = val
  }
  return utms
}

function send(name: string, properties?: Record<string, unknown>): void {
  if (!endpoint || !apiKey) return
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'x-funnelbarn-api-key': apiKey,
  }
  if (projectName) {
    headers['x-funnelbarn-project'] = projectName
  }
  const payload: Record<string, unknown> = {
    name,
    url: window.location.href,
    referrer: document.referrer || undefined,
    session_id: getSessionId(),
    timestamp: new Date().toISOString(),
    ...extractUTMs(),
  }
  if (properties && Object.keys(properties).length > 0) {
    payload.properties = properties
  }
  try {
    fetch(`${endpoint}/api/v1/events`, {
      method: 'POST',
      headers,
      body: JSON.stringify(payload),
      keepalive: true,
    }).catch(() => {})
  } catch {
    // Never throw from analytics.
  }
}

export function trackPageView(): void {
  send('page_view')
}

export function trackEvent(name: string, properties?: Record<string, unknown>): void {
  send(name, properties)
}
