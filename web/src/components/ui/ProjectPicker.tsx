import { useRef, useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ChevronDown, FolderOpen } from 'lucide-react'
import type { Project } from '../../lib/api'
import { readLastProjectId, writeLastProjectId } from '../../lib/selectedProject'
import { ProjectMenu } from './ProjectMenu'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
}

/**
 * Routes that take a `/:projectId` segment. Selecting a project there swaps the
 * id inline.
 */
const PROJECT_SCOPED_BASES = new Set(['dashboard', 'funnels', 'flags', 'insights', 'live', 'flows'])

/**
 * Routes that have no `/:projectId` segment but still act on the selected
 * project. Switching project there must stay on the page — sending someone to
 * the dashboard mid-task is how you end up changing project settings for a
 * project you are no longer looking at.
 */
const PROJECT_AWARE_BASES = new Set(['settings'])

interface ProjectPickerProps {
  projects: Project[]
  /** The base path segment used for navigation, e.g. "dashboard" or "funnels". */
  base: string
  /**
   * The currently-active project id, when known. Pages that are not
   * project-scoped (e.g. Settings) may omit this; the picker then falls back
   * to the last-selected project from localStorage, then to the first project.
   */
  projectId?: string
  /** Callback invoked after a project is selected (e.g. to close a parent sheet). */
  onSelect?: () => void
  /** Compact mode — used in the top nav badge. Full mode — used in "More" sheet. */
  variant?: 'badge' | 'full'
}

// pickCurrentProject resolves the "current" project: explicit prop > URL
// param > remembered id > first project.
function pickCurrentProject(projects: Project[], projectId?: string, urlProjectId?: string): Project | undefined {
  const effectiveProjectId = projectId ?? urlProjectId ?? readLastProjectId()
  return projects.find((p) => p.id === effectiveProjectId) ?? projects[0]
}

function BadgeButton({ name, multiple, open, onToggle }: { name?: string; multiple: boolean; open: boolean; onToggle: () => void }) {
  return (
    <button
      onClick={onToggle}
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
        cursor: multiple ? 'pointer' : 'default',
        maxWidth: 160,
        pointerEvents: multiple ? 'auto' : 'none',
      }}
    >
      <FolderOpen size={11} style={{ flexShrink: 0 }} />
      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{name ?? 'Select project'}</span>
      {multiple && (
        <ChevronDown
          size={11}
          style={{ flexShrink: 0, transform: open ? 'rotate(180deg)' : 'none', transition: 'transform 0.15s' }}
        />
      )}
    </button>
  )
}

export function ProjectPicker({ projects, base, projectId, onSelect, variant = 'badge' }: ProjectPickerProps) {
  const navigate = useNavigate()
  const { projectId: urlProjectId } = useParams<{ projectId?: string }>()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const currentProject = pickCurrentProject(projects, projectId, urlProjectId)

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
    writeLastProjectId(project.id)
    // On project-scoped routes, swap the id inline. On a project-aware route
    // with no id in the URL (/settings) the page re-reads the selection itself,
    // so stay put. Anywhere else, take the user to the project's dashboard —
    // the canonical landing page for a project context.
    if (PROJECT_AWARE_BASES.has(base)) return
    const target = PROJECT_SCOPED_BASES.has(base) ? base : 'dashboard'
    navigate(`/${target}/${project.id}`)
  }

  if (projects.length === 0) return null

  if (variant === 'badge') {
    return (
      <div ref={ref} style={{ position: 'relative', flexShrink: 0 }}>
        <BadgeButton name={currentProject?.name} multiple={projects.length > 1} open={open} onToggle={() => setOpen((v) => !v)} />

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
            <ProjectMenu
              projects={projects}
              currentId={currentProject?.id}
              onPick={handleSelect}
              onClose={() => setOpen(false)}
              autoFocusSearch
            />
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
        <ProjectMenu projects={projects} currentId={currentProject?.id} onPick={handleSelect} size="roomy" />
      </div>
    </div>
  )
}
