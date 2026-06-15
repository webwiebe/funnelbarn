import { ReactNode, useEffect, useState } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { BarChart2, Layers, Radio, Settings, Flag, Lightbulb, GitBranch, Video, PlugZap } from 'lucide-react'
import { useAuth } from '../../lib/auth'
import { useProjects } from '../../lib/projects'
import { LAST_PROJECT_ID_KEY } from '../ui/ProjectPicker'
import { api, type Project } from '../../lib/api'
import { C } from '../../lib/theme'
import { TopNav } from './TopNav'
import { BottomTabBar } from './BottomTabBar'
import { MoreSheet } from './MoreSheet'
import { ShellStyles } from './ShellStyles'

interface ShellProps {
  children: ReactNode
  projectId?: string
  projectName?: string  // display name for current project (optional, derived from context if omitted)
  /** Override the list of projects shown in the picker (defaults to context). */
  projects?: Project[]
}

export default function Shell({ children, projectId, projectName, projects: projectsProp }: ShellProps) {
  const { logout } = useAuth()
  const { projects: ctxProjects } = useProjects()
  const navigate = useNavigate()
  const location = useLocation()
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [moreSheetOpen, setMoreSheetOpen] = useState(false)
  const [iambarnProfileURL, setIambarnProfileURL] = useState<string | null>(null)

  // Remember the active project so that when the user lands on a
  // non-project-scoped route (e.g. /settings), the picker can show their
  // last choice instead of defaulting to the alphabetical first.
  useEffect(() => {
    if (!projectId) return
    try {
      window.localStorage.setItem(LAST_PROJECT_ID_KEY, projectId)
    } catch {
      /* ignore — quota / disabled storage */
    }
  }, [projectId])

  useEffect(() => {
    // Only show the IAMBarn profile link when the session was actually
    // minted by an OIDC callback — the callback sets a non-HttpOnly
    // `funnelbarn_auth_method=oidc` cookie. Local password sessions
    // don't have it, so the menu entry stays hidden for them.
    const isOIDCSession = typeof document !== 'undefined' &&
      document.cookie.split(';').some((c) => c.trim() === 'funnelbarn_auth_method=oidc')
    if (!isOIDCSession) {
      return
    }
    let cancelled = false
    api.getClientConfig()
      .then((cfg) => {
        if (!cancelled && cfg.iambarn?.profile_url) {
          setIambarnProfileURL(cfg.iambarn.profile_url)
        }
      })
      .catch(() => { /* non-fatal — menu item simply hidden */ })
    return () => { cancelled = true }
  }, [])

  const projects = projectsProp ?? ctxProjects

  // Derive display name from context if not explicitly provided
  const resolvedProjectName =
    projectName ?? (projectId ? projects.find((p) => p.id === projectId)?.name : undefined)

  const handleLogout = async () => {
    await logout()
    navigate('/')
  }

  const navLinks = [
    { to: projectId ? `/dashboard/${projectId}` : '/dashboard', label: 'Overview', icon: <BarChart2 size={16} /> },
    { to: projectId ? `/flows/${projectId}` : '/flows', label: 'Flows', icon: <GitBranch size={16} /> },
    { to: projectId ? `/funnels/${projectId}` : '/funnels', label: 'Funnels', icon: <Layers size={16} /> },
    { to: projectId ? `/flags/${projectId}` : '/flags', label: 'Flags', icon: <Flag size={16} /> },
    { to: projectId ? `/insights/${projectId}` : '/insights', label: 'Insights', icon: <Lightbulb size={16} /> },
    { to: projectId ? `/live/${projectId}` : '/live', label: 'Live', icon: <Radio size={16} /> },
    { to: projectId ? `/sessions/${projectId}` : '/sessions', label: 'Sessions', icon: <Video size={16} /> },
    { to: '/integrations', label: 'Integrations', icon: <PlugZap size={16} /> },
    { to: '/settings', label: 'Settings', icon: <Settings size={16} /> },
  ]

  const isActive = (to: string) => {
    const base = to.split('/')[1]
    return location.pathname.includes(`/${base}`)
  }

  return (
    <div style={{ minHeight: '100vh', width: '100%', background: C.bg, color: C.text, fontFamily: 'system-ui, -apple-system, sans-serif', overflowX: 'hidden' }}>
      <TopNav
        projectId={projectId}
        resolvedProjectName={resolvedProjectName}
        projects={projects}
        navLinks={navLinks}
        userMenuOpen={userMenuOpen}
        onUserMenuToggle={() => setUserMenuOpen((v) => !v)}
        iambarnProfileURL={iambarnProfileURL}
        onLogout={handleLogout}
        isActive={isActive}
      />

      {/* Content */}
      <main className="shell-main" style={{ width: '100%', boxSizing: 'border-box' }}>
        <div style={{ maxWidth: 1280, margin: '0 auto', padding: '1.5rem 1rem', width: '100%', boxSizing: 'border-box' }}>
          {children}
        </div>
      </main>

      <BottomTabBar
        projectId={projectId}
        onMoreOpen={() => setMoreSheetOpen(true)}
        isActive={isActive}
      />

      {moreSheetOpen && (
        <MoreSheet
          projectId={projectId}
          projects={projects}
          iambarnProfileURL={iambarnProfileURL}
          onClose={() => setMoreSheetOpen(false)}
          onLogout={handleLogout}
        />
      )}

      <ShellStyles />
    </div>
  )
}
