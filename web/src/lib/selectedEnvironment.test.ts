import { beforeEach, describe, expect, it } from 'vitest'
import { DEFAULT_ENVIRONMENT, readStoredEnvironment, writeStoredEnvironment } from './selectedEnvironment'

describe('selectedEnvironment', () => {
  beforeEach(() => localStorage.clear())

  it('defaults to production when nothing is stored', () => {
    expect(readStoredEnvironment()).toBe(DEFAULT_ENVIRONMENT)
    expect(DEFAULT_ENVIRONMENT).toBe('production')
  })

  it('returns a stored environment', () => {
    writeStoredEnvironment('staging')
    expect(readStoredEnvironment()).toBe('staging')
  })

  it('remembers All environments across a reload', () => {
    writeStoredEnvironment('')
    expect(localStorage.getItem('funnelbarn_environment')).toBe('all')
    expect(readStoredEnvironment()).toBe('')
  })
})
