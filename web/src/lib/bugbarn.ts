// BugBarn error reporting utility.
// Config is fetched from /api/v1/client-config at runtime so the Docker image
// is environment-agnostic. Errors arriving before config loads are queued and
// flushed once the endpoint is known.

interface ClientConfig {
  bugbarn_endpoint: string
  bugbarn_ingest_key: string
  bugbarn_project?: string
}

let endpoint = ''
let ingestKey = ''
let project = ''
let configReady = false
const pendingQueue: BugBarnPayload[] = []

async function loadConfig(): Promise<void> {
  try {
    const res = await fetch('/api/v1/client-config')
    if (!res.ok) return
    const cfg: ClientConfig = await res.json()
    endpoint = cfg.bugbarn_endpoint ?? ''
    ingestKey = cfg.bugbarn_ingest_key ?? ''
    project = cfg.bugbarn_project ?? ''
  } catch {
    // Config fetch failed — reporting falls back to console.error.
  } finally {
    configReady = true
    for (const payload of pendingQueue.splice(0)) {
      sendToBugBarn(payload)
    }
  }
}

loadConfig()

export interface BugBarnPayload {
  name: string
  properties: Record<string, unknown>
}

function sendToBugBarn(payload: BugBarnPayload): void {
  if (!endpoint || !ingestKey) {
    console.error('[BugBarn not configured]', payload.name, payload.properties)
    return
  }
  try {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-BugBarn-Api-Key': ingestKey,
    }
    if (project) headers['X-BugBarn-Project'] = project
    fetch(`${endpoint}/api/v1/events`, {
      method: 'POST',
      headers,
      body: JSON.stringify(payload),
      keepalive: true,
    }).catch(() => {})
  } catch {
    // Never throw from error reporting.
  }
}

export function reportToBugBarn(payload: BugBarnPayload): void {
  if (!configReady) {
    pendingQueue.push(payload)
    return
  }
  sendToBugBarn(payload)
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
