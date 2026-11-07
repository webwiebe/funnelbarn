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
  raw: string, extras: ConfigVariantRow[] = [], defaultName = 'default', booleanOnly = false,
): FlagShape | { error: string } {
  const parsed = parseConfigValue(raw)
  if ('error' in parsed) return parsed
  if (booleanOnly && typeof parsed.value !== 'boolean') {
    return { error: 'This flag is a boolean: the value must be true or false' }
  }
  const variantsObj: Record<string, unknown> = { [defaultName]: parsed.value }
  for (const row of extras) {
    const name = row.name.trim()
    if (!name) return { error: 'All variants need a name' }
    if (name in variantsObj) return { error: `Variant names must be unique ("${name}")` }
    const value = parseConfigValue(row.value)
    if ('error' in value) return { error: `Variant "${name}": ${value.error}` }
    variantsObj[name] = value.value
  }
  return {
    variantsObj,
    splitObj: { [defaultName]: 100 },
    resolvedDefault: defaultName,
    resolvedType: configFlagType(parsed.value),
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

// flipConfigBoolean returns the variants JSON of a boolean config flag with the
// value its default variant holds inverted. A default variant the flag does not
// have is created as true, so the toggle never leaves the flag without a value.
export function flipConfigBoolean(variantsJSON: string, defaultVariant: string): string {
  let variants: Record<string, unknown>
  try { variants = JSON.parse(variantsJSON) as Record<string, unknown> } catch { variants = {} }
  return JSON.stringify({ ...variants, [defaultVariant]: variants[defaultVariant] !== true })
}
