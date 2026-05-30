import { X } from 'lucide-react'
import type { TargetingRule, TargetingOperator } from '../../lib/api'

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

interface TargetingRuleEditorProps {
  rules: TargetingRule[]
  onChange: (rules: TargetingRule[]) => void
  variantNames: string[]
  defaultNewVariant: string
  contextKeySuggestions: { context_key: string; pct: number }[]
  activeKeyInput: string | null
  onActiveKeyInputChange: (key: string | null) => void
  inputStyle: React.CSSProperties
}

export function TargetingRuleEditor({
  rules,
  onChange,
  variantNames,
  defaultNewVariant,
  contextKeySuggestions,
  activeKeyInput,
  onActiveKeyInputChange,
  inputStyle,
}: TargetingRuleEditorProps) {
  return (
    <>
      {rules.map((rule, ri) => (
        <div key={ri} style={{ background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8, padding: '0.75rem', marginTop: '0.75rem' }}>
          <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
            <input value={rule.name} placeholder="Rule name" onChange={(e) => {
              const updated = [...rules]; updated[ri] = { ...rule, name: e.target.value }; onChange(updated)
            }} style={{ ...inputStyle, flex: 2 }} />
            <select value={rule.variant} onChange={(e) => {
              const updated = [...rules]; updated[ri] = { ...rule, variant: e.target.value }; onChange(updated)
            }} style={{ ...inputStyle, flex: 1 }}>
              {variantNames.map((v) => <option key={v} value={v}>{v}</option>)}
            </select>
            <button onClick={() => onChange(rules.filter((_, i) => i !== ri))} type="button"
              style={{ background: 'transparent', border: 'none', color: C.error, cursor: 'pointer', padding: 4 }}>
              <X size={14} />
            </button>
          </div>

          <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
            {(['all', 'any'] as const).map((m) => (
              <button key={m} type="button" onClick={() => {
                const updated = [...rules]; updated[ri] = { ...rule, match: m }; onChange(updated)
              }} style={{
                padding: '0.25rem 0.6rem', fontSize: 12, fontWeight: 600, cursor: 'pointer',
                border: `1px solid ${rule.match === m ? C.amber : C.border}`, borderRadius: 6,
                background: rule.match === m ? 'rgba(245,158,11,0.1)' : 'transparent',
                color: rule.match === m ? C.amber : C.muted,
              }}>
                {m === 'all' ? 'ALL (AND)' : 'ANY (OR)'}
              </button>
            ))}
          </div>

          {rule.conditions.map((cond, ci) => (
            <div key={ci} style={{ display: 'flex', gap: 4, marginBottom: 4 }}>
              <div style={{ position: 'relative', flex: 2 }}>
                <input
                  value={cond.context_key}
                  placeholder="context key"
                  onChange={(e) => {
                    const updated = [...rules]
                    const conds = [...rule.conditions]
                    conds[ci] = { ...cond, context_key: e.target.value }
                    updated[ri] = { ...rule, conditions: conds }
                    onChange(updated)
                  }}
                  onFocus={() => onActiveKeyInputChange(`${ri}-${ci}`)}
                  onBlur={() => setTimeout(() => onActiveKeyInputChange(null), 150)}
                  style={{ ...inputStyle, width: '100%', boxSizing: 'border-box', fontSize: 12, padding: '0.4rem 0.6rem', fontFamily: 'monospace' }}
                />
                {activeKeyInput === `${ri}-${ci}` && contextKeySuggestions.filter(s =>
                  !cond.context_key || s.context_key.toLowerCase().includes(cond.context_key.toLowerCase())
                ).length > 0 && (
                  <div style={{
                    position: 'absolute', top: '100%', left: 0, right: 0, zIndex: 200,
                    background: '#1a1d27', border: '1px solid #2a2d3a', borderRadius: 6,
                    marginTop: 2, boxShadow: '0 8px 24px rgba(0,0,0,0.5)', overflow: 'hidden',
                  }}>
                    {contextKeySuggestions
                      .filter(s => !cond.context_key || s.context_key.toLowerCase().includes(cond.context_key.toLowerCase()))
                      .slice(0, 8)
                      .map(s => (
                        <div
                          key={s.context_key}
                          onMouseDown={() => {
                            const updated = [...rules]
                            const conds = [...rule.conditions]
                            conds[ci] = { ...cond, context_key: s.context_key }
                            updated[ri] = { ...rule, conditions: conds }
                            onChange(updated)
                            onActiveKeyInputChange(null)
                          }}
                          style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                            padding: '0.35rem 0.6rem', cursor: 'pointer', fontFamily: 'monospace', fontSize: 12 }}
                          onMouseEnter={e => { (e.currentTarget as HTMLElement).style.background = 'rgba(245,158,11,0.08)' }}
                          onMouseLeave={e => { (e.currentTarget as HTMLElement).style.background = 'transparent' }}
                        >
                          <span style={{ color: '#e2e8f0' }}>{s.context_key}</span>
                          <span style={{ color: '#94a3b8', fontSize: 10 }}>{s.pct}%</span>
                        </div>
                      ))}
                  </div>
                )}
              </div>
              <select value={cond.operator} onChange={(e) => {
                const updated = [...rules]
                const conds = [...rule.conditions]; conds[ci] = { ...cond, operator: e.target.value as TargetingOperator }
                updated[ri] = { ...rule, conditions: conds }; onChange(updated)
              }} style={{ ...inputStyle, flex: 1.5, fontSize: 12, padding: '0.4rem 0.4rem' }}>
                {(['eq', 'neq', 'contains', 'not_contains', 'starts_with', 'ends_with', 'in', 'not_in', 'present', 'not_present'] as const).map((op) => (
                  <option key={op} value={op}>{op}</option>
                ))}
              </select>
              {cond.operator !== 'present' && cond.operator !== 'not_present' && (
                <input value={cond.value} placeholder="value" onChange={(e) => {
                  const updated = [...rules]
                  const conds = [...rule.conditions]; conds[ci] = { ...cond, value: e.target.value }
                  updated[ri] = { ...rule, conditions: conds }; onChange(updated)
                }} style={{ ...inputStyle, flex: 2, fontSize: 12, padding: '0.4rem 0.6rem' }} />
              )}
              <button onClick={() => {
                const updated = [...rules]
                updated[ri] = { ...rule, conditions: rule.conditions.filter((_, i) => i !== ci) }
                onChange(updated)
              }} type="button" style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer', padding: 2 }}>
                <X size={12} />
              </button>
            </div>
          ))}

          <button onClick={() => {
            const updated = [...rules]
            updated[ri] = { ...rule, conditions: [...rule.conditions, { context_key: '', operator: 'eq', value: '' }] }
            onChange(updated)
          }} type="button" style={{
            background: 'transparent', border: 'none', color: C.amber,
            fontSize: 12, cursor: 'pointer', padding: '4px 0', fontWeight: 600,
          }}>
            + Add condition
          </button>
        </div>
      ))}

      <button onClick={() => onChange([...rules, {
        name: '', variant: defaultNewVariant,
        match: 'all', conditions: [{ context_key: '', operator: 'eq', value: '' }],
      }])} type="button" style={{
        marginTop: '0.75rem', width: '100%', padding: '0.5rem',
        background: 'transparent', border: `1px dashed ${C.border}`, borderRadius: 8,
        color: C.amber, fontSize: 13, fontWeight: 600, cursor: 'pointer',
      }}>
        + Add rule
      </button>
    </>
  )
}
