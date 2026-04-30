import { describe, it, expect } from 'vitest'
import { ApiError } from './api'

describe('ApiError', () => {
  it('has the correct message', () => {
    const err = new ApiError(404, 'Not found')
    expect(err.message).toBe('Not found')
  })

  it('has the correct status code', () => {
    const err = new ApiError(403, 'Forbidden')
    expect(err.status).toBe(403)
  })

  it('is an instance of Error', () => {
    const err = new ApiError(500, 'Server error')
    expect(err).toBeInstanceOf(Error)
  })

  it('has name set to ApiError', () => {
    const err = new ApiError(401, 'Unauthorized')
    expect(err.name).toBe('ApiError')
  })
})
