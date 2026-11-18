import { reportError } from './bugbarn'
import { ApiError } from './api-error'

function getCookie(name: string): string | undefined {
  const match = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : undefined
}

// Transient statuses we'll retry once. 502/503/504 are almost always a
// k8s rollout in flight (pod cycling) — they recover in a couple of seconds.
const TRANSIENT_STATUSES = new Set([502, 503, 504])

const MUTATING_METHODS = new Set(['POST', 'PUT', 'DELETE', 'PATCH'])

export function buildHeaders(options: RequestInit, method: string): Record<string, string> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json', ...options.headers as Record<string, string> }
  if (MUTATING_METHODS.has(method)) {
    const csrfToken = getCookie('funnelbarn_csrf')
    if (csrfToken) {
      headers['X-FunnelBarn-CSRF'] = csrfToken
    }
  }
  return headers
}

export async function fetchWithRetry(doFetch: () => Promise<Response>, method: string): Promise<Response> {
  let res: Response
  try {
    res = await doFetch()
  } catch (e) {
    // TypeError: Failed to fetch — browser-level network failure. Don't
    // report (it's the user's network, not our code). Surface as an
    // ApiError so callers can render a "lost connection" state if they want.
    throw new ApiError(0, e instanceof Error ? e.message : 'network error')
  }

  // One-shot retry on transient 5xx after a short delay. Don't retry mutating
  // methods — the request may have already partially applied server-side.
  if (TRANSIENT_STATUSES.has(res.status) && method === 'GET') {
    await new Promise((r) => setTimeout(r, 800))
    try {
      res = await doFetch()
    } catch {
      // Same network-failure rationale as above.
      throw new ApiError(0, 'network error after retry')
    }
  }
  return res
}

async function errorMessage(res: Response): Promise<string> {
  try {
    const body = await res.json()
    if (body.error) return body.error
  } catch {
    // ignore parse errors
  }
  return `HTTP ${res.status}`
}

export async function failFor(res: Response, path: string): Promise<never> {
  if (res.status === 401) {
    const { pathname } = window.location
    if (pathname !== '/' && !pathname.startsWith('/login')) {
      window.location.href = '/login'
    }
    throw new ApiError(401, 'Unauthorized')
  }
  const apiErr = new ApiError(res.status, await errorMessage(res))
  // Report unexpected server errors (5xx) to BugBarn — but skip the transient
  // statuses we just retried. If we retried and still got 503, it's likely a
  // longer rollout or real outage; we still don't want a BugBarn spam storm
  // for either case, so suppress those too.
  if (res.status >= 500 && !TRANSIENT_STATUSES.has(res.status)) {
    reportError(apiErr, {
      source: 'api.request',
      path,
      status: res.status,
    })
  }
  throw apiErr
}
