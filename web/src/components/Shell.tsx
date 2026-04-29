import { ReactNode, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { BarChart2, Layers, Radio, Settings, ChevronDown, LogOut, User, FlaskConical, Menu, X } from 'lucide-react'
import { useAuth } from '../lib/auth'
import { api, Project } from '../lib/api'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
}

interface ShellProps {
  children: ReactNode
  projectId?: string
  onProjectChange?: (id: string) => void
}

export default function Shell({ children, projectId, onProjectChange }: ShellProps) {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [projects, setProjects] = useState<Project[]>([])
  const [dropdownOpen, setDropdownOpen] = useState(false)
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const mobileMenuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    api.listProjects()
      .then((d) => setProjects(d.projects || []))
      .catch(() => setProjects([]))
  }, [])

  const currentProject = projects.find((p) => p.id === projectId) || projects[0]

  const handleLogout = async () => {
    await logout()
    navigate('/')
  }

  const navLinks = [
    { to: projectId ? `/dashboard/${projectId}` : '/dashboard', label: 'Overview', icon: <BarChart2 size={16} /> },
    { to: projectId ? `/funnels/${projectId}` : '/funnels', label: 'Funnels', icon: <Layers size={16} /> },
    { to: projectId ? `/abtests/${projectId}` : '/abtests', label: 'A/B Tests', icon: <FlaskConical size={16} /> },
    { to: projectId ? `/live/${projectId}` : '/live', label: 'Live', icon: <Radio size={16} /> },
    { to: '/settings', label: 'Settings', icon: <Settings size={16} /> },
  ]

  const isActive = (to: string) => {
    const base = to.split('/')[1]
    return location.pathname.includes(`/${base}`)
  }

  return (
    <div style={{ minHeight: '100vh', width: '100%', background: C.bg, color: C.text, fontFamily: 'system-ui, -apple-system, sans-serif', overflowX: 'hidden' }}>
      {/* Top Nav */}
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
        overflow: 'hidden',
      }}>
        {/* Logo */}
        <Link to="/dashboard" style={{ textDecoration: 'none', display: 'flex', alignItems: 'center', gap: 8, marginRight: 8, flexShrink: 0 }}>
          <div style={{ fontSize: 20, color: C.amber }}>⬡</div>
          <span style={{ fontWeight: 700, fontSize: 17, color: C.text, letterSpacing: '-0.3px' }}>
            Funnel<span style={{ color: C.amber }}>Barn</span>
          </span>
        </Link>

        {/* Project switcher — hidden on mobile, use hamburger menu instead */}
        <div style={{ position: 'relative', flexShrink: 0 }} className="project-switcher desktop-only">
          <button
            onClick={() => setDropdownOpen((v) => !v)}
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
            <span className="project-name">{currentProject ? currentProject.name : 'Project'}</span>
            <ChevronDown size={14} color={C.muted} />
          </button>
          {dropdownOpen && (
            <div style={{
              position: 'absolute',
              top: '110%',
              left: 0,
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              minWidth: 180,
              boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
              zIndex: 200,
            }}>
              {projects.map((p) => (
                <button
                  key={p.id}
                  onClick={() => {
                    setDropdownOpen(false)
                    onProjectChange?.(p.id)
                    navigate(`/dashboard/${p.id}`)
                  }}
                  style={{
                    display: 'block',
                    width: '100%',
                    textAlign: 'left',
                    background: p.id === currentProject?.id ? '#2a2d3a' : 'transparent',
                    border: 'none',
                    color: C.text,
                    padding: '0.6rem 1rem',
                    cursor: 'pointer',
                    fontSize: 14,
                    minHeight: 'unset',
                  }}
                >
                  {p.name}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Nav links — hidden on mobile */}
        <div className="nav-links" style={{ display: 'flex', gap: 4, flex: 1 }}>
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

        {/* Right side — hidden on mobile except hamburger */}
        <div className="nav-right" style={{ display: 'flex', alignItems: 'center', gap: 8, marginLeft: 'auto' }}>
          {/* Live indicator */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: '#10b981' }}>
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
              onClick={() => setUserMenuOpen((v) => !v)}
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
                minWidth: 140,
                boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
                zIndex: 200,
              }}>
                <button
                  onClick={handleLogout}
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

        {/* Hamburger — mobile only */}
        <button
          className="hamburger"
          onClick={() => setMobileMenuOpen((v) => !v)}
          style={{
            background: 'transparent',
            border: 'none',
            color: C.text,
            cursor: 'pointer',
            padding: '0.35rem',
            display: 'none',
            alignItems: 'center',
            justifyContent: 'center',
            marginLeft: 'auto',
            minHeight: 'unset',
          }}
          aria-label="Toggle menu"
        >
          {mobileMenuOpen ? <X size={22} /> : <Menu size={22} />}
        </button>
      </nav>

      {/* Mobile slide-down menu */}
      {mobileMenuOpen && (
        <div
          ref={mobileMenuRef}
          style={{
            position: 'fixed',
            top: 56,
            left: 0,
            right: 0,
            background: C.surface,
            borderBottom: `1px solid ${C.border}`,
            zIndex: 99,
            boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
            padding: '0.5rem 0',
          }}
        >
          {/* Project switcher in mobile menu */}
          {projects.length > 0 && (
            <>
              <div style={{ padding: '0.5rem 1.25rem 0.25rem', fontSize: 11, fontWeight: 600, color: C.muted, textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                Project
              </div>
              {projects.map((p) => (
                <button
                  key={p.id}
                  onClick={() => {
                    setMobileMenuOpen(false)
                    onProjectChange?.(p.id)
                    navigate(`/dashboard/${p.id}`)
                  }}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    width: '100%',
                    textAlign: 'left',
                    background: p.id === currentProject?.id ? 'rgba(245,158,11,0.08)' : 'transparent',
                    border: 'none',
                    color: p.id === currentProject?.id ? C.amber : C.text,
                    padding: '0.6rem 1.25rem',
                    cursor: 'pointer',
                    fontSize: 14,
                    fontWeight: p.id === currentProject?.id ? 600 : 400,
                    borderLeft: `3px solid ${p.id === currentProject?.id ? C.amber : 'transparent'}`,
                    minHeight: 'unset',
                  }}
                >
                  {p.name}
                </button>
              ))}
              <div style={{ borderTop: `1px solid ${C.border}`, margin: '0.5rem 0' }} />
            </>
          )}

          {/* Nav links */}
          {navLinks.map((link) => (
            <Link
              key={link.to}
              to={link.to}
              onClick={() => setMobileMenuOpen(false)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                padding: '0.75rem 1.25rem',
                textDecoration: 'none',
                fontSize: 15,
                fontWeight: 500,
                color: isActive(link.to) ? C.amber : C.text,
                background: isActive(link.to) ? 'rgba(245,158,11,0.08)' : 'transparent',
                borderLeft: `3px solid ${isActive(link.to) ? C.amber : 'transparent'}`,
                minHeight: 'unset',
              }}
            >
              {link.icon}
              {link.label}
            </Link>
          ))}

          {/* User + logout */}
          <div style={{ borderTop: `1px solid ${C.border}`, margin: '0.5rem 0' }} />
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0.5rem 1.25rem' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 14, color: C.muted }}>
              <User size={14} />
              <span>{user?.username}</span>
              <span style={{ display: 'flex', alignItems: 'center', gap: 4, marginLeft: 8, color: '#10b981', fontSize: 13 }}>
                <span style={{ width: 7, height: 7, background: '#10b981', borderRadius: '50%', display: 'inline-block', animation: 'pulse 2s infinite' }} />
                Live
              </span>
            </div>
            <button
              onClick={() => { setMobileMenuOpen(false); handleLogout() }}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                background: 'transparent',
                border: 'none',
                color: '#ef4444',
                cursor: 'pointer',
                fontSize: 14,
                minHeight: 'unset',
                padding: '0.25rem 0.5rem',
              }}
            >
              <LogOut size={14} />
              Logout
            </button>
          </div>
        </div>
      )}

      {/* Content */}
      <main style={{ width: '100%', boxSizing: 'border-box' }}>
        <div style={{ maxWidth: 1280, margin: '0 auto', padding: '1.5rem 1rem', width: '100%', boxSizing: 'border-box' }}>
          {children}
        </div>
      </main>

      <style>{`
        @keyframes pulse {
          0% { box-shadow: 0 0 0 0 rgba(16,185,129,0.4); }
          70% { box-shadow: 0 0 0 6px rgba(16,185,129,0); }
          100% { box-shadow: 0 0 0 0 rgba(16,185,129,0); }
        }
        /* Mobile: logo + hamburger only */
        @media (max-width: 767px) {
          .nav-links { display: none !important; }
          .hamburger { display: flex !important; }
          .nav-right { display: none !important; }
          .desktop-only { display: none !important; }
        }
        /* Desktop: full nav, no hamburger */
        @media (min-width: 768px) {
          .hamburger { display: none !important; }
          .nav-links { display: flex !important; }
          .nav-right { display: flex !important; }
          .desktop-only { display: block !important; }
        }
      `}</style>
    </div>
  )
}
