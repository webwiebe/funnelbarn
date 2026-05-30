import { ChevronRight, Home, RotateCcw } from 'lucide-react'
import { C } from './colors'

interface Props {
  crumbs: string[]
  onCrumbClick: (index: number) => void
  onReset: () => void
}

export function FlowBreadcrumb({ crumbs, onCrumbClick, onReset }: Props) {
  if (crumbs.length === 0) return null

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      gap: 4,
      flexWrap: 'wrap',
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 8,
      padding: '0.5rem 0.75rem',
    }}>
      <button
        onClick={onReset}
        title="Reset to top page"
        style={{
          background: 'none', border: 'none', cursor: 'pointer',
          color: C.muted, display: 'flex', alignItems: 'center', padding: 0, minHeight: 'unset',
        }}
      >
        <Home size={14} />
      </button>

      {crumbs.map((crumb, i) => (
        <span key={i} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          <ChevronRight size={12} color={C.muted} />
          <button
            onClick={() => onCrumbClick(i)}
            style={{
              background: 'none',
              border: 'none',
              cursor: i < crumbs.length - 1 ? 'pointer' : 'default',
              color: i === crumbs.length - 1 ? C.amber : C.muted,
              fontSize: 12,
              fontWeight: i === crumbs.length - 1 ? 600 : 400,
              padding: 0,
              minHeight: 'unset',
              maxWidth: 240,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {crumb}
          </button>
        </span>
      ))}

      {crumbs.length > 1 && (
        <button
          onClick={onReset}
          style={{
            marginLeft: 'auto',
            background: 'none',
            border: `1px solid ${C.border}`,
            borderRadius: 4,
            cursor: 'pointer',
            color: C.muted,
            fontSize: 11,
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            padding: '0.2rem 0.5rem',
            minHeight: 'unset',
          }}
        >
          <RotateCcw size={11} />
          Reset
        </button>
      )}
    </div>
  )
}
