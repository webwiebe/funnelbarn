const C = {
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
}

// RetentionSettings explains how long raw event data is kept. The window is a
// server setting, so the card is read-only.
export function RetentionSettings() {
  return (
    <div style={{
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
      overflow: 'hidden',
      marginBottom: '2rem',
    }}>
      <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
        <div style={{ fontWeight: 700, fontSize: 15 }}>Data Retention</div>
        <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
          How long raw event data is kept before being purged.
        </div>
      </div>
      <div style={{ padding: '1.25rem 1.5rem', display: 'flex', alignItems: 'center', gap: 12 }}>
        <div style={{
          background: 'rgba(245,158,11,0.08)',
          border: `1px solid rgba(245,158,11,0.2)`,
          borderRadius: 8,
          padding: '0.75rem 1rem',
          flex: 1,
        }}>
          <div style={{ fontSize: 14, color: C.text, fontWeight: 600, marginBottom: 4 }}>
            Event retention: 90 days
          </div>
          <div style={{ fontSize: 12, color: C.muted }}>
            Configured via{' '}
            <code style={{
              fontFamily: '"SF Mono", "Fira Code", monospace',
              color: C.amber,
              background: 'rgba(245,158,11,0.08)',
              padding: '0.1rem 0.3rem',
              borderRadius: 3,
            }}>
              FUNNELBARN_EVENT_RETENTION_DAYS
            </code>
            {' '}on the server. Events older than this window are automatically deleted.
          </div>
        </div>
      </div>
    </div>
  )
}
