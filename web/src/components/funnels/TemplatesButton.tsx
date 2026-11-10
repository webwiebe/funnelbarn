import { useEffect, useRef, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { C } from '../../lib/theme'
import { FUNNEL_TEMPLATES, type FunnelTemplate } from './constants'

export function TemplatesButton({
  onSelect,
}: {
  onSelect: (tpl: FunnelTemplate) => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button
        onClick={() => setOpen((o) => !o)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          background: 'transparent',
          border: `1px solid ${C.border}`,
          borderRadius: 8,
          color: C.muted,
          padding: '0.6rem 1.1rem',
          cursor: 'pointer',
          fontSize: 14,
          fontWeight: 600,
        }}
      >
        Templates
        <ChevronDown size={14} />
      </button>
      {open && (
        <div style={{
          position: 'absolute',
          top: 'calc(100% + 6px)',
          right: 0,
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 10,
          boxShadow: '0 12px 32px rgba(0,0,0,0.4)',
          zIndex: 300,
          minWidth: 260,
          overflow: 'hidden',
        }}>
          {FUNNEL_TEMPLATES.map((tpl) => (
            <button
              key={tpl.name}
              onClick={() => { onSelect(tpl); setOpen(false) }}
              style={{
                display: 'block',
                width: '100%',
                textAlign: 'left',
                background: 'none',
                border: 'none',
                borderBottom: `1px solid ${C.border}`,
                padding: '0.75rem 1rem',
                cursor: 'pointer',
                color: C.text,
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(245,158,11,0.08)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'none')}
            >
              <div style={{ fontWeight: 700, fontSize: 13, marginBottom: 2 }}>{tpl.name}</div>
              <div style={{ fontSize: 11, color: C.muted }}>
                {tpl.steps.join(' → ')}
                <span style={{
                  marginLeft: 6,
                  background: 'rgba(245,158,11,0.15)',
                  color: C.amber,
                  borderRadius: 4,
                  padding: '0.1rem 0.4rem',
                  fontSize: 10,
                  fontWeight: 600,
                }}>
                  {tpl.scope === 'page_view' ? 'page view' : 'session'}
                </span>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
