import { useEffect, useState } from 'react'
import { api } from '../../lib/api'
import { C } from '../../lib/theme'

const LABELS: Record<string, string> = {
  production: 'Production',
  staging: 'Staging',
  test: 'Test',
  development: 'Development',
}

// One-click environment filter for the funnel analysis. '' means all environments.
export function EnvironmentSwitch({
  projectId,
  value,
  onChange,
}: {
  projectId: string
  value: string
  onChange: (env: string) => void
}) {
  const [recorded, setRecorded] = useState<string[]>([])

  useEffect(() => {
    api.getEnvironments(projectId)
      .then((d) => setRecorded(d.environments ?? []))
      .catch(() => setRecorded([]))
  }, [projectId])

  // Production is always offered so a single click hides test and staging traffic.
  const options = ['', 'production', ...recorded.filter((e) => e && e !== 'production')]

  return (
    <div style={{ marginBottom: '1rem' }}>
      <div role="group" aria-label="Environment" style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
        <span style={{ fontSize: 13, color: C.muted }}>Environment</span>
        {options.map((env) => {
          const active = env === value
          return (
            <button
              key={env || '__all__'}
              aria-pressed={active}
              onClick={() => onChange(env)}
              style={{
                padding: '0.35rem 0.875rem',
                borderRadius: 99,
                border: `1px solid ${active ? C.amber : C.border}`,
                background: active ? 'rgba(245,158,11,0.15)' : 'rgba(245,158,11,0.04)',
                color: active ? C.amber : C.muted,
                cursor: 'pointer',
                fontSize: 13,
                fontWeight: active ? 600 : 400,
              }}
            >
              {env === '' ? 'All' : (LABELS[env] ?? env)}
            </button>
          )
        })}
      </div>
      {value !== '' && (
        <div style={{ fontSize: 12, color: C.muted, marginTop: 6 }}>
          Events recorded without an environment appear only under All.
        </div>
      )}
    </div>
  )
}
