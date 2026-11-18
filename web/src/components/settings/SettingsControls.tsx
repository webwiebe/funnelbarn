import { C } from '../../lib/theme'

export function ToggleSwitch({ on, onToggle, disabled, label }: {
  on: boolean
  onToggle: () => void
  disabled?: boolean
  label?: string
}) {
  return (
    <button
      onClick={onToggle}
      disabled={disabled}
      aria-label={label}
      aria-pressed={label ? on : undefined}
      style={{
        position: 'relative',
        width: 44,
        height: 24,
        borderRadius: 99,
        border: 'none',
        background: on ? C.amber : C.border,
        cursor: disabled ? 'not-allowed' : 'pointer',
        transition: 'background 0.2s',
        flexShrink: 0,
      }}
    >
      <span style={{
        position: 'absolute',
        top: 3,
        left: on ? 23 : 3,
        width: 18,
        height: 18,
        borderRadius: '50%',
        background: on ? '#0f1117' : C.muted,
        transition: 'left 0.2s',
      }} />
    </button>
  )
}

export function PillOptions({ options, value, suffix, onSelect }: {
  options: number[]
  value: number
  suffix: string
  onSelect: (n: number) => void
}) {
  return (
    <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
      {options.map((n) => (
        <button
          key={n}
          onClick={() => onSelect(n)}
          style={{
            padding: '4px 14px', borderRadius: 99, fontSize: 13, fontWeight: 600, cursor: 'pointer',
            border: `1px solid ${value === n ? C.amber : C.border}`,
            background: value === n ? 'rgba(245,158,11,0.1)' : 'none',
            color: value === n ? C.amber : C.muted,
          }}
        >
          {n}{suffix}
        </button>
      ))}
    </div>
  )
}
