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

export function ScopeToggle({
  value,
  onChange,
}: {
  value: 'session' | 'page_view'
  onChange: (v: 'session' | 'page_view') => void
}) {
  return (
    <div style={{ marginBottom: '1.25rem' }}>
      <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>Scope</label>
      <div style={{
        display: 'inline-flex',
        background: C.bg,
        border: `1px solid ${C.border}`,
        borderRadius: 8,
        overflow: 'hidden',
      }}>
        {(['session', 'page_view'] as const).map((opt) => {
          const active = value === opt
          return (
            <button
              key={opt}
              onClick={() => onChange(opt)}
              style={{
                padding: '0.45rem 1rem',
                border: 'none',
                background: active ? 'rgba(245,158,11,0.18)' : 'transparent',
                color: active ? C.amber : C.muted,
                fontWeight: active ? 700 : 400,
                fontSize: 13,
                cursor: 'pointer',
                borderRight: opt === 'session' ? `1px solid ${C.border}` : 'none',
                transition: 'all 0.15s',
              }}
            >
              {opt === 'session' ? 'Session' : 'Page view'}
            </button>
          )
        })}
      </div>
    </div>
  )
}
