import { useRef, useState, useEffect } from 'react'
import { ChevronDown, Check, Layers } from 'lucide-react'
import { api } from '../../lib/api'
import { useProjects } from '../../lib/projects'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
}

const LABELS: Record<string, string> = {
  '': 'All envs',
  production: 'Production',
  staging: 'Staging',
  test: 'Test',
  development: 'Development',
}

function label(env: string): string {
  return LABELS[env] ?? env
}

interface EnvironmentPickerProps {
  projectId?: string
}

export function EnvironmentPicker({ projectId }: EnvironmentPickerProps) {
  const { selectedEnvironment, setSelectedEnvironment } = useProjects()
  const [open, setOpen] = useState(false)
  const [environments, setEnvironments] = useState<string[]>([])
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!projectId) return
    api.getEnvironments(projectId)
      .then((d) => setEnvironments(d.environments ?? []))
      .catch(() => setEnvironments([]))
  }, [projectId])

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

  // Only show if there are recorded environments to filter by
  if (environments.length === 0) return null

  const options = ['', ...environments]

  return (
    <div ref={ref} style={{ position: 'relative', flexShrink: 0 }}>
      <button
        onClick={() => setOpen((v) => !v)}
        aria-label="Filter by environment"
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 4,
          fontSize: 12,
          color: selectedEnvironment ? C.amber : C.muted,
          background: C.surface,
          border: `1px solid ${selectedEnvironment ? C.amber : C.border}`,
          borderRadius: 4,
          padding: '0.2rem 0.5rem',
          cursor: 'pointer',
          maxWidth: 140,
        }}
      >
        <Layers size={11} style={{ flexShrink: 0 }} />
        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {label(selectedEnvironment)}
        </span>
        <ChevronDown
          size={11}
          style={{
            flexShrink: 0,
            transform: open ? 'rotate(180deg)' : 'none',
            transition: 'transform 0.15s',
          }}
        />
      </button>

      {open && (
        <div style={{
          position: 'absolute',
          top: 'calc(100% + 4px)',
          left: 0,
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 8,
          minWidth: 160,
          boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
          zIndex: 300,
          overflow: 'hidden',
        }}>
          {options.map((env) => (
            <button
              key={env || '__all__'}
              onClick={() => { setSelectedEnvironment(env); setOpen(false) }}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                width: '100%',
                background: env === selectedEnvironment ? 'rgba(245,158,11,0.08)' : 'transparent',
                border: 'none',
                padding: '0.55rem 0.875rem',
                cursor: 'pointer',
                textAlign: 'left',
                color: env === selectedEnvironment ? C.amber : C.text,
                fontSize: 13,
                fontWeight: env === selectedEnvironment ? 600 : 400,
                transition: 'background 0.1s',
                minHeight: 'unset',
              }}
              onMouseEnter={(e) => { if (env !== selectedEnvironment) (e.currentTarget as HTMLButtonElement).style.background = 'rgba(255,255,255,0.04)' }}
              onMouseLeave={(e) => { if (env !== selectedEnvironment) (e.currentTarget as HTMLButtonElement).style.background = 'transparent' }}
            >
              <span style={{ flex: 1 }}>{label(env)}</span>
              {env === selectedEnvironment && <Check size={13} />}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
