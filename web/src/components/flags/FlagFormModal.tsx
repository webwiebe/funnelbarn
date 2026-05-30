import { useEffect, useState } from 'react'
import { Plus, X, ChevronDown, ChevronRight } from 'lucide-react'
import { api } from '../../lib/api'
import type { FeatureFlag, TargetingRule } from '../../lib/api'
import { trackEvent } from '../../lib/analytics'
import { parseFlagFormState } from '../../pages/Flags'
import { TargetingRuleEditor } from './TargetingRuleEditor'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  success: '#10b981',
  error: '#ef4444',
}

interface StringVariantRow {
  name: string
  returnValue: string
  splitPct: number
}

interface FlagFormModalProps {
  projectId: string
  flag?: FeatureFlag
  onClose: () => void
  onSaved: (f: FeatureFlag) => void
}

export function FlagFormModal({ projectId, flag, onClose, onSaved }: FlagFormModalProps) {
  const isEdit = flag !== undefined
  const initial = isEdit ? parseFlagFormState(flag) : null

  const [flagKey, setFlagKey] = useState(flag?.flag_key ?? '')
  const [name, setName] = useState(flag?.name ?? '')
  const [flagType, setFlagType] = useState<'boolean' | 'string'>(
    (flag?.flag_type as 'boolean' | 'string') ?? 'boolean'
  )

  // Boolean (release) state
  const [defaultBool, setDefaultBool] = useState<'on' | 'off'>(initial?.defaultBool ?? 'off')
  const [rolloutEnabled, setRolloutEnabled] = useState(initial?.rolloutEnabled ?? false)
  const [rolloutPct, setRolloutPct] = useState(initial?.rolloutPct ?? 0)

  // String (experiment) state
  const [variants, setVariants] = useState<StringVariantRow[]>(initial?.variants ?? [
    { name: 'control', returnValue: 'control', splitPct: 50 },
    { name: 'treatment', returnValue: 'treatment', splitPct: 50 },
  ])
  const [defaultVariant, setDefaultVariant] = useState(initial?.defaultVariant ?? 'control')
  const [advancedReturnValues, setAdvancedReturnValues] = useState(initial?.advancedReturnValues ?? false)

  const [conversionEvent, setConversionEvent] = useState(flag?.conversion_event ?? '')
  const [eventNames, setEventNames] = useState<string[]>([])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [targetingRules, setTargetingRules] = useState<TargetingRule[]>(initial?.targetingRules ?? [])
  const [showRules, setShowRules] = useState((initial?.targetingRules?.length ?? 0) > 0)
  const [contextKeySuggestions, setContextKeySuggestions] = useState<{ context_key: string; pct: number }[]>([])
  const [activeKeyInput, setActiveKeyInput] = useState<string | null>(null)

  useEffect(() => {
    api.getEventNames(projectId).then((d) => setEventNames(d.event_names || [])).catch(() => {})
  }, [projectId])

  useEffect(() => {
    api.getContextKeySuggestions(projectId).then((d) => setContextKeySuggestions(d.suggestions || [])).catch(() => {})
  }, [projectId])

  const autoKey = (n: string) => n.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')

  // Variant names targeting-rule dropdowns and default-variant selectors can pick from.
  const variantNames = flagType === 'boolean'
    ? ['on', 'off']
    : variants.map((v) => v.name).filter((n) => n.trim() !== '')

  // Keep defaultVariant in sync when the user removes/renames the row it pointed at.
  useEffect(() => {
    if (flagType !== 'string') return
    if (!variantNames.includes(defaultVariant) && variantNames.length > 0) {
      setDefaultVariant(variantNames[0])
    }
  }, [flagType, variantNames, defaultVariant])

  const splitSum = variants.reduce((sum, v) => sum + (v.splitPct || 0), 0)

  const validateString = (): string | null => {
    if (variants.length < 2) return 'Need at least two variants'
    const trimmedNames = variants.map((v) => v.name.trim())
    if (trimmedNames.some((n) => !n)) return 'All variants need a name'
    if (new Set(trimmedNames).size !== trimmedNames.length) return 'Variant names must be unique'
    if (splitSum !== 100) return `Splits must add up to 100% (currently ${splitSum}%)`
    return null
  }

  const handleSave = async () => {
    if (!flagKey.trim() || !name.trim()) { setError('Key and name are required'); return }

    let variantsObj: Record<string, unknown>
    let splitObj: Record<string, number>
    let resolvedDefault: string

    if (flagType === 'boolean') {
      variantsObj = { on: true, off: false }
      resolvedDefault = defaultBool
      // No rollout → 100% to default. With rollout → that % goes to the
      // *opposite* of the default ("flip X% of users"), the rest stays default.
      if (!rolloutEnabled || rolloutPct === 0) {
        splitObj = { [defaultBool]: 100, [defaultBool === 'on' ? 'off' : 'on']: 0 }
      } else {
        const opposite = defaultBool === 'on' ? 'off' : 'on'
        splitObj = { [defaultBool]: 100 - rolloutPct, [opposite]: rolloutPct }
      }
    } else {
      const err = validateString()
      if (err) { setError(err); return }
      variantsObj = Object.fromEntries(variants.map((v) => [v.name.trim(), v.returnValue]))
      splitObj = Object.fromEntries(variants.map((v) => [v.name.trim(), v.splitPct]))
      resolvedDefault = defaultVariant
    }

    setSaving(true)
    setError(null)
    try {
      const body = {
        flag_key: flagKey,
        name,
        flag_type: flagType,
        variants: JSON.stringify(variantsObj),
        default_variant: resolvedDefault,
        split: JSON.stringify(splitObj),
        conversion_event: conversionEvent,
        targeting_rules: targetingRules.length > 0 ? JSON.stringify(targetingRules) : '[]',
      }
      const f = isEdit && flag
        ? await api.updateFlag(projectId, flag.id, body)
        : await api.createFlag(projectId, body)
      if (!isEdit) trackEvent('flag_created', { flag_key: f.flag_key, flag_type: f.flag_type })
      onSaved(f)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  const inputStyle = {
    background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8,
    padding: '0.6rem 0.875rem', color: C.text, fontSize: 14,
    width: '100%', boxSizing: 'border-box' as const, outline: 'none',
  }

  const defaultNewVariant = flagType === 'boolean'
    ? (defaultBool === 'on' ? 'off' : 'on')
    : (variantNames[0] || '')

  return (
    <>
      <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', zIndex: 1000 }} />
      <div style={{
        position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%, -50%)',
        background: C.surface, border: `1px solid ${C.border}`, borderRadius: 16,
        width: '90%', maxWidth: 520, zIndex: 1001, maxHeight: '90vh', overflowY: 'auto',
      }}>
        <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}`, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>{isEdit ? 'Edit Flag' : 'Create Flag'}</h2>
          <button onClick={onClose} style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer' }}><X size={18} /></button>
        </div>

        <div style={{ padding: '1.5rem' }}>
          {error && (
            <div style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: 8, padding: '0.6rem', color: C.error, fontSize: 13, marginBottom: '1rem' }}>
              {error}
            </div>
          )}

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Name</label>
          <input value={name} onChange={(e) => {
            setName(e.target.value)
            // Auto-sync the key to the name only when creating — once a flag is
            // created its key is the stable identifier consumers reference, so
            // changing it via this auto-derivation would silently break them.
            if (!isEdit && (!flagKey || flagKey === autoKey(name))) setFlagKey(autoKey(e.target.value))
          }}
            placeholder="e.g. Checkout Redesign" style={{ ...inputStyle, marginBottom: 16 }} />

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>
            Key {isEdit && <span style={{ color: C.muted, fontWeight: 400 }}>(immutable)</span>}
          </label>
          <input value={flagKey}
            onChange={(e) => setFlagKey(e.target.value)}
            disabled={isEdit}
            placeholder="e.g. checkout-redesign"
            style={{ ...inputStyle, marginBottom: 16, fontFamily: 'monospace', opacity: isEdit ? 0.6 : 1, cursor: isEdit ? 'not-allowed' : 'text' }} />

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>
            Type {isEdit && <span style={{ color: C.muted, fontWeight: 400 }}>(immutable)</span>}
          </label>
          <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
            {(['boolean', 'string'] as const).map((t) => (
              <button key={t} type="button" onClick={() => { if (!isEdit) setFlagType(t) }} disabled={isEdit} style={{
                flex: 1, padding: '0.5rem', border: `1px solid ${flagType === t ? C.amber : C.border}`,
                borderRadius: 8, background: flagType === t ? 'rgba(245,158,11,0.1)' : 'transparent',
                color: flagType === t ? C.amber : C.muted, fontSize: 13, fontWeight: 600,
                cursor: isEdit ? 'not-allowed' : 'pointer',
                opacity: isEdit && flagType !== t ? 0.5 : 1,
              }}>
                {t}
              </button>
            ))}
          </div>

          {flagType === 'boolean' && (
            <>
              <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Default value</label>
              <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
                {(['on', 'off'] as const).map((v) => (
                  <button key={v} onClick={() => setDefaultBool(v)} type="button" style={{
                    flex: 1, padding: '0.5rem', border: `1px solid ${defaultBool === v ? C.amber : C.border}`,
                    borderRadius: 8, background: defaultBool === v ? 'rgba(245,158,11,0.1)' : 'transparent',
                    color: defaultBool === v ? C.amber : C.muted, fontSize: 13, fontWeight: 600, cursor: 'pointer',
                  }}>
                    {v}
                  </button>
                ))}
              </div>
              <div style={{ marginBottom: 16 }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: C.muted, fontWeight: 600, cursor: 'pointer' }}>
                  <input type="checkbox" checked={rolloutEnabled} onChange={(e) => setRolloutEnabled(e.target.checked)} />
                  Gradual rollout
                </label>
                {rolloutEnabled && (
                  <div style={{ marginTop: 8, padding: '0.75rem', background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8 }}>
                    <div style={{ fontSize: 12, color: C.muted, marginBottom: 6 }}>
                      Flip <span style={{ color: C.amber, fontWeight: 700 }}>{rolloutPct}%</span> of users to{' '}
                      <code style={{ color: C.text }}>{defaultBool === 'on' ? 'off' : 'on'}</code>; the rest stay on the default.
                    </div>
                    <input type="range" min={0} max={100} value={rolloutPct}
                      onChange={(e) => setRolloutPct(Number(e.target.value))}
                      style={{ width: '100%', accentColor: C.amber }} />
                  </div>
                )}
              </div>
            </>
          )}

          {flagType === 'string' && (
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Variants</label>
              {variants.map((v, i) => (
                <div key={i} style={{ display: 'flex', gap: 6, marginBottom: 6, alignItems: 'center' }}>
                  <input
                    type="radio"
                    name="defaultVariant"
                    checked={defaultVariant === v.name}
                    onChange={() => setDefaultVariant(v.name)}
                    title="Default variant"
                    style={{ accentColor: C.amber }}
                  />
                  <input
                    value={v.name}
                    onChange={(e) => {
                      const next = [...variants]
                      const oldName = next[i].name
                      next[i] = { ...v, name: e.target.value, returnValue: advancedReturnValues ? v.returnValue : e.target.value }
                      setVariants(next)
                      if (defaultVariant === oldName) setDefaultVariant(e.target.value)
                    }}
                    placeholder="variant name"
                    style={{ ...inputStyle, flex: 2, fontFamily: 'monospace' }}
                  />
                  {advancedReturnValues && (
                    <input
                      value={v.returnValue}
                      onChange={(e) => {
                        const next = [...variants]; next[i] = { ...v, returnValue: e.target.value }; setVariants(next)
                      }}
                      placeholder="return value"
                      style={{ ...inputStyle, flex: 2 }}
                    />
                  )}
                  <input
                    type="number" min={0} max={100}
                    value={v.splitPct}
                    onChange={(e) => {
                      const next = [...variants]; next[i] = { ...v, splitPct: Number(e.target.value) }; setVariants(next)
                    }}
                    style={{ ...inputStyle, width: 70, textAlign: 'right' }}
                  />
                  <span style={{ color: C.muted, fontSize: 12, width: 14 }}>%</span>
                  <button type="button" onClick={() => {
                    if (variants.length <= 2) return
                    const next = variants.filter((_, idx) => idx !== i); setVariants(next)
                  }} disabled={variants.length <= 2}
                    style={{ background: 'transparent', border: 'none', color: variants.length <= 2 ? '#4a4d5a' : C.muted, cursor: variants.length <= 2 ? 'default' : 'pointer', padding: 2 }}>
                    <X size={14} />
                  </button>
                </div>
              ))}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 6 }}>
                <button type="button"
                  onClick={() => setVariants([...variants, { name: '', returnValue: '', splitPct: 0 }])}
                  style={{ background: 'transparent', border: `1px dashed ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '4px 10px', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                  <Plus size={12} /> Add variant
                </button>
                <span style={{ fontSize: 12, color: splitSum === 100 ? C.success : C.error }}>
                  Splits sum: {splitSum}%
                </span>
              </div>
              <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: C.muted, marginTop: 10, cursor: 'pointer' }}>
                <input type="checkbox" checked={advancedReturnValues} onChange={(e) => {
                  setAdvancedReturnValues(e.target.checked)
                  if (!e.target.checked) {
                    setVariants((rows) => rows.map((r) => ({ ...r, returnValue: r.name })))
                  }
                }} />
                Use a different return value than the variant name
              </label>
            </div>
          )}

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Conversion event</label>
          <select value={conversionEvent} onChange={(e) => setConversionEvent(e.target.value)}
            style={{ ...inputStyle, marginBottom: 16 }}>
            <option value="">Select an event (optional)...</option>
            {eventNames.map((n) => <option key={n} value={n}>{n}</option>)}
          </select>

          <div style={{ border: `1px solid ${C.border}`, borderRadius: 10, overflow: 'hidden' }}>
            <button onClick={() => setShowRules(!showRules)} type="button" style={{
              width: '100%', display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '0.75rem 1rem', background: 'transparent', border: 'none',
              color: C.muted, fontSize: 13, fontWeight: 600, cursor: 'pointer',
            }}>
              <span>Targeting Rules ({targetingRules.length})</span>
              {showRules ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            </button>

            {showRules && (
              <div style={{ padding: '0 1rem 1rem', borderTop: `1px solid ${C.border}` }}>
                <TargetingRuleEditor
                  rules={targetingRules}
                  onChange={setTargetingRules}
                  variantNames={variantNames}
                  defaultNewVariant={defaultNewVariant}
                  contextKeySuggestions={contextKeySuggestions}
                  activeKeyInput={activeKeyInput}
                  onActiveKeyInputChange={setActiveKeyInput}
                  inputStyle={inputStyle}
                />
              </div>
            )}
          </div>
        </div>

        <div style={{ padding: '1rem 1.5rem', borderTop: `1px solid ${C.border}`, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button onClick={onClose} style={{ padding: '0.5rem 1rem', background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 8, color: C.text, fontSize: 13, fontWeight: 600, cursor: 'pointer' }}>
            Cancel
          </button>
          <button onClick={handleSave} disabled={saving || !flagKey || !name}
            style={{
              padding: '0.5rem 1rem', border: 'none', borderRadius: 8, fontSize: 13, fontWeight: 700, cursor: saving ? 'default' : 'pointer',
              background: saving || !flagKey || !name ? '#4a4d5a' : C.amber,
              color: saving || !flagKey || !name ? C.muted : '#000',
            }}>
            {saving ? 'Saving...' : (isEdit ? 'Save changes' : 'Create Flag')}
          </button>
        </div>
      </div>
    </>
  )
}
