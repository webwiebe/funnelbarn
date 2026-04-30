import { ReactNode, useState } from 'react'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { BarChart2, Layers, Radio, Settings, ChevronDown, LogOut, User, FlaskConical, MoreHorizontal, Star } from 'lucide-react'
import { useAuth } from '../../lib/auth'
import { useProjects } from '../../lib/projects'

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
  const { projects, defaultProjectId, setDefaultProjectId } = useProjects()
  const navigate = useNavigate()
  const location = useLocation()
  const [dropdownOpen, setDropdownOpen] = useState(false)
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [moreSheetOpen, setMoreSheetOpen] = useState(false)
  const effectiveId = projectId ?? (defaultProjectId && projects.some(p => p.id === defaultProjectId) ? defaultProjectId : projects[0]?.id)
  const currentProject = projects.find((p) => p.id === effectiveId) ?? projects[0]

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

  // Bottom tabs (excludes Settings — that goes in More sheet)
  const bottomTabs = [
    { to: projectId ? `/dashboard/${projectId}` : '/dashboard', label: 'Overview', icon: BarChart2 },
    { to: projectId ? `/funnels/${projectId}` : '/funnels', label: 'Funnels', icon: Layers },
    { to: projectId ? `/abtests/${projectId}` : '/abtests', label: 'A/B Tests', icon: FlaskConical },
    { to: projectId ? `/live/${projectId}` : '/live', label: 'Live', icon: Radio },
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
      }}>
        {/* Logo */}
        <Link to="/dashboard" style={{ textDecoration: 'none', display: 'flex', alignItems: 'center', gap: 8, marginRight: 8, flexShrink: 0 }}>
          <div style={{ fontSize: 20, color: C.amber }}>⬡</div>
          <span style={{ fontWeight: 700, fontSize: 17, color: C.text, letterSpacing: '-0.3px' }}>
            Funnel<span style={{ color: C.amber }}>Barn</span>
          </span>
        </Link>

        {/* Desktop: project switcher */}
        <div style={{ position: 'relative', flexShrink: 0 }} className="desktop-project-switcher">
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
              minWidth: 200,
              boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
              zIndex: 300,
            }}>
              {projects.map((p) => {
                const isDefault = p.id === defaultProjectId
                return (
                  <div
                    key={p.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      background: p.id === currentProject?.id ? '#2a2d3a' : 'transparent',
                    }}
                  >
                    <button
                      onClick={() => {
                        setDropdownOpen(false)
                        onProjectChange?.(p.id)
                        navigate(`/dashboard/${p.id}`)
                      }}
                      style={{
                        flex: 1,
                        textAlign: 'left',
                        background: 'transparent',
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
                    {projects.length > 1 && (
                      <button
                        onClick={(e) => { e.stopPropagation(); setDefaultProjectId(p.id) }}
                        title={isDefault ? 'Default project' : 'Set as default'}
                        style={{
                          background: 'transparent',
                          border: 'none',
                          cursor: 'pointer',
                          padding: '0.6rem 0.75rem 0.6rem 0',
                          color: isDefault ? C.amber : C.muted,
                          display: 'flex',
                          alignItems: 'center',
                          minHeight: 'unset',
                          flexShrink: 0,
                        }}
                      >
                        <Star size={13} fill={isDefault ? C.amber : 'none'} />
                      </button>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>

        {/* Mobile: compact project switcher (right side of top bar) */}
        <div className="mobile-project-switcher" style={{ marginLeft: 'auto', position: 'relative' }}>
          <button
            onClick={() => setDropdownOpen((v) => !v)}
            style={{
              background: 'transparent',
              border: `1px solid ${C.border}`,
              borderRadius: 6,
              color: C.muted,
              padding: '0.3rem 0.55rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: 5,
              fontSize: 12,
              minHeight: 'unset',
            }}
          >
            <span style={{ color: currentProject ? C.text : C.muted, maxWidth: 110, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {currentProject?.name ?? 'Select project'}
            </span>
            <ChevronDown size={12} color={C.muted} />
          </button>
          {dropdownOpen && (
            <div style={{
              position: 'absolute',
              top: '110%',
              right: 0,
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              minWidth: 200,
              boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
              zIndex: 300,
            }}>
              {projects.map((p) => {
                const isDefault = p.id === defaultProjectId
                return (
                  <div
                    key={p.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      background: p.id === currentProject?.id ? '#2a2d3a' : 'transparent',
                    }}
                  >
                    <button
                      onClick={() => {
                        setDropdownOpen(false)
                        onProjectChange?.(p.id)
                        navigate(`/dashboard/${p.id}`)
                      }}
                      style={{
                        flex: 1,
                        textAlign: 'left',
                        background: 'transparent',
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
                    {projects.length > 1 && (
                      <button
                        onClick={(e) => { e.stopPropagation(); setDefaultProjectId(p.id) }}
                        title={isDefault ? 'Default project' : 'Set as default'}
                        style={{
                          background: 'transparent',
                          border: 'none',
                          cursor: 'pointer',
                          padding: '0.6rem 0.75rem 0.6rem 0',
                          color: isDefault ? C.amber : C.muted,
                          display: 'flex',
                          alignItems: 'center',
                          minHeight: 'unset',
                          flexShrink: 0,
                        }}
                      >
                        <Star size={13} fill={isDefault ? C.amber : 'none'} />
                      </button>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>

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
      </nav>

      {/* Content */}
      <main className="shell-main" style={{ width: '100%', boxSizing: 'border-box' }}>
        <div style={{ maxWidth: 1280, margin: '0 auto', padding: '1.5rem 1rem', width: '100%', boxSizing: 'border-box' }}>
          {children}
        </div>
      </main>

      {/* Mobile: Bottom tab bar */}
      <nav className="bottom-tab-bar" style={{
        position: 'fixed',
        bottom: 0,
        left: 0,
        right: 0,
        background: C.surface,
        borderTop: `1px solid ${C.border}`,
        display: 'none',
        paddingBottom: 'env(safe-area-inset-bottom)',
        zIndex: 100,
      }}>
        {bottomTabs.map((tab) => {
          const active = isActive(tab.to)
          const Icon = tab.icon
          return (
            <Link
              key={tab.to}
              to={tab.to}
              style={{
                flex: 1,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                padding: '8px 0',
                gap: 3,
                textDecoration: 'none',
                color: active ? C.amber : C.muted,
                fontSize: 10,
                fontWeight: active ? 700 : 400,
                transition: 'color 0.15s',
              }}
            >
              <Icon size={20} />
              <span>{tab.label}</span>
            </Link>
          )
        })}

        {/* More tab */}
        <button
          onClick={() => setMoreSheetOpen(true)}
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '8px 0',
            gap: 3,
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            color: C.muted,
            fontSize: 10,
            fontWeight: 400,
            transition: 'color 0.15s',
          }}
        >
          <MoreHorizontal size={20} />
          <span>More</span>
        </button>
      </nav>

      {/* Mobile: More sheet overlay */}
      {moreSheetOpen && (
        <>
          {/* Dark overlay */}
          <div
            onClick={() => setMoreSheetOpen(false)}
            style={{
              position: 'fixed',
              inset: 0,
              background: 'rgba(0,0,0,0.5)',
              zIndex: 200,
            }}
          />

          {/* Slide-up sheet */}
          <div style={{
            position: 'fixed',
            bottom: 0,
            left: 0,
            right: 0,
            background: C.surface,
            borderTopLeftRadius: 20,
            borderTopRightRadius: 20,
            borderTop: `1px solid ${C.border}`,
            padding: '1rem 1.5rem',
            paddingBottom: 'calc(1rem + env(safe-area-inset-bottom))',
            zIndex: 201,
            animation: 'slideUp 0.2s ease-out',
          }}>
            {/* Drag handle */}
            <div style={{
              width: 36,
              height: 4,
              background: C.border,
              borderRadius: 2,
              margin: '0 auto 1.25rem',
            }} />

            {/* User info */}
            <div style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              padding: '0.5rem 0 1rem',
              borderBottom: `1px solid ${C.border}`,
              marginBottom: '0.75rem',
            }}>
              <div style={{
                width: 32,
                height: 32,
                borderRadius: '50%',
                background: C.bg,
                border: `1px solid ${C.border}`,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                flexShrink: 0,
              }}>
                <User size={15} color={C.muted} />
              </div>
              <div>
                <div style={{ fontSize: 14, fontWeight: 600, color: C.text }}>{user?.username}</div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 12, color: '#10b981', marginTop: 2 }}>
                  <span style={{
                    width: 6,
                    height: 6,
                    background: '#10b981',
                    borderRadius: '50%',
                    display: 'inline-block',
                    animation: 'pulse 2s infinite',
                  }} />
                  Live
                </div>
              </div>
            </div>

            {/* Settings link */}
            <Link
              to="/settings"
              onClick={() => setMoreSheetOpen(false)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 12,
                padding: '0.75rem 0',
                textDecoration: 'none',
                color: C.text,
                fontSize: 15,
                fontWeight: 500,
                borderBottom: `1px solid ${C.border}`,
              }}
            >
              <Settings size={18} color={C.muted} />
              Settings
            </Link>

            {/* Logout */}
            <button
              onClick={() => { setMoreSheetOpen(false); handleLogout() }}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 12,
                width: '100%',
                background: 'transparent',
                border: 'none',
                color: '#ef4444',
                padding: '0.75rem 0',
                cursor: 'pointer',
                fontSize: 15,
                fontWeight: 500,
                marginTop: 0,
                textAlign: 'left',
                minHeight: 'unset',
              }}
            >
              <LogOut size={18} />
              Log out
            </button>
          </div>
        </>
      )}

      <style>{`
        @keyframes pulse {
          0% { box-shadow: 0 0 0 0 rgba(16,185,129,0.4); }
          70% { box-shadow: 0 0 0 6px rgba(16,185,129,0); }
          100% { box-shadow: 0 0 0 0 rgba(16,185,129,0); }
        }
        @keyframes slideUp {
          from { transform: translateY(100%); }
          to { transform: translateY(0); }
        }

        /* Mobile layout */
        @media (max-width: 767px) {
          .desktop-nav { display: none !important; }
          .desktop-user-menu { display: none !important; }
          .desktop-live-indicator { display: none !important; }
          .desktop-project-switcher { display: none !important; }
          .mobile-project-switcher { display: flex !important; }
          .bottom-tab-bar { display: flex !important; }
          .shell-main { padding-bottom: calc(70px + env(safe-area-inset-bottom)) !important; }
        }

        /* Desktop layout */
        @media (min-width: 768px) {
          .desktop-nav { display: flex !important; }
          .desktop-user-menu { display: flex !important; }
          .desktop-project-switcher { display: block !important; }
          .mobile-project-switcher { display: none !important; }
          .bottom-tab-bar { display: none !important; }
        }
      `}</style>
    </div>
  )
}
