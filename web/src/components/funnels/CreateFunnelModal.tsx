import { useState } from 'react'
import { X } from 'lucide-react'
import { api, Funnel, FunnelStepInput } from '../../lib/api'
import { trackEvent } from '../../lib/analytics'
import { FunnelTemplate } from './constants'
import { StepFilterEditor, StepWithFilters } from './StepFilterEditor'
import { ScopeToggle } from './ScopeToggle'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  success: '#10b981',
  error: '#ef4444',
}

export function CreateFunnelModal({
  projectId,
  onClose,
  onCreated,
  initialTemplate,
}: {
  projectId: string
  onClose: () => void
  onCreated: (f: Funnel) => void
  initialTemplate?: FunnelTemplate
}) {
  const [name, setName] = useState(initialTemplate?.name ?? '')
  const [scope, setScope] = useState<'session' | 'page_view'>(initialTemplate?.scope ?? 'session')
  const [steps, setSteps] = useState<StepWithFilters[]>(
    initialTemplate
      ? initialTemplate.steps.map((s) => ({ event_name: s, filters: [] }))
      : [{ event_name: '', filters: [] }, { event_name: '', filters: [] }]
  )
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCreate = async () => {
    if (!name.trim()) { setError('Name is required'); return }
    if (steps.some((s) => !s.event_name.trim())) { setError('All steps need an event name'); return }
    setSaving(true)
    setError(null)
    try {
      const apiSteps: FunnelStepInput[] = steps.map((s) => ({
        event_name: s.event_name,
        ...(s.filters.length > 0 ? { filters: s.filters.filter((f) => f.property && f.value) } : {}),
      }))
      const f = await api.createFunnel(projectId, name, apiSteps, scope)
      trackEvent('funnel_created', { funnel_name: name, step_count: steps.length })
      onCreated(f)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      background: 'rgba(0,0,0,0.7)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 500,
      padding: '1rem',
    }}>
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 14,
        padding: '2rem',
        width: '100%',
        maxWidth: 500,
        boxShadow: '0 25px 60px rgba(0,0,0,0.5)',
        maxHeight: '90vh',
        overflowY: 'auto',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>Create funnel</h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer' }}>
            <X size={18} />
          </button>
        </div>

        {error && (
          <div style={{
            background: 'rgba(239,68,68,0.1)',
            border: `1px solid rgba(239,68,68,0.3)`,
            borderRadius: 8,
            padding: '0.6rem 0.875rem',
            color: C.error,
            fontSize: 14,
            marginBottom: '1rem',
          }}>
            {error}
          </div>
        )}

        <div style={{ marginBottom: '1.25rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>Funnel name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Signup flow"
            style={{
              width: '100%',
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              padding: '0.6rem 0.875rem',
              color: C.text,
              fontSize: 14,
              boxSizing: 'border-box',
              outline: 'none',
            }}
            onFocus={(e) => (e.target.style.borderColor = C.amber)}
            onBlur={(e) => (e.target.style.borderColor = C.border)}
          />
        </div>

        <ScopeToggle value={scope} onChange={setScope} />

        <StepFilterEditor
          projectId={projectId}
          steps={steps}
          onChange={setSteps}
          minSteps={2}
        />

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button
            onClick={onClose}
            style={{
              background: 'transparent',
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              color: C.muted,
              padding: '0.6rem 1.25rem',
              cursor: 'pointer',
              fontSize: 14,
            }}
          >
            Cancel
          </button>
          <button
            onClick={handleCreate}
            disabled={saving}
            style={{
              background: saving ? '#78481a' : C.amber,
              border: 'none',
              borderRadius: 8,
              color: '#0f1117',
              padding: '0.6rem 1.25rem',
              cursor: saving ? 'not-allowed' : 'pointer',
              fontSize: 14,
              fontWeight: 700,
            }}
          >
            {saving ? 'Creating…' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  )
}
