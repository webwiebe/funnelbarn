// Fields for a config flag. The type comes first: a boolean is one On/Off
// switch, anything else is a value plus the named values a targeting rule can
// serve.

import { Plus, X } from 'lucide-react'
import { C } from './shared'
import type { ConfigType, ConfigVariantRow } from './shape'

const labelStyle = { display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }

const TYPE_LABELS: Array<[ConfigType, string]> = [
  ['boolean', 'Boolean'], ['text', 'Text'], ['number', 'Number'], ['json', 'JSON'],
]

const TYPE_HINTS: Record<ConfigType, string> = {
  boolean: 'On or off. A targeting rule picks On or Off for the visitors it matches.',
  text: 'A piece of text, kept exactly as typed.',
  number: 'A number, such as 250 or 0.5.',
  json: 'A JSON value: an object, a list, or any other JSON.',
}

// ConfigTypeField picks the value type of a new config flag. A stored flag keeps
// its type, so it is shown without the choice.
export function ConfigTypeField({ value, locked, onChange }: {
  value: ConfigType | null
  locked: boolean
  onChange: (t: ConfigType) => void
}) {
  return (
    <div style={{ marginBottom: 16 }}>
      <span style={labelStyle}>Value type {locked && <span style={{ fontWeight: 400 }}>(immutable)</span>}</span>
      <div role="group" aria-label="Value type" style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {TYPE_LABELS.map(([t, label]) => {
          const active = value === t
          return (
            <button key={t} type="button" disabled={locked} onClick={() => onChange(t)} aria-pressed={active} style={{
              flex: '1 1 70px', padding: '0.5rem', border: `1px solid ${active ? C.amber : C.border}`,
              borderRadius: 8, background: active ? 'rgba(245,158,11,0.1)' : 'transparent',
              color: active ? C.amber : C.muted, fontSize: 13, fontWeight: 600,
              cursor: locked ? 'not-allowed' : 'pointer', opacity: locked && !active ? 0.5 : 1,
            }}>
              {label}
            </button>
          )
        })}
      </div>
      {value && <div style={{ fontSize: 12, color: C.muted, marginTop: 6 }}>{TYPE_HINTS[value]}</div>}
    </div>
  )
}

// OnOffField is the whole value of a boolean flag: what everyone gets without a
// matching rule.
export function OnOffField({ value, onChange }: { value: boolean; onChange: (on: boolean) => void }) {
  return (
    <div style={{ marginBottom: 16 }}>
      <span style={labelStyle}>Value without a matching rule</span>
      <div role="group" aria-label="Value without a matching rule" style={{ display: 'flex', gap: 8 }}>
        {([[true, 'On'], [false, 'Off']] as const).map(([v, label]) => (
          <button key={label} type="button" aria-pressed={value === v} onClick={() => onChange(v)} style={{
            flex: 1, padding: '0.5rem', border: `1px solid ${value === v ? C.amber : C.border}`,
            borderRadius: 8, background: value === v ? 'rgba(245,158,11,0.1)' : 'transparent',
            color: value === v ? C.amber : C.muted, fontSize: 13, fontWeight: 600, cursor: 'pointer',
          }}>
            {label}
          </button>
        ))}
      </div>
      <div style={{ fontSize: 12, color: C.muted, marginTop: 6 }}>
        Targeting rules below choose On or Off for the visitors they match.
      </div>
    </div>
  )
}

// ConfigValuesField lists every value of a text, number or JSON flag. Row 1 is
// the value everyone gets without a matching rule, its name fixed; the rows
// below are named values a targeting rule can serve. Every name that can
// collide is on screen.
export function ConfigValuesField({ type, defaultName, value, onValue, rows, onRows, inputStyle }: {
  type: ConfigType
  defaultName: string
  value: string
  onValue: (v: string) => void
  rows: ConfigVariantRow[]
  onRows: (rows: ConfigVariantRow[]) => void
  inputStyle: React.CSSProperties
}) {
  const update = (i: number, patch: Partial<ConfigVariantRow>) =>
    onRows(rows.map((r, idx) => (idx === i ? { ...r, ...patch } : r)))
  const placeholder = type === 'number' ? 'e.g. 250' : type === 'json' ? '{"mode":"slow"}' : 'text'
  const cell = { ...inputStyle, minWidth: 0, fontFamily: 'monospace' }
  return (
    <div style={{ marginBottom: 16 }}>
      <label htmlFor="config-value" style={labelStyle}>Values</label>
      <div style={{ display: 'flex', gap: 6, marginBottom: 6, alignItems: 'center' }}>
        <input aria-label="Default name" value={defaultName} readOnly title="The default value's name cannot change"
          style={{ ...cell, flex: 1, opacity: 0.7, cursor: 'not-allowed' }} />
        <input id="config-value" aria-label="Value" value={value} placeholder={placeholder}
          onChange={(e) => onValue(e.target.value)} style={{ ...cell, flex: 2 }} />
        <span style={{ width: 18, flexShrink: 0 }} />
      </div>
      <div style={{ fontSize: 12, color: C.muted, marginBottom: 10 }}>
        Row 1 is the default: what everyone gets without a matching rule.
      </div>
      {rows.map((row, i) => (
        <div key={i} style={{ display: 'flex', gap: 6, marginBottom: 6, alignItems: 'center' }}>
          <input aria-label={`Variant ${i + 1} name`} value={row.name} placeholder="name"
            onChange={(e) => update(i, { name: e.target.value })} style={{ ...cell, flex: 1 }} />
          <input aria-label={`Variant ${i + 1} value`} value={row.value} placeholder="value"
            onChange={(e) => update(i, { value: e.target.value })} style={{ ...cell, flex: 2 }} />
          <button type="button" aria-label={`Remove variant ${i + 1}`}
            onClick={() => onRows(rows.filter((_, idx) => idx !== i))}
            style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer', padding: 2, flexShrink: 0 }}>
            <X size={14} />
          </button>
        </div>
      ))}
      <button type="button" onClick={() => onRows([...rows, { name: '', value: '' }])}
        style={{ background: 'transparent', border: `1px dashed ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '4px 10px', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
        <Plus size={12} /> Add rule value
      </button>
      <div style={{ fontSize: 12, color: C.muted, marginTop: 6 }}>
        A targeting rule below can serve a named value. Without a matching rule everyone gets row 1.
      </div>
    </div>
  )
}
