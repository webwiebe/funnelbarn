import { Link, useLocation } from 'react-router-dom'
import { Settings, LogOut, User, UserCog, Flag, Radio, Lightbulb, PlugZap, Globe, ListOrdered, Filter, Shuffle } from 'lucide-react'
import { useAuth } from '../../lib/auth'
import { ProjectPicker } from '../ui/ProjectPicker'
import { type Project } from '../../lib/api'
import { iambarnThemeVars, type IambarnConfig } from '../../lib/iambarn-widget'
import { C } from '../../lib/theme'

interface MoreSheetProps {
  projectId?: string
  projects: Project[]
  /** IAMBarn component config, set only for IAMBarn/OIDC sessions. */
  iambarn: IambarnConfig | null
  /** Whether the hosted IAMBarn widget bundle has loaded and upgraded. */
  iambarnReady: boolean
  onClose: () => void
  onLogout: () => void
}

export function MoreSheet({ projectId, projects, iambarn, iambarnReady, onClose, onLogout }: MoreSheetProps) {
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

        {/* Overflow nav links */}
        {[
          { to: '/overview', label: 'All Projects', icon: Globe },
          { to: '/overview/events', label: 'All Events', icon: ListOrdered },
          { to: '/overview/funnels', label: 'Cross-site Funnels', icon: Filter },
          { to: '/event-mapping', label: 'Event Normalization', icon: Shuffle },
          { to: projectId ? `/flags/${projectId}` : '/flags', label: 'Flags', icon: Flag },
          { to: projectId ? `/live/${projectId}` : '/live', label: 'Live', icon: Radio },
          { to: projectId ? `/insights/${projectId}` : '/insights', label: 'Insights', icon: Lightbulb },
          { to: '/integrations', label: 'Integrations', icon: PlugZap },
        ].map(({ to, label, icon: Icon }) => (
          <Link
            key={to}
            to={to}
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
            <Icon size={18} color={C.muted} />
            {label}
          </Link>
        ))}

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

        {/* Manage account — routes to the local /account page (hosted
            <iambarn-profile>). Shown only for IAMBarn/OIDC sessions. */}
        {iambarn && (
          <Link
            to="/account"
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
            <UserCog size={18} color={C.muted} />
            Manage account
          </Link>
        )}

        {/* Logout — for OIDC sessions use IAMBarn's RP-initiated logout (ends
            the IAMBarn session too); otherwise clear the local session. */}
        {iambarn?.server_url && iambarn?.client_id && iambarnReady ? (
          <div style={{ padding: '0.75rem 0' }}>
            <iambarn-logout-button
              server-url={iambarn.server_url}
              client-id={iambarn.client_id}
              post-logout-redirect-uri={iambarn.post_logout_redirect_uri}
              label="Log out"
              style={iambarnThemeVars}
            />
          </div>
        ) : (
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
        )}
      </div>
    </>
  )
}
