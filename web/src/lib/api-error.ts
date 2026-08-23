/**
 * ApiError lives in its own module so the error reporter can recognise it
 * without importing the API client — api.ts already imports the reporter, and
 * a cycle between them makes `instanceof` depend on module evaluation order.
 */
export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

/**
 * isNonActionableError reports whether a failure is a condition of the user's
 * session or network rather than a defect in our code, and so should not become
 * a BugBarn issue.
 *
 *  - status 0 — the fetch itself threw (offline, DNS, TLS, connection reset).
 *    The API client already documents this as "the user's network, not our
 *    code"; this is what makes every call site honour that.
 *  - status 401 — the session expired. api.request redirects to /login, so it
 *    is an expected transition, not a fault.
 *  - a bare fetch TypeError, for paths that call fetch directly and never wrap
 *    it in an ApiError.
 *
 * Deliberately narrow: 4xx other than 401 and every 5xx still report, because
 * those are ours.
 *
 * Matched structurally rather than with `instanceof`. An error that crossed a
 * bundle or realm boundary carries a different class identity for the same
 * shape, and failing open there would report exactly the noise this exists to
 * suppress.
 */
export function isNonActionableError(error: unknown): boolean {
  if (typeof error !== 'object' || error === null) return false
  const { name, message, status } = error as { name?: unknown; message?: unknown; status?: unknown }

  if (name === 'ApiError' && typeof status === 'number') {
    return status === 0 || status === 401
  }
  if (name === 'TypeError' && typeof message === 'string') {
    const m = message.toLowerCase()
    // "Failed to fetch" (Chromium), "Load failed" (Safari),
    // "NetworkError when attempting to fetch resource" (Firefox).
    return m.includes('failed to fetch') || m.includes('load failed') || m.includes('networkerror')
  }
  return false
}
