import { describe, it, expect } from 'vitest'
import { scopeTone, scopeHint, formatLastUsed } from './ApiKeySettings'

describe('scopeTone', () => {
  it('marks scopes that can change state', () => {
    expect(scopeTone('full')).toBe('write')
    expect(scopeTone('flags:write')).toBe('write')
  })

  it('marks send-only and read-only scopes', () => {
    expect(scopeTone('ingest')).toBe('read')
    expect(scopeTone('flags:read')).toBe('read')
    expect(scopeTone('analytics:read')).toBe('read')
  })

  it('treats an unrecognised scope as read rather than implying power it lacks', () => {
    expect(scopeTone('flags:admin')).toBe('read')
  })
})

describe('scopeHint', () => {
  it('says what each scope may do', () => {
    expect(scopeHint('ingest')).toMatch(/Send events/)
    expect(scopeHint('analytics:read')).toMatch(/Cannot change anything/)
    expect(scopeHint('flags:read')).toMatch(/Cannot change/)
    expect(scopeHint('flags:write')).toMatch(/Cannot create or delete/)
    expect(scopeHint('full')).toMatch(/Full API access/)
  })

  it('is explicit that an unknown scope grants nothing', () => {
    expect(scopeHint('flags:admin')).toMatch(/grants nothing/)
  })
})

describe('formatLastUsed', () => {
  it('reports never for a key that has not authenticated', () => {
    expect(formatLastUsed(undefined)).toBe('never')
    expect(formatLastUsed('')).toBe('never')
  })

  it('reports never rather than "Invalid Date" for junk', () => {
    expect(formatLastUsed('not-a-date')).toBe('never')
  })

  it('formats a real timestamp', () => {
    expect(formatLastUsed('2026-08-23T10:00:00Z')).not.toBe('never')
  })
})
