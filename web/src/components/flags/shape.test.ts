import { describe, it, expect } from 'vitest'
import {
  parseConfigValue, configFlagType, configShape, booleanShape, stringShape, initialConfigValue,
} from './shape'
import type { FeatureFlag } from '../../lib/api'

describe('parseConfigValue', () => {
  it('keeps numbers as numbers, so a cap is not stored as "250"', () => {
    expect(parseConfigValue('250')).toEqual({ value: 250 })
  })

  it('keeps booleans as booleans', () => {
    expect(parseConfigValue('true')).toEqual({ value: true })
  })

  it('parses objects', () => {
    expect(parseConfigValue('{"mode":"slow"}')).toEqual({ value: { mode: 'slow' } })
  })

  it('takes a bare word as a string — that is what a field labelled Value implies', () => {
    expect(parseConfigValue('slow')).toEqual({ value: 'slow' })
  })

  it('rejects something that looks like JSON but is not, rather than storing the raw text', () => {
    expect(parseConfigValue('{"mode":}')).toEqual({ error: 'Value looks like JSON but does not parse' })
  })

  it('rejects an empty value', () => {
    expect(parseConfigValue('   ')).toEqual({ error: 'Value is required' })
  })
})

describe('configFlagType', () => {
  it('maps a value to the flag_type the dashboard shows', () => {
    expect(configFlagType(true)).toBe('boolean')
    expect(configFlagType(250)).toBe('number')
    expect(configFlagType('slow')).toBe('string')
    expect(configFlagType({ a: 1 })).toBe('json')
  })
})

describe('configShape', () => {
  it('produces a single variant at 100% — a config value is the same for everyone', () => {
    expect(configShape('250')).toEqual({
      variantsObj: { default: 250 },
      splitObj: { default: 100 },
      resolvedDefault: 'default',
      resolvedType: 'number',
    })
  })

  it('passes the parse error through instead of saving garbage', () => {
    expect(configShape('')).toEqual({ error: 'Value is required' })
  })
})

describe('booleanShape', () => {
  it('returns null for a non-boolean flag so the caller falls through', () => {
    expect(booleanShape('string', 'on', false, 0)).toBeNull()
  })

  it('sends 100% to the default when there is no rollout', () => {
    expect(booleanShape('boolean', 'off', false, 0)).toEqual({
      variantsObj: { on: true, off: false },
      splitObj: { off: 100, on: 0 },
      resolvedDefault: 'off',
      resolvedType: 'boolean',
    })
  })

  it('flips the rollout percentage to the opposite of the default', () => {
    expect(booleanShape('boolean', 'off', true, 20)).toEqual({
      variantsObj: { on: true, off: false },
      splitObj: { off: 80, on: 20 },
      resolvedDefault: 'off',
      resolvedType: 'boolean',
    })
  })

  it('treats a 0% rollout as no rollout', () => {
    const shape = booleanShape('boolean', 'on', true, 0)
    expect(shape?.splitObj).toEqual({ on: 100, off: 0 })
  })
})

describe('stringShape', () => {
  const rows = [
    { name: 'control', returnValue: 'control', splitPct: 60 },
    { name: 'treatment', returnValue: 'v2', splitPct: 40 },
  ]

  it('builds variants and split from the rows', () => {
    expect(stringShape(rows, 'control', null)).toEqual({
      variantsObj: { control: 'control', treatment: 'v2' },
      splitObj: { control: 60, treatment: 40 },
      resolvedDefault: 'control',
      resolvedType: 'string',
    })
  })

  it('refuses to save when validation failed', () => {
    expect(stringShape(rows, 'control', 'Splits must add up to 100%'))
      .toEqual({ error: 'Splits must add up to 100%' })
  })
})

describe('initialConfigValue', () => {
  const flag = (variants: string, defaultVariant = 'default') => ({
    variants, default_variant: defaultVariant,
  } as FeatureFlag)

  it('is empty when creating a new flag', () => {
    expect(initialConfigValue(undefined)).toBe('')
  })

  it('pre-fills from the flag default variant so editing shows the real value', () => {
    expect(initialConfigValue(flag('{"default":250}'))).toBe('250')
  })

  it('shows a string value unquoted, matching what the field accepts back', () => {
    expect(initialConfigValue(flag('{"default":"slow"}'))).toBe('slow')
  })

  it('falls back to empty rather than throwing on unparseable variants', () => {
    expect(initialConfigValue(flag('not json'))).toBe('')
  })
})
