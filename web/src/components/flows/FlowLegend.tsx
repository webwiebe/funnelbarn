import { C } from './colors'

const ENTRIES = [
  { color: C.amber, label: 'Focused page' },
  { color: C.indigo, label: 'Inbound pages' },
  { color: C.green, label: 'Outbound pages' },
  { color: C.blue, label: 'Entry referrers' },
  { color: C.red, label: 'Drop-off' },
]

export function FlowLegend() {
  return (
    <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
      {ENTRIES.map(({ color, label }) => (
        <div key={label} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: C.muted }}>
          <div style={{ width: 10, height: 10, borderRadius: 2, background: color, flexShrink: 0 }} />
          {label}
        </div>
      ))}
    </div>
  )
}
