import { describe, it, expect } from 'vitest'
import type { FeatureFlag } from '../../lib/api'
import { applyFlagListQuery, defaultFlagListQuery, flagDate, isFiltered, statusCounts, type FlagListQuery } from './flagListQuery'

function flag(key: string, overrides: Partial<FeatureFlag> = {}): FeatureFlag {
  return {
    id: key, project_id: 'p', flag_key: key, name: key, flag_type: 'boolean',
    variants: '{}', default_variant: 'off', split: '{}', targeting_rules: '[]',
    status: 'active', created_at: '2026-10-01 10:00:00', origin: 'manual', flag_kind: 'experiment',
    ...overrides,
  }
}

const q = (patch: Partial<FlagListQuery>): FlagListQuery => ({ ...defaultFlagListQuery, ...patch })

const flags = [
  flag('checkout_redesign', { name: 'Checkout redesign', created_at: '2026-10-03 09:00:00', last_evaluated_at: '2026-10-28 10:00:00' }),
  flag('mail_paused', { name: 'Mail paused', status: 'paused', flag_kind: 'config', created_at: '2026-10-01 09:00:00' }),
  flag('auto_search', { name: 'auto_search', status: 'inactive', origin: 'auto', created_at: '2026-10-02 09:00:00', last_evaluated_at: '2026-10-29 10:00:00' }),
  flag('legacy', { name: 'Legacy', flag_kind: '', created_at: '2026-09-01T09:00:00Z' }),
]
const keys = (list: FeatureFlag[]) => list.map((f) => f.flag_key)

describe('applyFlagListQuery', () => {
  it('returns every flag, newest first, for the default query', () => {
    expect(keys(applyFlagListQuery(flags, defaultFlagListQuery)))
      .toEqual(['checkout_redesign', 'auto_search', 'mail_paused', 'legacy'])
  })

  it('searches name and key, ignoring case and surrounding spaces', () => {
    expect(keys(applyFlagListQuery(flags, q({ search: '  CHECKOUT ' })))).toEqual(['checkout_redesign'])
    expect(keys(applyFlagListQuery(flags, q({ search: 'mail_' })))).toEqual(['mail_paused'])
  })

  it('filters by status, origin and kind', () => {
    expect(keys(applyFlagListQuery(flags, q({ status: 'paused' })))).toEqual(['mail_paused'])
    expect(keys(applyFlagListQuery(flags, q({ origin: 'auto' })))).toEqual(['auto_search'])
    expect(keys(applyFlagListQuery(flags, q({ origin: 'manual' })))).not.toContain('auto_search')
    expect(keys(applyFlagListQuery(flags, q({ kind: 'config' })))).toEqual(['mail_paused'])
    // A flag without flag_kind counts as an experiment.
    expect(keys(applyFlagListQuery(flags, q({ kind: 'experiment' })))).toContain('legacy')
  })

  it('sorts by name and by last evaluation, never-evaluated last', () => {
    expect(keys(applyFlagListQuery(flags, q({ sort: 'name' }))))
      .toEqual(['auto_search', 'checkout_redesign', 'legacy', 'mail_paused'])
    expect(keys(applyFlagListQuery(flags, q({ sort: 'evaluated' })))).toEqual(['auto_search', 'checkout_redesign', 'mail_paused', 'legacy'])
  })

  it('does not reorder the input array', () => {
    const input = [...flags]
    applyFlagListQuery(input, q({ sort: 'name' }))
    expect(keys(input)).toEqual(keys(flags))
  })
})

describe('statusCounts', () => {
  it('counts every status when nothing else is filtered', () => {
    expect(statusCounts(flags, defaultFlagListQuery)).toEqual({ all: 4, active: 2, paused: 1, inactive: 1 })
  })

  it('applies search, origin and kind but ignores the chosen status', () => {
    expect(statusCounts(flags, q({ status: 'paused', origin: 'manual' }))).toEqual({ all: 3, active: 2, paused: 1, inactive: 0 })
  })
})

describe('isFiltered', () => {
  it('ignores sort and whitespace-only search', () => {
    expect(isFiltered(q({ sort: 'name', search: '   ' }))).toBe(false)
    expect(isFiltered(q({ kind: 'config' }))).toBe(true)
  })
})

describe('flagDate', () => {
  it('reads SQLite and RFC 3339 timestamps as UTC', () => {
    expect(flagDate('2026-10-29 10:31:41')).toBe('2026-10-29 10:31')
    expect(flagDate('2026-10-29T12:31:41+02:00')).toBe('2026-10-29 10:31')
  })

  it('returns null for a missing or unreadable value', () => {
    expect(flagDate(undefined)).toBeNull()
    expect(flagDate('')).toBeNull()
    expect(flagDate('not a date')).toBeNull()
  })
})
