import { Link, useLocation } from 'react-router-dom'
import { Settings, LogOut, User, ExternalLink } from 'lucide-react'
import { useAuth } from '../../lib/auth'
import { ProjectPicker } from '../ui/ProjectPicker'
import { type Project } from '../../lib/api'
import { C } from '../../lib/theme'

interface MoreSheetProps {
  projectId?: string
  projects: Project[]
  iambarnProfileURL: string | null
  onClose: () => void
  onLogout: () => void
}

export function MoreSheet({ projectId, projects, iambarnProfileURL, onClose, onLogout }: MoreSheetProps) {
  const { user } = useAuth()
  const location = useLocation()

  return (
    <>
      {/* Dark overlay */}
      <div
        onClick={onClose}
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

        {/* Project picker — show whenever there are projects to switch
            between, even on non-project-scoped routes like Settings. */}
        {projects.length > 0 && (
          <div style={{ marginBottom: '0.75rem' }}>
            <ProjectPicker
              projects={projects}
              base={location.pathname.split('/')[1] || 'dashboard'}
              projectId={projectId}
              variant="full"
              onSelect={onClose}
            />
          </div>
        )}

        {/* Settings link */}
        <Link
          to="/settings"
          onClick={onClose}
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

        {/* Edit IAMBarn profile — only when iambarn issuer is configured */}
        {iambarnProfileURL && (
          <a
            href={iambarnProfileURL}
            target="_blank"
            rel="noopener noreferrer"
            onClick={onClose}
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
            <ExternalLink size={18} color={C.muted} />
            Edit IAMBarn profile
          </a>
        )}

        {/* Logout */}
        <button
          onClick={() => { onClose(); onLogout() }}
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
  )
}
