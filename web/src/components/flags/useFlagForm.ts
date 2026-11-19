import { useEffect, useState } from 'react'
import { api } from '../../lib/api'
import type { FeatureFlag, TargetingRule } from '../../lib/api'
import { trackEvent } from '../../lib/analytics'
import { parseFlagFormState } from '../../pages/Flags'
import {
  booleanRuleChoices, booleanShape, storedConfigDefault, configBooleanValue, configDefaultName, configTypeOf, initialConfigValue,
  initialConfigVariants, resolveConfigShape, ruleVariants, stringShape,
  type ConfigType, type ConfigVariantRow, type StringVariantRow,
} from './shape'

type FlagFormInitial = ReturnType<typeof parseFlagFormState> | null

// A config flag serves one value to everyone without a matching rule: no bucketing, no evaluation
// rows, no variant/conversion report. See the setup doc's "Two kinds of flag".
function useConfigState(flag?: FeatureFlag) {
  const [flagKind, setFlagKind] = useState<'experiment' | 'config'>(
    flag?.flag_kind === 'config' ? 'config' : 'experiment'
  )
  const [configValue, setConfigValue] = useState(initialConfigValue(flag))
  // Named values beside the default, for targeting rules to serve.
  const [configVariants, setConfigVariants] = useState<ConfigVariantRow[]>(initialConfigVariants(flag))
  // A new config flag chooses its type first; a stored one keeps it.
  const [configType, setConfigType] = useState<ConfigType | null>(configTypeOf(flag))
  const [configBool, setConfigBool] = useState(configBooleanValue(flag))
  return {
    flagKind, setFlagKind, configValue, setConfigValue, configVariants, setConfigVariants,
    configType, setConfigType, configBool, setConfigBool,
    configDefault: configDefaultName(flag),
    // The stored default name of a boolean flag, kept when it is not on or off.
    storedDefault: storedConfigDefault(flag),
    typeLocked: configTypeOf(flag) !== null,
  }
}

function useReleaseState(initial: FlagFormInitial) {
  const [defaultBool, setDefaultBool] = useState<'on' | 'off'>(initial?.defaultBool ?? 'off')
  const [rolloutEnabled, setRolloutEnabled] = useState(initial?.rolloutEnabled ?? false)
  const [rolloutPct, setRolloutPct] = useState(initial?.rolloutPct ?? 0)
  return { defaultBool, setDefaultBool, rolloutEnabled, setRolloutEnabled, rolloutPct, setRolloutPct }
}

function useStringState(initial: FlagFormInitial) {
  const [variants, setVariants] = useState<StringVariantRow[]>(initial?.variants ?? [
    { name: 'control', returnValue: 'control', splitPct: 50 },
    { name: 'treatment', returnValue: 'treatment', splitPct: 50 },
  ])
  const [defaultVariant, setDefaultVariant] = useState(initial?.defaultVariant ?? 'control')
  const [advancedReturnValues, setAdvancedReturnValues] = useState(initial?.advancedReturnValues ?? false)
  const splitSum = variants.reduce((sum, v) => sum + (v.splitPct || 0), 0)
  return { variants, setVariants, defaultVariant, setDefaultVariant, advancedReturnValues, setAdvancedReturnValues, splitSum }
}

function useRulesState(projectId: string, initial: FlagFormInitial) {
  const [targetingRules, setTargetingRules] = useState<TargetingRule[]>(initial?.targetingRules ?? [])
  const [showRules, setShowRules] = useState((initial?.targetingRules?.length ?? 0) > 0)
  const [contextKeySuggestions, setContextKeySuggestions] = useState<{ context_key: string; pct: number }[]>([])
  const [activeKeyInput, setActiveKeyInput] = useState<string | null>(null)
  const [eventNames, setEventNames] = useState<string[]>([])

  useEffect(() => {
    api.getEventNames(projectId).then((d) => setEventNames(d.event_names || [])).catch(() => {})
  }, [projectId])

  useEffect(() => {
    api.getContextKeySuggestions(projectId).then((d) => setContextKeySuggestions(d.suggestions || [])).catch(() => {})
  }, [projectId])

  return {
    targetingRules, setTargetingRules, showRules, setShowRules,
    contextKeySuggestions, activeKeyInput, setActiveKeyInput, eventNames,
  }
}

export const autoKey = (n: string) => n.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')

export function useFlagForm(projectId: string, flag: FeatureFlag | undefined, onSaved: (f: FeatureFlag) => void) {
  const isEdit = flag !== undefined
  const initial = isEdit ? parseFlagFormState(flag) : null

  const config = useConfigState(flag)
  const release = useReleaseState(initial)
  const str = useStringState(initial)
  const rules = useRulesState(projectId, initial)

  const [flagKey, setFlagKey] = useState(flag?.flag_key ?? '')
  const [name, setName] = useState(flag?.name ?? '')
  const [flagType, setFlagType] = useState<'boolean' | 'string'>(
    (flag?.flag_type as 'boolean' | 'string') ?? 'boolean'
  )
  const [conversionEvent, setConversionEvent] = useState(flag?.conversion_event ?? '')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Variant names targeting-rule dropdowns and default-variant selectors can pick from.
  const ruleChoices = ruleVariants({
    flagKind: config.flagKind, flagType, defaultBool: release.defaultBool, variants: str.variants,
    configDefault: config.configDefault, configVariants: config.configVariants,
  })
  // A boolean config flag offers On and Off, and a stored default under another name its rules may use.
  const { variantNames, newRuleVariant: defaultNewVariant } = config.flagKind === 'config' && config.configType === 'boolean'
    ? booleanRuleChoices(config.configBool, config.storedDefault)
    : ruleChoices

  const { setDefaultVariant, defaultVariant } = str
  // Keep defaultVariant in sync when the user removes/renames the row it pointed at.
  useEffect(() => {
    if (config.flagKind !== 'experiment' || flagType !== 'string') return
    if (!variantNames.includes(defaultVariant) && variantNames.length > 0) {
      setDefaultVariant(variantNames[0])
    }
  }, [config.flagKind, flagType, variantNames, defaultVariant, setDefaultVariant])

  const validateString = (): string | null => {
    const { variants, splitSum } = str
    if (variants.length < 2) return 'Need at least two variants'
    const trimmedNames = variants.map((v) => v.name.trim())
    if (trimmedNames.some((n) => !n)) return 'All variants need a name'
    if (new Set(trimmedNames).size !== trimmedNames.length) return 'Variant names must be unique'
    if (splitSum !== 100) return `Splits must add up to 100% (currently ${splitSum}%)`
    return null
  }

  const resolveShape = () => {
    if (config.flagKind === 'config') {
      return resolveConfigShape({
        type: config.configType, bool: config.configBool, storedDefault: config.storedDefault,
        value: config.configValue, rows: config.configVariants, defaultName: config.configDefault,
      })
    }
    return booleanShape(flagType, release.defaultBool, release.rolloutEnabled, release.rolloutPct)
      ?? stringShape(str.variants, str.defaultVariant, validateString())
  }

  const persist = async (body: Parameters<typeof api.createFlag>[1] & { flag_kind: string }) => {
    const f = flag ? await api.updateFlag(projectId, flag.id, body) : await api.createFlag(projectId, body)
    if (!isEdit) trackEvent('flag_created', { flag_key: f.flag_key, flag_type: f.flag_type })
    onSaved(f)
  }

  const save = async () => {
    if (!flagKey.trim() || !name.trim()) { setError('Key and name are required'); return }
    const shape = resolveShape()
    if ('error' in shape) { setError(shape.error); return }
    const { variantsObj, splitObj, resolvedDefault, resolvedType } = shape

    setSaving(true)
    setError(null)
    try {
      const body = {
        flag_key: flagKey,
        name,
        flag_type: resolvedType,
        flag_kind: config.flagKind,
        variants: JSON.stringify(variantsObj),
        default_variant: resolvedDefault,
        split: JSON.stringify(splitObj),
        conversion_event: conversionEvent,
        targeting_rules: rules.targetingRules.length > 0 ? JSON.stringify(rules.targetingRules) : '[]',
      }
      await persist(body)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  return {
    isEdit, config, release, str, rules,
    flagKey, setFlagKey, name, setName, flagType, setFlagType,
    conversionEvent, setConversionEvent, saving, error,
    variantNames, defaultNewVariant, save,
  }
}

export type FlagForm = ReturnType<typeof useFlagForm>
