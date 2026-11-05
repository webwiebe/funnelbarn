// Fields for a config flag: the value everyone gets, and the named values a
// targeting rule can serve instead.

import { Plus, X } from 'lucide-react'
import { C } from './shared'
import type { ConfigVariantRow } from './shape'

const labelStyle = { display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }

// ConfigValueField edits the value a config flag serves when no rule matches.
export function ConfigValueField({ value, onChange, inputStyle }: {
  value: string
  onChange: (v: string) => void
  inputStyle: React.CSSProperties
}) {
  return (
    <>
      <label htmlFor="config-value" style={labelStyle}>Value</label>
      <input id="config-value" value={value} onChange={(e) => onChange(e.target.value)}
        placeholder={'e.g. 250, true, or {"mode":"slow"}'}
        style={{ ...inputStyle, marginBottom: 6, fontFamily: 'monospace' }} />
      <div style={{ fontSize: 12, color: C.muted, marginBottom: 16 }}>
        Read as JSON, so <code style={{ color: C.text }}>250</code> is a number and{' '}
        <code style={{ color: C.text }}>true</code> is a boolean. Anything that isn&apos;t valid
        JSON is kept as a string.
      </div>
    </>
  )
}

// ConfigVariantsField edits the named values beside the default. Only a
// targeting rule serves them; without a match everyone gets the value above.
export function ConfigVariantsField({ rows, onChange, inputStyle }: {
  rows: ConfigVariantRow[]
  onChange: (rows: ConfigVariantRow[]) => void
  inputStyle: React.CSSProperties
}) {
  const update = (i: number, patch: Partial<ConfigVariantRow>) =>
    onChange(rows.map((r, idx) => (idx === i ? { ...r, ...patch } : r)))
  return (
    <div style={{ marginBottom: 16 }}>
      <span style={labelStyle}>Rule values</span>
      {rows.map((row, i) => (
        <div key={i} style={{ display: 'flex', gap: 6, marginBottom: 6, alignItems: 'center' }}>
          <input aria-label={`Variant ${i + 1} name`} value={row.name} placeholder="variant name"
            onChange={(e) => update(i, { name: e.target.value })}
            style={{ ...inputStyle, flex: 1, fontFamily: 'monospace' }} />
          <input aria-label={`Variant ${i + 1} value`} value={row.value} placeholder="value"
            onChange={(e) => update(i, { value: e.target.value })}
            style={{ ...inputStyle, flex: 2, fontFamily: 'monospace' }} />
          <button type="button" aria-label={`Remove variant ${i + 1}`}
            onClick={() => onChange(rows.filter((_, idx) => idx !== i))}
            style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer', padding: 2 }}>
            <X size={14} />
          </button>
        </div>
      ))}
      <button type="button" onClick={() => onChange([...rows, { name: '', value: '' }])}
        style={{ background: 'transparent', border: `1px dashed ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '4px 10px', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
        <Plus size={12} /> Add rule value
      </button>
      <div style={{ fontSize: 12, color: C.muted, marginTop: 6 }}>
        A targeting rule below can serve one of these by name. Without a matching rule,
        everyone gets the value above.
      </div>
    </div>
  )
}
