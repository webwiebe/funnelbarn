import { useRef, useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ChevronDown, Check, FolderOpen } from 'lucide-react'
import type { Project } from '../../lib/api'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
}

interface ProjectPickerProps {
  projects: Project[]
  /** The base path segment used for navigation, e.g. "dashboard" or "funnels". */
  base: string
  /** Callback invoked after a project is selected (e.g. to close a parent sheet). */
  onSelect?: () => void
  /** Compact mode — used in the top nav badge. Full mode — used in "More" sheet. */
  variant?: 'badge' | 'full'
}

export function ProjectPicker({ projects, base, onSelect, variant = 'badge' }: ProjectPickerProps) {
  const navigate = useNavigate()
  const { projectId: urlProjectId } = useParams<{ projectId?: string }>()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const currentProject = projects.find((p) => p.id === urlProjectId) ?? projects[0]

  // Close dropdown when clicking outside
  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  const handleSelect = (project: Project) => {
    setOpen(false)
    onSelect?.()
    navigate(`/${base}/${project.id}`)
  }

  if (projects.length === 0) return null

  if (variant === 'badge') {
    return (
      <div ref={ref} style={{ position: 'relative', flexShrink: 0 }}>
        <button
          onClick={() => setOpen((v) => !v)}
          aria-label="Switch project"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            fontSize: 12,
            color: C.muted,
            background: C.surface,
            border: `1px solid ${C.border}`,
            borderRadius: 4,
            padding: '0.2rem 0.5rem',
            cursor: projects.length > 1 ? 'pointer' : 'default',
            maxWidth: 160,
            pointerEvents: projects.length > 1 ? 'auto' : 'none',
          }}
        >
          <FolderOpen size={11} style={{ flexShrink: 0 }} />
          <span style={{
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}>
            {currentProject?.name ?? 'Select project'}
          </span>
          {projects.length > 1 && (
            <ChevronDown
              size={11}
              style={{
                flexShrink: 0,
                transform: open ? 'rotate(180deg)' : 'none',
                transition: 'transform 0.15s',
              }}
            />
          )}
        </button>

        {open && (
          <div style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            left: 0,
            background: C.surface,
            border: `1px solid ${C.border}`,
            borderRadius: 8,
            minWidth: 180,
            maxWidth: 280,
            boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
            zIndex: 300,
            overflow: 'hidden',
          }}>
            {projects.map((p) => (
              <button
                key={p.id}
                onClick={() => handleSelect(p)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  width: '100%',
                  background: p.id === currentProject?.id ? 'rgba(245,158,11,0.08)' : 'transparent',
                  border: 'none',
                  padding: '0.6rem 0.875rem',
                  cursor: 'pointer',
                  textAlign: 'left',
                  color: p.id === currentProject?.id ? C.amber : C.text,
                  fontSize: 13,
                  fontWeight: p.id === currentProject?.id ? 600 : 400,
                  transition: 'background 0.1s',
                  minHeight: 'unset',
                }}
                onMouseEnter={(e) => { if (p.id !== currentProject?.id) (e.currentTarget as HTMLButtonElement).style.background = 'rgba(255,255,255,0.04)' }}
                onMouseLeave={(e) => { if (p.id !== currentProject?.id) (e.currentTarget as HTMLButtonElement).style.background = 'transparent' }}
              >
                <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {p.name}
                </span>
                {p.id === currentProject?.id && <Check size={13} />}
              </button>
            ))}
          </div>
        )}
      </div>
    )
  }

  // variant === 'full' — used in the More sheet (mobile)
  return (
    <div style={{ paddingBottom: '0.25rem' }}>
      <div style={{
        fontSize: 11,
        fontWeight: 700,
        color: C.muted,
        textTransform: 'uppercase',
        letterSpacing: '0.06em',
        marginBottom: '0.5rem',
      }}>
        Switch Project
      </div>
      <div style={{
        background: C.bg,
        border: `1px solid ${C.border}`,
        borderRadius: 10,
        overflow: 'hidden',
      }}>
        {projects.map((p, i) => (
          <button
            key={p.id}
            onClick={() => handleSelect(p)}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 12,
              width: '100%',
              background: p.id === currentProject?.id ? 'rgba(245,158,11,0.07)' : 'transparent',
              border: 'none',
              borderBottom: i < projects.length - 1 ? `1px solid ${C.border}` : 'none',
              padding: '0.75rem 1rem',
              cursor: 'pointer',
              textAlign: 'left',
              color: p.id === currentProject?.id ? C.amber : C.text,
              fontSize: 14,
              fontWeight: p.id === currentProject?.id ? 600 : 400,
              minHeight: 'unset',
            }}
          >
            <FolderOpen size={16} color={p.id === currentProject?.id ? C.amber : C.muted} style={{ flexShrink: 0 }} />
            <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {p.name}
            </span>
            {p.id === currentProject?.id && <Check size={15} color={C.amber} />}
          </button>
        ))}
      </div>
    </div>
  )
}
