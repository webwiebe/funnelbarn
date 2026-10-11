import type { FunnelScope } from '../../lib/api'
import { DEFAULT_WINDOW_DAYS } from './constants'

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

const OPTIONS: { value: FunnelScope; label: string }[] = [
  { value: 'session', label: 'Session' },
  { value: 'page_view', label: 'Page view' },
  { value: 'user', label: 'User' },
]

export function ScopeToggle({
  value,
  onChange,
  windowDays,
  onWindowDaysChange,
}: {
  value: FunnelScope
  onChange: (v: FunnelScope) => void
  windowDays?: number
  onWindowDaysChange?: (days: number) => void
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
        {OPTIONS.map((opt, i) => {
          const active = value === opt.value
          return (
            <button
              key={opt.value}
              onClick={() => onChange(opt.value)}
              style={{
                padding: '0.45rem 1rem',
                border: 'none',
                background: active ? 'rgba(245,158,11,0.18)' : 'transparent',
                color: active ? C.amber : C.muted,
                fontWeight: active ? 700 : 400,
                fontSize: 13,
                cursor: 'pointer',
                borderRight: i < OPTIONS.length - 1 ? `1px solid ${C.border}` : 'none',
                transition: 'all 0.15s',
              }}
            >
              {opt.label}
            </button>
          )
        })}
      </div>
      {value === 'user' && (
        <p style={{ fontSize: 12, color: C.muted, margin: '6px 0 0' }}>
          Follows each user through the steps in order. Events without a user ID are left out.
        </p>
      )}
      {value === 'user' && onWindowDaysChange && (
        <div style={{ marginTop: 10 }}>
          <label htmlFor="funnel-window-days" style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>
            Conversion window (days)
          </label>
          <input
            id="funnel-window-days"
            type="number"
            min={1}
            value={windowDays ?? DEFAULT_WINDOW_DAYS}
            onChange={(e) => onWindowDaysChange(Number(e.target.value))}
            style={{
              width: 100,
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              padding: '0.45rem 0.75rem',
              color: C.text,
              fontSize: 14,
              boxSizing: 'border-box',
              outline: 'none',
            }}
          />
        </div>
      )}
    </div>
  )
}
