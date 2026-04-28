import { ReactNode, useEffect, useState } from 'react'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { BarChart2, Layers, Radio, Settings, ChevronDown, LogOut, User } from 'lucide-react'
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
    { to: projectId ? `/live/${projectId}` : '/live', label: 'Live', icon: <Radio size={16} /> },
    { to: '/settings', label: 'Settings', icon: <Settings size={16} /> },
  ]

  const isActive = (to: string) => {
    const base = to.split('/')[1]
    return location.pathname.includes(`/${base}`)
  }

  return (
    <div style={{ minHeight: '100vh', background: C.bg, color: C.text, fontFamily: 'system-ui, -apple-system, sans-serif' }}>
      {/* Top Nav */}
      <nav style={{
        background: C.surface,
        borderBottom: `1px solid ${C.border}`,
        display: 'flex',
        alignItems: 'center',
        padding: '0 1.5rem',
        height: 56,
        position: 'sticky',
        top: 0,
        zIndex: 100,
      }}>
        {/* Logo */}
        <Link to="/dashboard" style={{ textDecoration: 'none', display: 'flex', alignItems: 'center', gap: 8, marginRight: 24 }}>
          <div style={{ fontSize: 20, color: C.amber }}>⬡</div>
          <span style={{ fontWeight: 700, fontSize: 17, color: C.text, letterSpacing: '-0.3px' }}>
            Funnel<span style={{ color: C.amber }}>Barn</span>
          </span>
        </Link>

        {/* Project switcher */}
        <div style={{ position: 'relative', marginRight: 24 }}>
          <button
            onClick={() => setDropdownOpen((v) => !v)}
            style={{
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 6,
              color: C.text,
              padding: '0.35rem 0.75rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              fontSize: 14,
            }}
          >
            {currentProject ? currentProject.name : 'Select project'}
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
                  }}
                >
                  {p.name}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Nav links */}
        <div style={{ display: 'flex', gap: 4, flex: 1 }}>
          {navLinks.map((link) => (
            <Link
              key={link.to}
              to={link.to}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                padding: '0.4rem 0.85rem',
                borderRadius: 6,
                textDecoration: 'none',
                fontSize: 14,
                fontWeight: 500,
                color: isActive(link.to) ? C.amber : C.muted,
                background: isActive(link.to) ? 'rgba(245,158,11,0.1)' : 'transparent',
                transition: 'all 0.15s',
              }}
            >
              {link.icon}
              {link.label}
            </Link>
          ))}
        </div>

        {/* Right side */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
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
            }} />
            Live
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
                padding: '0.35rem 0.75rem',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                fontSize: 14,
              }}
            >
              <User size={14} />
              {user?.username}
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

      {/* Content */}
      <main style={{ maxWidth: 1280, margin: '0 auto', padding: '2rem 1.5rem' }}>
        {children}
      </main>

      <style>{`
        @keyframes pulse {
          0% { box-shadow: 0 0 0 0 rgba(16,185,129,0.4); }
          70% { box-shadow: 0 0 0 6px rgba(16,185,129,0); }
          100% { box-shadow: 0 0 0 0 rgba(16,185,129,0); }
        }
      `}</style>
    </div>
  )
}
