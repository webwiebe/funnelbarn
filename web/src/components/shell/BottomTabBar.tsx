import { Link } from 'react-router-dom'
import { BarChart2, Layers, MoreHorizontal, GitBranch, Video } from 'lucide-react'
import { C } from '../../lib/theme'

interface BottomTab {
  to: string
  label: string
  icon: React.ElementType
}

interface BottomTabBarProps {
  projectId?: string
  onMoreOpen: () => void
  isActive: (to: string) => boolean
}

export function BottomTabBar({ projectId, onMoreOpen, isActive }: BottomTabBarProps) {
  const bottomTabs: BottomTab[] = [
    { to: projectId ? `/dashboard/${projectId}` : '/dashboard', label: 'Overview', icon: BarChart2 },
    { to: projectId ? `/flows/${projectId}` : '/flows', label: 'Flows', icon: GitBranch },
    { to: projectId ? `/funnels/${projectId}` : '/funnels', label: 'Funnels', icon: Layers },
    { to: projectId ? `/sessions/${projectId}` : '/sessions', label: 'Sessions', icon: Video },
  ]

  return (
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
        onClick={onMoreOpen}
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
  )
}
