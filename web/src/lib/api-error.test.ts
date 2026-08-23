import { describe, it, expect } from 'vitest'
import { ApiError, isNonActionableError } from './api-error'

describe('isNonActionableError', () => {
  it('skips a browser-level network failure (status 0)', () => {
    // The API client wraps a thrown fetch as ApiError(0) and documents it as
    // "the user's network, not our code" — this is what makes every call site
    // honour that instead of only the two that remembered to.
    expect(isNonActionableError(new ApiError(0, 'Failed to fetch'))).toBe(true)
  })

  it('skips a 401 — api.request already redirects to /login', () => {
    expect(isNonActionableError(new ApiError(401, 'unauthorized'))).toBe(true)
  })

  it('reports every other status, because those are ours', () => {
    for (const status of [400, 403, 404, 409, 422, 429, 500, 502, 503]) {
      expect(isNonActionableError(new ApiError(status, 'boom'))).toBe(false)
    }
  })

  it('skips a bare fetch TypeError across browsers', () => {
    expect(isNonActionableError(new TypeError('Failed to fetch'))).toBe(true)
    expect(isNonActionableError(new TypeError('Load failed'))).toBe(true)
    expect(isNonActionableError(
      new TypeError('NetworkError when attempting to fetch resource.'))).toBe(true)
  })

  it('does not swallow an ordinary TypeError — that is a real defect', () => {
    expect(isNonActionableError(
      new TypeError("Cannot read properties of null (reading 'map')"))).toBe(false)
  })

  it('reports anything that is not an error object at all', () => {
    expect(isNonActionableError(new Error('boom'))).toBe(false)
    expect(isNonActionableError('boom')).toBe(false)
    expect(isNonActionableError(null)).toBe(false)
    expect(isNonActionableError(undefined)).toBe(false)
  })
})

describe('cross-boundary identity', () => {
  it('recognises an ApiError-shaped object from another module instance', () => {
    // Two copies of the class (separate bundles, or a vi.resetModules reload)
    // make `instanceof` false for the same shape. Failing open there would
    // report exactly the noise this filter exists to suppress.
    const fromElsewhere = Object.assign(new Error('Failed to fetch'), {
      name: 'ApiError',
      status: 0,
    })
    expect(isNonActionableError(fromElsewhere)).toBe(true)
  })

  it('does not treat an unrelated object with a status as an ApiError', () => {
    expect(isNonActionableError({ name: 'Error', status: 0 })).toBe(false)
    expect(isNonActionableError({ status: 401 })).toBe(false)
  })
})
