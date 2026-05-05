// BugBarn error reporting utility.
// Config is fetched from /api/v1/client-config at runtime so the Docker image
// is environment-agnostic. Falls back to console.error only when unconfigured.

interface ClientConfig {
  bugbarn_endpoint: string
  bugbarn_ingest_key: string
}

let endpoint = ''
let ingestKey = ''
let initialized = false

async function loadConfig(): Promise<void> {
  if (initialized) return
  initialized = true
  try {
    const res = await fetch('/api/v1/client-config')
    if (!res.ok) return
    const cfg: ClientConfig = await res.json()
    endpoint = cfg.bugbarn_endpoint ?? ''
    ingestKey = cfg.bugbarn_ingest_key ?? ''
  } catch {
    // Config fetch failed — reporting falls back to console.error.
  }
}

// Kick off config fetch immediately so it's ready before the first error.
loadConfig()

export interface BugBarnPayload {
  name: string
  properties: Record<string, unknown>
}

export function reportToBugBarn(payload: BugBarnPayload): void {
  if (!endpoint || !ingestKey) {
    console.error('[BugBarn not configured]', payload.name, payload.properties)
    return
  }
  try {
    fetch(`${endpoint}/api/v1/events`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-BugBarn-Api-Key': ingestKey,
      },
      body: JSON.stringify(payload),
      keepalive: true,
    }).catch(() => {})
  } catch {
    // Never throw from error reporting.
  }
}

export function reportError(
  error: unknown,
  context: Record<string, unknown> = {},
): void {
  const message = error instanceof Error ? error.message : String(error)
  const stack = error instanceof Error ? (error.stack ?? '') : ''
  reportToBugBarn({
    name: 'error',
    properties: {
      message,
      stack,
      url: typeof window !== 'undefined' ? window.location.href : '',
      ...context,
    },
  })
}
