import { Link, useLocation } from 'react-router-dom'
import { LogOut, User, ExternalLink } from 'lucide-react'
import { useAuth } from '../../lib/auth'
import { ProjectPicker } from '../ui/ProjectPicker'
import { EnvironmentPicker } from '../ui/EnvironmentPicker'
import { type Project } from '../../lib/api'
import { C } from '../../lib/theme'

interface NavLink {
  to: string
  label: string
  icon: React.ReactNode
}

interface TopNavProps {
  projectId?: string
  resolvedProjectName?: string
  projects: Project[]
  navLinks: NavLink[]
  userMenuOpen: boolean
  onUserMenuToggle: () => void
  iambarnProfileURL: string | null
  onLogout: () => void
  isActive: (to: string) => boolean
}

export function TopNav({
  projectId,
  resolvedProjectName,
  projects,
  navLinks,
  userMenuOpen,
  onUserMenuToggle,
  iambarnProfileURL,
  onLogout,
  isActive,
}: TopNavProps) {
  const { user } = useAuth()
  const location = useLocation()

  return (
    <nav style={{
      background: C.surface,
      borderBottom: `1px solid ${C.border}`,
      display: 'flex',
      alignItems: 'center',
      padding: '0 1rem',
      height: 56,
      position: 'sticky',
      top: 0,
      zIndex: 100,
      gap: 8,
      width: '100%',
      boxSizing: 'border-box',
    }}>
      {/* Logo */}
      <Link to="/dashboard" style={{ textDecoration: 'none', display: 'flex', alignItems: 'center', gap: 8, marginRight: 8, flexShrink: 0 }}>
        <div style={{ fontSize: 20, color: C.amber }}>⬡</div>
        <span style={{ fontWeight: 700, fontSize: 17, color: C.text, letterSpacing: '-0.3px' }}>
          Funnel<span style={{ color: C.amber }}>Barn</span>
        </span>
      </Link>

      {/* Project picker badge — shown on every page that has projects,
          including non-project-scoped routes like Settings. */}
      {projects.length > 0 && (
        <div style={{ marginRight: 8 }}>
          <ProjectPicker
            projects={projects}
            base={location.pathname.split('/')[1] || 'dashboard'}
            projectId={projectId}
            variant="badge"
          />
        </div>
      )}
      {/* Environment picker — only visible when the active project has recorded environments */}
      {projectId && (
        <div style={{ marginRight: 8 }}>
          <EnvironmentPicker projectId={projectId} />
        </div>
      )}
      {/* Fallback static badge when no projects loaded yet but name is known */}
      {projects.length === 0 && resolvedProjectName && (
        <span style={{
          fontSize: 12, color: '#94a3b8', background: '#1a1d27',
          border: '1px solid #2a2d3a', borderRadius: 4,
          padding: '0.2rem 0.5rem', marginRight: 8, flexShrink: 0,
          maxWidth: 150, overflow: 'hidden', textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}>
          {resolvedProjectName}
        </span>
      )}

      {/* Desktop: Nav links */}
      <div className="desktop-nav" style={{ display: 'flex', gap: 4, flex: 1 }}>
        {navLinks.map((link) => (
          <Link
            key={link.to}
            to={link.to}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              padding: '0.4rem 0.75rem',
              borderRadius: 6,
              textDecoration: 'none',
              fontSize: 13,
              fontWeight: 500,
              color: isActive(link.to) ? C.amber : C.muted,
              background: isActive(link.to) ? 'rgba(245,158,11,0.1)' : 'transparent',
              transition: 'all 0.15s',
              minHeight: 'unset',
              whiteSpace: 'nowrap',
            }}
          >
            {link.icon}
            <span className="nav-label">{link.label}</span>
          </Link>
        ))}
      </div>

      {/* Desktop: Right side user menu + live indicator */}
      <div className="desktop-user-menu" style={{ display: 'flex', alignItems: 'center', gap: 8, marginLeft: 'auto' }}>
        {/* Live indicator */}
        <div className="desktop-live-indicator" style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: '#10b981' }}>
          <span style={{
            display: 'inline-block',
            width: 8,
            height: 8,
            background: '#10b981',
            borderRadius: '50%',
            boxShadow: '0 0 0 0 rgba(16,185,129,0.4)',
            animation: 'pulse 2s infinite',
            flexShrink: 0,
          }} />
          <span className="live-label">Live</span>
        </div>

        {/* User menu */}
        <div style={{ position: 'relative' }}>
          <button
            onClick={onUserMenuToggle}
            style={{
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 6,
              color: C.text,
              padding: '0.35rem 0.6rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              fontSize: 13,
              minHeight: 'unset',
            }}
          >
            <User size={14} />
            <span className="user-name">{user?.username}</span>
          </button>
          {userMenuOpen && (
            <div style={{
              position: 'absolute',
              top: '110%',
              right: 0,
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              minWidth: 200,
              boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
              zIndex: 200,
            }}>
              {iambarnProfileURL && (
                <a
                  href={iambarnProfileURL}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={onUserMenuToggle}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 8,
                    width: '100%',
                    textAlign: 'left',
                    background: 'transparent',
                    border: 'none',
                    borderBottom: `1px solid ${C.border}`,
                    color: C.text,
                    padding: '0.6rem 1rem',
                    cursor: 'pointer',
                    fontSize: 14,
                    textDecoration: 'none',
                    boxSizing: 'border-box',
                  }}
                >
                  <ExternalLink size={14} />
                  Edit IAMBarn profile
                </a>
              )}
              <button
                onClick={onLogout}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  width: '100%',
                  textAlign: 'left',
                  background: 'transparent',
                  border: 'none',
                  color: '#ef4444',
                  padding: '0.6rem 1rem',
                  cursor: 'pointer',
                  fontSize: 14,
                  minHeight: 'unset',
                }}
              >
                <LogOut size={14} />
                Logout
              </button>
            </div>
          )}
        </div>
      </div>
    </nav>
  )
}
