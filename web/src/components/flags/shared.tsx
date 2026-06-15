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

export { C }

export function statusBadge(status: string) {
  const styles: Record<string, { bg: string; color: string }> = {
    active: { bg: 'rgba(16,185,129,0.1)', color: '#10b981' },
    paused: { bg: 'rgba(245,158,11,0.1)', color: '#f59e0b' },
    inactive: { bg: 'rgba(148,163,184,0.12)', color: '#94a3b8' },
  }
  const s = styles[status] ?? styles.active
  return (
    <span style={{
      fontSize: 11, fontWeight: 700,
      background: s.bg, color: s.color,
      border: `1px solid ${s.color}33`,
      borderRadius: 99, padding: '0.15rem 0.6rem',
      textTransform: 'uppercase', letterSpacing: '0.05em',
    }}>
      {status}
    </span>
  )
}

// originBadge flags an auto-created (never-configured) flag so users know it was
// seeded from an SDK evaluation and is waiting to be set up.
export function originBadge(origin: string) {
  if (origin !== 'auto') return null
  return (
    <span title="Created automatically from an SDK evaluation — configure it to start an experiment." style={{
      fontSize: 11, fontWeight: 700,
      background: 'rgba(99,102,241,0.12)', color: '#818cf8',
      border: '1px solid rgba(99,102,241,0.3)',
      borderRadius: 99, padding: '0.15rem 0.6rem',
      textTransform: 'uppercase', letterSpacing: '0.05em',
    }}>
      auto-detected
    </span>
  )
}

export function StatCard({ label, sample, conversions, rate, winner }: {
  label: string; sample: number; conversions: number; rate: number; winner: boolean
}) {
  return (
    <div style={{
      flex: 1, background: C.bg,
      border: `1px solid ${winner ? 'rgba(16,185,129,0.4)' : C.border}`,
      borderRadius: 12, padding: '1.25rem', position: 'relative',
    }}>
      {winner && (
        <div style={{
          position: 'absolute', top: 10, right: 10,
          fontSize: 11, fontWeight: 700,
          background: 'rgba(16,185,129,0.15)', color: C.success,
          border: '1px solid rgba(16,185,129,0.3)',
          borderRadius: 99, padding: '0.15rem 0.6rem',
        }}>
          Winner
        </div>
      )}
      <div style={{ fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: '1rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        {label}
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.75rem' }}>
        <div>
          <div style={{ fontSize: 22, fontWeight: 800, color: C.text }}>{sample.toLocaleString()}</div>
          <div style={{ fontSize: 12, color: C.muted }}>evaluations</div>
        </div>
        <div>
          <div style={{ fontSize: 22, fontWeight: 800, color: C.text }}>{conversions.toLocaleString()}</div>
          <div style={{ fontSize: 12, color: C.muted }}>conversions</div>
        </div>
        <div style={{ gridColumn: '1 / -1' }}>
          <div style={{ fontSize: 28, fontWeight: 800, color: winner ? C.success : C.amber }}>
            {(rate * 100).toFixed(2)}%
          </div>
          <div style={{ fontSize: 12, color: C.muted }}>conversion rate</div>
        </div>
      </div>
    </div>
  )
}
