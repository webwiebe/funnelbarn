import { useEffect, useState } from 'react'
import { CheckCircle, Circle, RotateCcw } from 'lucide-react'
import { api, Project, ProjectHealth } from '../../lib/api'
import { reportError } from '../../lib/bugbarn'
import { FEATURES } from '../integrations/features'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  error: '#ef4444',
  success: '#10b981',
}

interface ProjectHealthCardProps {
  projects: Project[]
  defaultProjectId: string | null
  /** Notified whenever the loaded health / selected project changes, so a
   *  sibling (e.g. the agent-instruction generator) can share the same data. */
  onHealthChange?: (health: ProjectHealth | null, project: Project | null) => void
}

export function ProjectHealthCard({ projects, defaultProjectId, onHealthChange }: ProjectHealthCardProps) {
  const [selectedId, setSelectedId] = useState<string>('')
  const [health, setHealth] = useState<ProjectHealth | null>(null)
  const [loading, setLoading] = useState(false)
  const [resetting, setResetting] = useState(false)
  const [resetConfirm, setResetConfirm] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Resolve selected project on mount or when defaultProjectId/projects change.
  useEffect(() => {
    const id = defaultProjectId ?? projects[0]?.id ?? ''
    setSelectedId(id)
  }, [defaultProjectId, projects])

  useEffect(() => {
    if (!selectedId) return
    setLoading(true)
    setError(null)
    api.getProjectHealth(selectedId)
      .then((h) => {
        setHealth(h)
        onHealthChange?.(h, projects.find((p) => p.id === selectedId) ?? null)
      })
      .catch((e) => { reportError(e, { source: 'ProjectHealthCard.getProjectHealth' }); setError(String(e)) })
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId])

  const handleReset = async () => {
    if (!resetConfirm) { setResetConfirm(true); return }
    setResetting(true)
    setResetConfirm(false)
    try {
      await api.resetProjectHealth(selectedId)
      const updated = await api.getProjectHealth(selectedId)
      setHealth(updated)
      onHealthChange?.(updated, projects.find((p) => p.id === selectedId) ?? null)
    } catch (e) {
      reportError(e, { source: 'ProjectHealthCard.resetProjectHealth' })
      setError(String(e))
    } finally {
      setResetting(false)
    }
  }

  if (projects.length === 0) return null

  const allChecked = health
    ? FEATURES.every((f) => health[f.key] as boolean)
    : false

  return (
    <div style={{
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
      overflow: 'hidden',
      marginBottom: '2rem',
    }}>
      {/* Header */}
      <div style={{
        padding: '1.25rem 1.5rem',
        borderBottom: `1px solid ${C.border}`,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 12,
        flexWrap: 'wrap',
      }}>
        <div>
          <div style={{ fontWeight: 700, fontSize: 15 }}>Integration Health</div>
          <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
            Which features have been called at least once by the implementing agent.
          </div>
        </div>
        {projects.length > 1 && (
          <select
            value={selectedId}
            onChange={(e) => setSelectedId(e.target.value)}
            style={{
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              color: C.text,
              fontSize: 13,
              padding: '0.4rem 0.75rem',
              cursor: 'pointer',
            }}
          >
            {projects.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
        )}
      </div>

      {/* Feature rows */}
      <div style={{ padding: '0.5rem 0' }}>
        {loading ? (
          <div style={{ padding: '1.25rem 1.5rem', color: C.muted, fontSize: 13 }}>Loading…</div>
        ) : error ? (
          <div style={{ padding: '1.25rem 1.5rem', color: C.error, fontSize: 13 }}>{error}</div>
        ) : (
          FEATURES.map((f, i) => {
            const checked = health ? (health[f.key] as boolean) : false
            return (
              <div
                key={f.key}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '0.75rem 1.5rem',
                  borderBottom: i < FEATURES.length - 1 ? `1px solid ${C.border}` : undefined,
                }}
              >
                {checked
                  ? <CheckCircle size={18} color={C.success} />
                  : <Circle size={18} color={C.muted} />
                }
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 14, color: C.text, fontWeight: checked ? 600 : 400 }}>
                    {f.label}
                  </div>
                  <div style={{ fontSize: 12, color: C.muted }}>{f.description}</div>
                </div>
              </div>
            )
          })
        )}
      </div>

      {/* Footer */}
      {!loading && !error && (
        <div style={{
          padding: '0.75rem 1.5rem',
          borderTop: `1px solid ${C.border}`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 8,
        }}>
          {allChecked ? (
            <div style={{ fontSize: 13, color: C.success, fontWeight: 600 }}>
              All features active
            </div>
          ) : (
            <div style={{ fontSize: 13, color: C.muted }}>
              {health ? FEATURES.filter((f) => health[f.key] as boolean).length : 0} / {FEATURES.length} active
            </div>
          )}
          <button
            onClick={handleReset}
            disabled={resetting}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              background: resetConfirm ? 'rgba(239,68,68,0.1)' : 'transparent',
              border: `1px solid ${resetConfirm ? C.error : C.border}`,
              borderRadius: 8,
              color: resetConfirm ? C.error : C.muted,
              fontSize: 13,
              padding: '0.35rem 0.75rem',
              cursor: resetting ? 'default' : 'pointer',
              opacity: resetting ? 0.6 : 1,
              transition: 'all 0.15s',
            }}
          >
            <RotateCcw size={13} />
            {resetConfirm ? 'Confirm reset' : 'Reset'}
          </button>
        </div>
      )}
    </div>
  )
}
