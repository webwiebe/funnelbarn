import type { DistributionEntry } from '../../lib/api'
import { C } from '../../lib/theme'

const BREAKDOWN_FIELDS = [
  { key: 'device_type', label: 'Device type' },
  { key: 'browser', label: 'Browser' },
  { key: 'country_code', label: 'Country' },
  { key: 'os', label: 'OS' },
  { key: 'connection_class', label: 'Connection' },
  { key: 'dark_mode', label: 'Dark mode' },
] as const

export function VisitorBreakdown({ distributions }: {
  distributions: Record<string, DistributionEntry[]>
}) {
  const visibleFields = BREAKDOWN_FIELDS.filter(({ key }) => distributions[key]?.length)
  if (visibleFields.length === 0) return null

  return (
    <div style={{ marginTop: '2rem' }}>
      <h2 style={{ fontSize: 17, fontWeight: 700, letterSpacing: '-0.3px', marginBottom: '1rem' }}>
        Visitor breakdown
      </h2>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: '1rem' }}>
        {visibleFields.map(({ key, label }) => {
          const entries = distributions[key]
          const maxPct = entries[0]?.pct ?? 1
          return (
            <div key={key} style={{
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 12,
              padding: '1rem 1.25rem',
            }}>
              <div style={{ fontSize: 12, fontWeight: 700, color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 10 }}>
                {label}
              </div>
              {entries.slice(0, 6).map((e) => (
                <div key={e.value} style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 7 }}>
                  <div style={{ minWidth: 80, fontSize: 12, color: C.text, textAlign: 'right', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {e.value}
                  </div>
                  <div style={{ flex: 1, height: 12, background: '#2a2d3a', borderRadius: 3, overflow: 'hidden' }}>
                    <div style={{
                      height: '100%',
                      width: `${(e.pct / maxPct) * 100}%`,
                      background: C.amber,
                      opacity: 0.65,
                      borderRadius: 3,
                      transition: 'width 0.5s',
                    }} />
                  </div>
                  <div style={{ fontSize: 11, color: C.muted, minWidth: 28, textAlign: 'right' }}>{e.pct}%</div>
                </div>
              ))}
            </div>
          )
        })}
      </div>
    </div>
  )
}
