import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { Check, FolderOpen, Search, Star } from 'lucide-react'
import type { Project } from '../../lib/api'
import { readFavouriteProjectIds, toggleFavouriteId, writeFavouriteProjectIds } from '../../lib/favouriteProjects'
import { filterProjects, groupProjects, menuOrder } from './projectPickerModel'

const C = {
  bg: '#0f1117',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
}

interface ProjectMenuProps {
  projects: Project[]
  currentId?: string
  onPick: (project: Project) => void
  /** Called on Escape. */
  onClose?: () => void
  /** Focus the search field on mount (dropdown); the sheet variant keeps the keyboard closed. */
  autoFocusSearch?: boolean
  /** Row size: `compact` for the header dropdown, `roomy` for the mobile sheet. */
  size?: 'compact' | 'roomy'
}

function rowSizes(roomy: boolean) {
  return {
    gap: roomy ? 12 : 8,
    pickPadding: roomy ? '0.75rem 0.5rem 0.75rem 1rem' : '0.6rem 0.5rem 0.6rem 0.875rem',
    starPadding: roomy ? '0.75rem 1rem 0.75rem 0.5rem' : '0.6rem 0.75rem 0.6rem 0.5rem',
    fontSize: roomy ? 14 : 13,
    iconSize: roomy ? 16 : 13,
    checkSize: roomy ? 15 : 13,
  }
}

function Row({ project, current, favourite, size, onPick, onToggle }: {
  project: Project
  current: boolean
  favourite: boolean
  size: 'compact' | 'roomy'
  onPick: () => void
  onToggle: () => void
}) {
  const roomy = size === 'roomy'
  const z = rowSizes(roomy)
  const label = `${favourite ? 'Remove' : 'Add'} ${project.name} ${favourite ? 'from' : 'to'} favourites`
  return (
    <div style={{ display: 'flex', alignItems: 'center', background: current ? 'rgba(245,158,11,0.08)' : 'transparent' }}>
      <button
        type="button"
        onClick={onPick}
        title={project.slug}
        aria-current={current ? 'true' : undefined}
        style={{
          display: 'flex', alignItems: 'center', gap: z.gap, flex: 1, minWidth: 0,
          background: 'transparent', border: 'none', padding: z.pickPadding, cursor: 'pointer', textAlign: 'left',
          color: current ? C.amber : C.text, fontSize: z.fontSize, fontWeight: current ? 600 : 400, minHeight: 'unset',
        }}
      >
        {roomy && <FolderOpen size={16} color={current ? C.amber : C.muted} style={{ flexShrink: 0 }} />}
        <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{project.name}</span>
        {current && <Check size={z.checkSize} />}
      </button>
      <button
        type="button"
        onClick={onToggle}
        aria-label={label}
        aria-pressed={favourite}
        style={{
          background: 'transparent', border: 'none', padding: z.starPadding, cursor: 'pointer',
          color: favourite ? C.amber : C.muted, minHeight: 'unset', display: 'flex',
        }}
      >
        <Star size={z.iconSize} fill={favourite ? C.amber : 'none'} />
      </button>
    </div>
  )
}

function GroupLabel({ children }: { children: string }) {
  return (
    <div style={{
      padding: '0.4rem 0.875rem 0.2rem',
      fontSize: 10,
      fontWeight: 700,
      letterSpacing: '0.06em',
      textTransform: 'uppercase',
      color: C.muted,
    }}>
      {children}
    </div>
  )
}

/**
 * The list inside the project switcher: search, a favourites group, and an
 * alphabetical rest, scrolling inside a capped height so the page behind never
 * moves. Shared by the header dropdown and the mobile sheet.
 */
export function ProjectMenu({ projects, currentId, onPick, onClose, autoFocusSearch = false, size = 'compact' }: ProjectMenuProps) {
  const [query, setQuery] = useState('')
  const [favouriteIds, setFavouriteIds] = useState<string[]>(readFavouriteProjectIds)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (autoFocusSearch) inputRef.current?.focus()
  }, [autoFocusSearch])

  const toggle = (id: string) => {
    const next = toggleFavouriteId(favouriteIds, id)
    setFavouriteIds(next)
    writeFavouriteProjectIds(next)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      onClose?.()
    } else if (e.key === 'Enter') {
      const first = menuOrder(projects, favouriteIds, query)[0]
      if (first) onPick(first)
    }
  }

  const groups = groupProjects(filterProjects(projects, query), favouriteIds)
  const empty = groups.favourites.length + groups.others.length === 0
  const showLabels = groups.favourites.length > 0
  const renderRows = (list: Project[]) => list.map((p) => (
    <Row
      key={p.id}
      project={p}
      size={size}
      current={p.id === currentId}
      favourite={favouriteIds.includes(p.id)}
      onPick={() => onPick(p)}
      onToggle={() => toggle(p.id)}
    />
  ))

  return (
    <div>
      <div style={{ position: 'relative', borderBottom: `1px solid ${C.border}` }}>
        <Search size={13} color={C.muted} style={{ position: 'absolute', left: 12, top: '50%', transform: 'translateY(-50%)' }} />
        <input
          ref={inputRef}
          type="search"
          role="searchbox"
          aria-label="Search projects"
          placeholder="Search projects"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onKeyDown}
          style={{
            width: '100%',
            boxSizing: 'border-box',
            background: C.bg,
            border: 'none',
            outline: 'none',
            color: C.text,
            fontSize: 16,
            padding: '0.55rem 0.75rem 0.55rem 2rem',
          }}
        />
      </div>
      <div style={{ maxHeight: '60vh', overflowY: 'auto', overscrollBehavior: 'contain' }}>
        {empty && <div style={{ padding: '0.75rem 0.875rem', fontSize: 13, color: C.muted }}>No projects match</div>}
        {groups.favourites.length > 0 && (
          <div role="group" aria-label="Favourites">
            <GroupLabel>Favourites</GroupLabel>
            {renderRows(groups.favourites)}
          </div>
        )}
        {groups.others.length > 0 && (
          <div role="group" aria-label={showLabels ? 'All projects' : 'Projects'}>
            {showLabels && <GroupLabel>All projects</GroupLabel>}
            {renderRows(groups.others)}
          </div>
        )}
      </div>
    </div>
  )
}
