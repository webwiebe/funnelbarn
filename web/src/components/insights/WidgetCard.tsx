import { Trash2, Columns2, Columns3, Square } from 'lucide-react'
import type { DashboardWidget, PropertyBreakdown } from '../../lib/api'
import { C } from '../../lib/theme'

const sizeIcons = { 1: Square, 2: Columns2, 3: Columns3 } as const
const sizeLabels = { 1: 'Small (1 col)', 2: 'Medium (2 cols)', 3: 'Wide (3 cols)' } as const

function isUrl(value: string): boolean {
  return /^https?:\/\//.test(value)
}

function BreakdownValue({ value }: { value: string }) {
  if (isUrl(value)) {
    return (
      <a
        href={value}
        target="_blank"
        rel="noopener noreferrer"
        style={{
          color: C.amber,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          maxWidth: '70%',
          textDecoration: 'none',
        }}
        title={value}
        onMouseOver={(e) => (e.currentTarget.style.textDecoration = 'underline')}
        onMouseOut={(e) => (e.currentTarget.style.textDecoration = 'none')}
      >
        {value}
      </a>
    )
  }
  return (
    <span style={{
      color: C.text,
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap',
      maxWidth: '70%',
    }} title={value}>
      {value}
    </span>
  )
}

export function WidgetCard({ widget, breakdown, onDelete, onResize }: {
  widget: DashboardWidget
  breakdown: PropertyBreakdown[]
  onDelete: () => void
  onResize: () => void
}) {
  const isCountOnly = !widget.property
  const maxCount = breakdown.length > 0 ? breakdown[0].count : 1
  const total = breakdown.reduce((sum, r) => sum + r.count, 0)
  const size = (widget.size || 1) as 1 | 2 | 3
  const nextSize = size >= 3 ? 1 : (size + 1) as 1 | 2 | 3
  const SizeIcon = sizeIcons[nextSize]

  const subtitle = widget.property
    ? `${widget.event_name} → ${widget.property}`
    : widget.event_name

  return (
    <div style={{
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
      overflow: 'hidden',
      gridColumn: `span ${size}`,
    }}>
      <div style={{
        padding: '1rem 1.25rem',
        borderBottom: `1px solid ${C.border}`,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}>
        <div>
          <div style={{ fontSize: 14, fontWeight: 700, color: C.text }}>
            {widget.title || subtitle}
          </div>
          {widget.title && (
            <div style={{ fontSize: 12, color: C.muted, marginTop: 2 }}>
              {subtitle}
            </div>
          )}
        </div>
        <div style={{ display: 'flex', gap: 4 }}>
          <button
            onClick={onResize}
            style={{
              background: 'transparent',
              border: 'none',
              color: C.muted,
              cursor: 'pointer',
              padding: 4,
              borderRadius: 4,
              display: 'flex',
            }}
            title={sizeLabels[nextSize]}
          >
            <SizeIcon size={14} />
          </button>
          <button
            onClick={onDelete}
            style={{
              background: 'transparent',
              border: 'none',
              color: C.muted,
              cursor: 'pointer',
              padding: 4,
              borderRadius: 4,
              display: 'flex',
            }}
            title="Delete widget"
          >
            <Trash2 size={14} />
          </button>
        </div>
      </div>

      <div style={{ padding: '1rem 1.25rem' }}>
        {breakdown.length === 0 ? (
          <div style={{ color: C.muted, fontSize: 13, textAlign: 'center', padding: '1rem 0' }}>
            No data yet
          </div>
        ) : isCountOnly ? (
          <div style={{ textAlign: 'center', padding: '1rem 0' }}>
            <div style={{ fontSize: 36, fontWeight: 800, color: C.amber }}>{breakdown[0].count.toLocaleString()}</div>
            <div style={{ fontSize: 13, color: C.muted, marginTop: 4 }}>events</div>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {breakdown.map((row) => (
              <div key={row.value}>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 4 }}>
                  <BreakdownValue value={row.value} />
                  <span style={{ color: C.muted, flexShrink: 0, marginLeft: 8 }}>
                    {row.count} <span style={{ fontSize: 11 }}>({total > 0 ? Math.round(row.count / total * 100) : 0}%)</span>
                  </span>
                </div>
                <div style={{
                  height: 6,
                  background: C.bg,
                  borderRadius: 3,
                  overflow: 'hidden',
                }}>
                  <div style={{
                    height: '100%',
                    width: `${(row.count / maxCount) * 100}%`,
                    background: C.amber,
                    borderRadius: 3,
                    transition: 'width 0.3s ease',
                  }} />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div style={{
        padding: '0.5rem 1.25rem',
        borderTop: `1px solid ${C.border}`,
        fontSize: 11,
        color: C.muted,
      }}>
        Rolling window: last 100 events
      </div>
    </div>
  )
}
