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

// configShape: one variant at 100%. Anything else is noise — a config value
// is the same for everyone by definition, so the default variant always wins.
export function configShape(raw: string): FlagShape | { error: string } {
  const parsed = parseConfigValue(raw)
  if ('error' in parsed) return parsed
  return {
    variantsObj: { default: parsed.value },
    splitObj: { default: 100 },
    resolvedDefault: 'default',
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
