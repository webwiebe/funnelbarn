// Flag form shapes: the pure translation between what the modal's fields hold
// and what the API takes (variants and split as encoded JSON objects, plus the
// resolved default variant and the inferred flag_type).
//
// Kept out of FlagFormModal.tsx so it can be tested without rendering, and so
// the modal stays a component file.

import type { FeatureFlag } from '../../lib/api'

export interface StringVariantRow {
  name: string
  returnValue: string
  splitPct: number
}

// parseConfigValue reads a config flag's value the way the evaluate API does:
// as JSON, so numbers stay numbers and objects stay objects. A bare word that
// isn't valid JSON is taken as a string, which is what people expect from a
// field labelled "Value".
export function parseConfigValue(raw: string): { value: unknown } | { error: string } {
  const trimmed = raw.trim()
  if (trimmed === '') return { error: 'Value is required' }
  try {
    return { value: JSON.parse(trimmed) }
  } catch {
    if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
      return { error: 'Value looks like JSON but does not parse' }
    }
    return { value: trimmed }
  }
}

// FlagShape is the wire representation the API takes: variants and split as
// already-encoded JSON objects, plus the resolved default and inferred type.
export type FlagShape = {
  variantsObj: Record<string, unknown>
  splitObj: Record<string, number>
  resolvedDefault: string
  resolvedType: string
}

// ConfigVariantRow is a named value a config flag holds beside its default,
// for a targeting rule to serve. value is the raw text of the field.
export interface ConfigVariantRow {
  name: string
  value: string
}

// configShape: the default variant at 100%, plus the named variants a targeting
// rule can serve. Without a matching rule everyone gets the default, so the
// split never names the others. defaultName is the stored default variant, kept
// so rules and clients that name it keep working.
export function configShape(
  raw: string, extras: ConfigVariantRow[] = [], defaultName = 'default', type?: ValueConfigType,
): FlagShape | { error: string } {
  const read = (text: string) => (type ? parseTypedValue(type, text) : parseConfigValue(text))
  const parsed = read(raw)
  if ('error' in parsed) return parsed
  const variantsObj: Record<string, unknown> = { [defaultName]: parsed.value }
  // Row 1 is the default; the named values follow, so an error can name the row it collides with.
  const rowOf: Record<string, number> = { [defaultName]: 1 }
  for (const [i, row] of extras.entries()) {
    const name = row.name.trim()
    if (!name) return { error: 'All variants need a name' }
    if (name in rowOf) {
      const first = rowOf[name]
      return { error: `Row ${i + 2} repeats the name "${name}" of row ${first}${first === 1 ? ' (the default value)' : ''}` }
    }
    const value = read(row.value)
    if ('error' in value) return { error: `Variant "${name}": ${value.error}` }
    variantsObj[name] = value.value
    rowOf[name] = i + 2
  }
  return {
    variantsObj,
    splitObj: { [defaultName]: 100 },
    resolvedDefault: defaultName,
    resolvedType: type ? CONFIG_FLAG_TYPE[type] : configFlagType(parsed.value),
  }
}

// ConfigType is what a config flag's value is. A boolean is one On/Off switch
// with the fixed variants on and off; the others hold a value and named values.
export type ConfigType = 'boolean' | 'text' | 'number' | 'json'
export type ValueConfigType = Exclude<ConfigType, 'boolean'>

const CONFIG_FLAG_TYPE: Record<ConfigType, string> = {
  boolean: 'boolean', text: 'string', number: 'number', json: 'json',
}

// configTypeOf is the type of a stored config flag, or null for a new flag or
// an experiment, which has not chosen one yet.
export function configTypeOf(flag?: FeatureFlag): ConfigType | null {
  if (flag?.flag_kind !== 'config') return null
  switch (flag.flag_type) {
    case 'boolean': return 'boolean'
    case 'number': return 'number'
    case 'json': return 'json'
    default: return 'text'
  }
}

// parseTypedValue reads the text of a value field as the chosen type.
export function parseTypedValue(type: ValueConfigType, raw: string): { value: unknown } | { error: string } {
  const trimmed = raw.trim()
  if (type === 'text') return raw === '' ? { error: 'Value is required' } : { value: raw }
  if (trimmed === '') return { error: 'Value is required' }
  if (type === 'number') {
    const n = Number(trimmed)
    return Number.isFinite(n) ? { value: n } : { error: 'Value must be a number' }
  }
  try {
    return { value: JSON.parse(trimmed) }
  } catch {
    return { error: 'Value must be valid JSON' }
  }
}

// configBooleanValue is what a boolean config flag serves without a matching
// rule: the value its default variant holds, off when the variant is missing.
export function configBooleanValue(flag?: FeatureFlag): boolean {
  if (!flag) return false
  try {
    return (JSON.parse(flag.variants) as Record<string, unknown>)[flag.default_variant] === true
  } catch {
    return false
  }
}

// booleanConfigShape stores a boolean config flag as the fixed variants on: true
// and off: false. The no-rule value is the default variant: on or off, or, when
// the flag's default has another name that clients and rules use, that name
// holding the chosen value.
export function booleanConfigShape(on: boolean, defaultName: string): FlagShape {
  const custom = defaultName !== '' && defaultName !== 'on' && defaultName !== 'off'
  const name = custom ? defaultName : (on ? 'on' : 'off')
  return {
    variantsObj: { on: true, off: false, ...(custom ? { [defaultName]: on } : {}) },
    splitObj: { [name]: 100 },
    resolvedDefault: name,
    resolvedType: 'boolean',
  }
}

// toggleConfigBoolean flips what a boolean config flag serves without a rule,
// as the fields the header toggle writes. The variants stay the fixed on/off
// pair and the default never points at a variant the flag does not have.
export function toggleConfigBoolean(flag: FeatureFlag): { variants: string; default_variant: string; split: string } {
  const shape = booleanConfigShape(!configBooleanValue(flag), flag.default_variant)
  return {
    variants: JSON.stringify(shape.variantsObj),
    default_variant: shape.resolvedDefault,
    split: JSON.stringify(shape.splitObj),
  }
}

// booleanShape returns null when the flag isn't a boolean, so the caller can
// fall through to stringShape.
export function booleanShape(
  flagType: string, defaultBool: 'on' | 'off', rolloutEnabled: boolean, rolloutPct: number,
): FlagShape | null {
  if (flagType !== 'boolean') return null
  // No rollout -> 100% to default. With rollout -> that % goes to the
  // *opposite* of the default ("flip X% of users"), the rest stays default.
  const opposite = defaultBool === 'on' ? 'off' : 'on'
  const splitObj = !rolloutEnabled || rolloutPct === 0
    ? { [defaultBool]: 100, [opposite]: 0 }
    : { [defaultBool]: 100 - rolloutPct, [opposite]: rolloutPct }
  return { variantsObj: { on: true, off: false }, splitObj, resolvedDefault: defaultBool, resolvedType: 'boolean' }
}

export function stringShape(
  variants: StringVariantRow[], defaultVariant: string, validationError: string | null,
): FlagShape | { error: string } {
  if (validationError) return { error: validationError }
  return {
    variantsObj: Object.fromEntries(variants.map((v) => [v.name.trim(), v.returnValue])),
    splitObj: Object.fromEntries(variants.map((v) => [v.name.trim(), v.splitPct])),
    resolvedDefault: defaultVariant,
    resolvedType: 'string',
  }
}

export function configFlagType(value: unknown): string {
  if (typeof value === 'boolean') return 'boolean'
  if (typeof value === 'number') return 'number'
  if (typeof value === 'string') return 'string'
  return 'json'
}

// initialConfigValue pre-fills the editor from the flag's stored default
// variant, so editing a config flag shows the value it actually holds.
export function initialConfigValue(flag?: FeatureFlag): string {
  if (!flag) return ''
  try {
    const variants = JSON.parse(flag.variants) as Record<string, unknown>
    const value = variants[flag.default_variant]
    return typeof value === 'string' ? value : JSON.stringify(value ?? '')
  } catch {
    return ''
  }
}

// configDefaultName is the variant a config flag serves without a rule: the
// stored one when editing, "default" for a new flag.
export function configDefaultName(flag?: FeatureFlag): string {
  return flag?.flag_kind === 'config' && flag.default_variant ? flag.default_variant : 'default'
}

// initialConfigVariants pre-fills the named variants of a config flag: every
// stored variant except the default, in stored order.
export function initialConfigVariants(flag?: FeatureFlag): ConfigVariantRow[] {
  if (flag?.flag_kind !== 'config') return []
  try {
    const variants = JSON.parse(flag.variants) as Record<string, unknown>
    return Object.entries(variants)
      .filter(([name]) => name !== flag.default_variant)
      .map(([name, value]) => ({ name, value: typeof value === 'string' ? value : JSON.stringify(value) }))
  } catch {
    return []
  }
}

// ruleVariants returns the variant names a targeting rule can serve, and the one
// a new rule starts on. A config flag offers its default and its named values,
// and a new rule starts on the first named value, since serving the default
// changes nothing.
export function ruleVariants(f: {
  flagKind: 'experiment' | 'config'
  flagType: string
  defaultBool: 'on' | 'off'
  variants: StringVariantRow[]
  configDefault: string
  configVariants: ConfigVariantRow[]
}): { variantNames: string[]; newRuleVariant: string } {
  if (f.flagKind === 'config') {
    const named = f.configVariants.map((v) => v.name.trim()).filter((n) => n !== '')
    return { variantNames: [f.configDefault, ...named], newRuleVariant: named[0] ?? f.configDefault }
  }
  if (f.flagType === 'boolean') {
    return { variantNames: ['on', 'off'], newRuleVariant: f.defaultBool === 'on' ? 'off' : 'on' }
  }
  const names = f.variants.map((v) => v.name).filter((n) => n.trim() !== '')
  return { variantNames: names, newRuleVariant: names[0] || '' }
}

// booleanRuleChoices lists the names a targeting rule of a boolean config flag can
// serve, and the one a new rule starts on (the opposite of the default).
export function booleanRuleChoices(on: boolean, storedDefault: string) {
  const custom = storedDefault && storedDefault !== 'on' && storedDefault !== 'off' ? [storedDefault] : []
  return { variantNames: ['on', 'off', ...custom], newRuleVariant: on ? 'off' : 'on' }
}

// resolveConfigShape builds the stored shape of a config flag from the dialog state.
export function resolveConfigShape(a: {
  type: ConfigType | null; bool: boolean; storedDefault: string
  value: string; rows: ConfigVariantRow[]; defaultName: string
}) {
  if (a.type === null) return { error: 'Choose the value type first' }
  return a.type === 'boolean'
    ? booleanConfigShape(a.bool, a.storedDefault)
    : configShape(a.value, a.rows, a.defaultName, a.type)
}

// storedConfigDefault is the default variant name a stored config flag carries.
export function storedConfigDefault(flag?: FeatureFlag): string {
  return flag?.flag_kind === 'config' ? flag.default_variant : ''
}
