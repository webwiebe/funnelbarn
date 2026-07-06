import { Link, useLocation } from 'react-router-dom'
import { C } from '../lib/theme'

// Sub-navigation shared by the cross-project ("All Projects") surfaces.
const TABS = [
  { to: '/overview', label: 'Dashboard' },
  { to: '/overview/events', label: 'All Events' },
  { to: '/overview/funnels', label: 'Cross-site Funnels' },
  { to: '/event-mapping', label: 'Normalization' },
]

export function OverviewTabs() {
  const location = useLocation()
  return (
    <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap', marginBottom: '1.25rem', borderBottom: `1px solid ${C.border}` }}>
      {TABS.map((t) => {
        const active = location.pathname === t.to
        return (
          <Link
            key={t.to}
            to={t.to}
            style={{
              textDecoration: 'none',
              color: active ? C.text : C.muted,
              fontSize: 13,
              fontWeight: 600,
              padding: '0.5rem 0.75rem',
              borderBottom: `2px solid ${active ? C.amber : 'transparent'}`,
              marginBottom: -1,
            }}
          >
            {t.label}
          </Link>
        )
      })}
    </div>
  )
}
