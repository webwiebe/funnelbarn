// BugBarn error reporting utility.
// Reads VITE_BUGBARN_ENDPOINT and VITE_BUGBARN_INGEST_KEY at build time.
// Falls back to console.error only when env vars are not configured.

const endpoint = import.meta.env.VITE_BUGBARN_ENDPOINT as string | undefined
const ingestKey = import.meta.env.VITE_BUGBARN_INGEST_KEY as string | undefined

const configured = Boolean(endpoint && ingestKey)

export interface BugBarnPayload {
  name: string
  properties: Record<string, unknown>
}

/**
 * Report an error to BugBarn. If the env vars are not set, falls back to
 * console.error so local development is unaffected.
 */
export function reportToBugBarn(payload: BugBarnPayload): void {
  if (!configured) {
    console.error('[BugBarn not configured]', payload.name, payload.properties)
    return
  }

  // Fire-and-forget: never let reporting itself throw.
  try {
    fetch(`${endpoint}/api/v1/ingest`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-BugBarn-Api-Key': ingestKey!,
      },
      body: JSON.stringify(payload),
      // keepalive so the request survives page unload (e.g. for onerror)
      keepalive: true,
    }).catch(() => {
      // Silently swallow — we must never throw from error reporting.
    })
  } catch {
    // Defensive: JSON.stringify can theoretically throw for circular refs.
  }
}

/**
 * Convenience wrapper that extracts message and stack from an Error (or any
 * thrown value) and sends it to BugBarn.
 */
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
      url: window.location.href,
      ...context,
    },
  })
}
