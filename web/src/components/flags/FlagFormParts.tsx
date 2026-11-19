import { Plus, X, ChevronDown, ChevronRight } from 'lucide-react'
import type { CSSProperties } from 'react'
import { TargetingRuleEditor } from './TargetingRuleEditor'
import { ConfigTypeField, ConfigValuesField, OnOffField } from './ConfigFields'
import { ENVIRONMENT_SCOPE_NOTE } from './environmentScope'
import { configTypeOf } from './shape'
import type { StringVariantRow } from './shape'
import type { FlagForm } from './useFlagForm'
import { autoKey } from './useFlagForm'
import { C } from './flagFormTheme'

const labelStyle = { display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 } as const

// FlagKindSelector picks what the flag is. The kind decides the evaluation
// semantics, not just the label, so the copy spells out the difference.
export function FlagKindSelector({ value, onChange }: {
  value: 'experiment' | 'config'
  onChange: (k: 'experiment' | 'config') => void
}) {
  return (
    <>
      <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Kind</label>
      <div style={{ display: 'flex', gap: 8, marginBottom: 6 }}>
        {([['experiment', 'Experiment'], ['config', 'Config value']] as const).map(([k, label]) => {
          const active = value === k
          return (
            <button key={k} type="button" onClick={() => onChange(k)} style={{
              flex: 1, padding: '0.5rem', border: `1px solid ${active ? C.amber : C.border}`,
              borderRadius: 8, background: active ? 'rgba(245,158,11,0.1)' : 'transparent',
              color: active ? C.amber : C.muted, fontSize: 13, fontWeight: 600, cursor: 'pointer',
            }}>
              {label}
            </button>
          )
        })}
      </div>
      <div style={{ fontSize: 12, color: C.muted, marginBottom: 16, lineHeight: 1.5 }}>
        {value === 'config'
          ? 'One value for everyone, read by a server, unless a targeting rule serves one of its named values. No bucketing, no evaluation rows, and no variant report \u2014 change the value here and the fleet picks it up without a deploy.'
          : 'Bucketed per user by targeting key. Every evaluation is recorded and reported on by variant and conversion.'}
      </div>
    </>
  )
}

// BooleanVariantFields edits a release flag: an on/off default plus an optional
// gradual rollout that flips a percentage of users to the other side.
function BooleanVariantFields({
  visible, defaultBool, setDefaultBool, rolloutEnabled, setRolloutEnabled, rolloutPct, setRolloutPct,
}: {
  visible: boolean
  defaultBool: 'on' | 'off'
  setDefaultBool: (v: 'on' | 'off') => void
  rolloutEnabled: boolean
  setRolloutEnabled: (v: boolean) => void
  rolloutPct: number
  setRolloutPct: (v: number) => void
}) {
  if (!visible) return null
  return (
    <>
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
    </>
  )
}

// StringVariantFields edits a multi-variant experiment: named variants, their
// return values, and the split that must add up to 100%.
function StringVariantFields({
  visible, variants, setVariants, defaultVariant, setDefaultVariant,
  advancedReturnValues, setAdvancedReturnValues, splitSum, inputStyle,
}: {
  visible: boolean
  variants: StringVariantRow[]
  setVariants: React.Dispatch<React.SetStateAction<StringVariantRow[]>>
  defaultVariant: string
  setDefaultVariant: (v: string) => void
  advancedReturnValues: boolean
  setAdvancedReturnValues: (v: boolean) => void
  splitSum: number
  inputStyle: React.CSSProperties
}) {
  if (!visible) return null
  return (
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
  )
}

const inputStyle: CSSProperties = {
  background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8,
  padding: '0.6rem 0.875rem', color: C.text, fontSize: 14,
  width: '100%', boxSizing: 'border-box', outline: 'none',
}

export function ModalHeader({ isEdit, onClose }: { isEdit: boolean; onClose: () => void }) {
  return (
    <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}`, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
      <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>{isEdit ? 'Edit Flag' : 'Create Flag'}</h2>
      <button onClick={onClose} style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer' }}><X size={18} /></button>
    </div>
  )
}

export function ErrorBanner({ message }: { message: string | null }) {
  if (!message) return null
  return (
    <div style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: 8, padding: '0.6rem', color: C.error, fontSize: 13, marginBottom: '1rem' }}>
      {message}
    </div>
  )
}

export function IdentityFields({ form }: { form: FlagForm }) {
  const { isEdit, name, setName, flagKey, setFlagKey } = form
  return (
    <>
      <label style={labelStyle}>Name</label>
      <input value={name} onChange={(e) => {
        setName(e.target.value)
        // Auto-sync the key to the name only when creating, because once a flag is
        // created its key is the stable identifier consumers reference, so
        // changing it via this auto-derivation would silently break them.
        if (!isEdit && (!flagKey || flagKey === autoKey(name))) setFlagKey(autoKey(e.target.value))
      }}
        placeholder="e.g. Checkout Redesign" style={{ ...inputStyle, marginBottom: 16 }} />

      <label style={labelStyle}>
        Key {isEdit && <span style={{ color: C.muted, fontWeight: 400 }}>(immutable)</span>}
      </label>
      <input value={flagKey}
        onChange={(e) => setFlagKey(e.target.value)}
        disabled={isEdit}
        placeholder="e.g. checkout-redesign"
        style={{ ...inputStyle, marginBottom: 16, fontFamily: 'monospace', opacity: isEdit ? 0.6 : 1, cursor: isEdit ? 'not-allowed' : 'text' }} />
    </>
  )
}

export function ConfigFields({ form, flag }: { form: FlagForm; flag?: Parameters<typeof configTypeOf>[0] }) {
  const { config } = form
  return (
    <>
      <ConfigTypeField value={config.configType} locked={configTypeOf(flag) !== null} onChange={config.setConfigType} />
      {config.configType === 'boolean' && <OnOffField value={config.configBool} onChange={config.setConfigBool} />}
      {config.configType !== null && config.configType !== 'boolean' && (
        <ConfigValuesField type={config.configType} defaultName={config.configDefault} value={config.configValue} onValue={config.setConfigValue}
          rows={config.configVariants} onRows={config.setConfigVariants} inputStyle={inputStyle} />
      )}
    </>
  )
}

function FlagTypeSelector({ form }: { form: FlagForm }) {
  const { isEdit, flagType, setFlagType } = form
  return (
    <>
      <label style={labelStyle}>
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
    </>
  )
}

export function ExperimentFields({ form }: { form: FlagForm }) {
  const { release, str, flagType } = form
  return (
    <>
      <FlagTypeSelector form={form} />
      <BooleanVariantFields
        visible={flagType === 'boolean'}
        defaultBool={release.defaultBool}
        setDefaultBool={release.setDefaultBool}
        rolloutEnabled={release.rolloutEnabled}
        setRolloutEnabled={release.setRolloutEnabled}
        rolloutPct={release.rolloutPct}
        setRolloutPct={release.setRolloutPct}
      />
      <StringVariantFields
        visible={flagType === 'string'}
        variants={str.variants}
        setVariants={str.setVariants}
        defaultVariant={str.defaultVariant}
        setDefaultVariant={str.setDefaultVariant}
        advancedReturnValues={str.advancedReturnValues}
        setAdvancedReturnValues={str.setAdvancedReturnValues}
        splitSum={str.splitSum}
        inputStyle={inputStyle}
      />
    </>
  )
}

export function ConversionField({ form }: { form: FlagForm }) {
  return (
    <>
      <label style={labelStyle}>Conversion event</label>
      <select value={form.conversionEvent} onChange={(e) => form.setConversionEvent(e.target.value)}
        style={{ ...inputStyle, marginBottom: 16 }}>
        <option value="">Select an event (optional)...</option>
        {form.rules.eventNames.map((n) => <option key={n} value={n}>{n}</option>)}
      </select>
    </>
  )
}

export function TargetingSection({ form }: { form: FlagForm }) {
  const { rules, variantNames, defaultNewVariant } = form
  const { showRules, setShowRules, targetingRules } = rules
  return (
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
            onChange={rules.setTargetingRules}
            variantNames={variantNames}
            defaultNewVariant={defaultNewVariant}
            contextKeySuggestions={rules.contextKeySuggestions}
            activeKeyInput={rules.activeKeyInput}
            onActiveKeyInputChange={rules.setActiveKeyInput}
            inputStyle={inputStyle}
          />
        </div>
      )}
    </div>
  )
}

export function ModalFooter({ form, onClose }: { form: FlagForm; onClose: () => void }) {
  const { saving, flagKey, name, isEdit } = form
  const disabled = saving || !flagKey || !name
  return (
    <>
      <div data-testid="flag-scope-note" style={{ padding: '0.75rem 1.5rem 0', fontSize: 12, color: C.muted, borderTop: `1px solid ${C.border}` }}>
        {ENVIRONMENT_SCOPE_NOTE}
      </div>
      <div style={{ padding: '0.75rem 1.5rem 1rem', display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
        <button onClick={onClose} style={{ padding: '0.5rem 1rem', background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 8, color: C.text, fontSize: 13, fontWeight: 600, cursor: 'pointer' }}>
          Cancel
        </button>
        <button onClick={form.save} disabled={disabled}
          style={{
            padding: '0.5rem 1rem', border: 'none', borderRadius: 8, fontSize: 13, fontWeight: 700, cursor: saving ? 'default' : 'pointer',
            background: disabled ? '#4a4d5a' : C.amber,
            color: disabled ? C.muted : '#000',
          }}>
          {saving ? 'Saving...' : (isEdit ? 'Save changes' : 'Create Flag')}
        </button>
      </div>
    </>
  )
}

