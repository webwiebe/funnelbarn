import { describe, it, expect } from 'vitest'
import {
  parseConfigValue, configFlagType, configShape, booleanShape, stringShape, initialConfigValue,
  configDefaultName, initialConfigVariants, ruleVariants,
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

  it('keeps named values for rules beside the default, and splits only on the default', () => {
    expect(configShape('false', [{ name: ' on ', value: 'true' }], 'default')).toEqual({
      variantsObj: { default: false, on: true },
      splitObj: { default: 100 },
      resolvedDefault: 'default',
      resolvedType: 'boolean',
    })
  })

  it('keeps a stored default name other than "default"', () => {
    expect(configShape('1', [{ name: 'fast', value: '5' }], 'base')).toMatchObject({
      variantsObj: { base: 1, fast: 5 },
      splitObj: { base: 100 },
      resolvedDefault: 'base',
    })
  })

  it('rejects an empty, repeated or unparsable named value', () => {
    expect(configShape('1', [{ name: ' ', value: '2' }])).toEqual({ error: 'All variants need a name' })
    expect(configShape('1', [{ name: 'default', value: '2' }])).toEqual({ error: 'Variant names must be unique ("default")' })
    expect(configShape('1', [{ name: 'a', value: '2' }, { name: 'a', value: '3' }])).toEqual({ error: 'Variant names must be unique ("a")' })
    expect(configShape('1', [{ name: 'a', value: '{"x":' }])).toEqual({ error: 'Variant "a": Value looks like JSON but does not parse' })
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

describe('config flag variants', () => {
  const config = {
    id: 'f', project_id: 'p', flag_key: 'k', name: 'n', flag_type: 'boolean',
    variants: '{"default":false,"on":true,"label":"x"}', default_variant: 'default', split: '{"default":100}',
    targeting_rules: '[]', status: 'active', created_at: '', origin: 'manual', flag_kind: 'config',
  } as FeatureFlag

  it('reads every stored variant except the default', () => {
    expect(initialConfigVariants(config)).toEqual([{ name: 'on', value: 'true' }, { name: 'label', value: 'x' }])
    expect(initialConfigVariants({ ...config, flag_kind: 'experiment' })).toEqual([])
    expect(initialConfigVariants({ ...config, variants: 'not json' })).toEqual([])
    expect(initialConfigVariants(undefined)).toEqual([])
  })

  it('keeps the stored default name, and uses "default" for a new flag', () => {
    expect(configDefaultName({ ...config, default_variant: 'base' })).toBe('base')
    expect(configDefaultName({ ...config, flag_kind: 'experiment', default_variant: 'on' })).toBe('default')
    expect(configDefaultName(undefined)).toBe('default')
  })
})

describe('ruleVariants', () => {
  const base = {
    flagKind: 'experiment' as const, flagType: 'boolean', defaultBool: 'off' as const,
    variants: [{ name: 'control', returnValue: 'control', splitPct: 50 }, { name: ' ', returnValue: '', splitPct: 50 }],
    configDefault: 'default', configVariants: [{ name: ' on ', value: 'true' }, { name: '', value: '' }],
  }

  it('offers a config flag its default and named values, starting a rule on the first named one', () => {
    expect(ruleVariants({ ...base, flagKind: 'config' })).toEqual({ variantNames: ['default', 'on'], newRuleVariant: 'on' })
    expect(ruleVariants({ ...base, flagKind: 'config', configVariants: [] })).toEqual({ variantNames: ['default'], newRuleVariant: 'default' })
  })

  it('starts a boolean rule on the opposite of the default', () => {
    expect(ruleVariants(base)).toEqual({ variantNames: ['on', 'off'], newRuleVariant: 'on' })
  })

  it('offers a string flag its named variants', () => {
    expect(ruleVariants({ ...base, flagType: 'string' })).toEqual({ variantNames: ['control'], newRuleVariant: 'control' })
  })
})
