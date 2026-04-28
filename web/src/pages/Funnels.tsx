import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Plus, X, Layers } from 'lucide-react'
import Shell from '../components/Shell'
import { api, Funnel, FunnelAnalysis, FunnelStepInput } from '../lib/api'

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

function conversionColor(pct: number) {
  if (pct >= 0.6) return C.success
  if (pct >= 0.3) return C.amber
  return C.error
}

function CreateFunnelModal({
  projectId,
  onClose,
  onCreated,
}: {
  projectId: string
  onClose: () => void
  onCreated: (f: Funnel) => void
}) {
  const [name, setName] = useState('')
  const [steps, setSteps] = useState<FunnelStepInput[]>([{ event_name: '' }, { event_name: '' }])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const addStep = () => setSteps((s) => [...s, { event_name: '' }])
  const removeStep = (i: number) => setSteps((s) => s.filter((_, idx) => idx !== i))
  const updateStep = (i: number, val: string) =>
    setSteps((s) => s.map((st, idx) => (idx === i ? { event_name: val } : st)))

  const handleCreate = async () => {
    if (!name.trim()) { setError('Name is required'); return }
    if (steps.some((s) => !s.event_name.trim())) { setError('All steps need an event name'); return }
    setSaving(true)
    setError(null)
    try {
      const f = await api.createFunnel(projectId, name, steps)
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

        <div style={{ marginBottom: '1.25rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 10 }}>Steps</label>
          {steps.map((s, i) => (
            <div key={i} style={{ display: 'flex', gap: 8, marginBottom: 8, alignItems: 'center' }}>
              <div style={{
                width: 24,
                height: 24,
                background: C.amber,
                color: '#0f1117',
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 12,
                fontWeight: 800,
                flexShrink: 0,
              }}>
                {i + 1}
              </div>
              <input
                value={s.event_name}
                onChange={(e) => updateStep(i, e.target.value)}
                placeholder={`Event name, e.g. page_view`}
                style={{
                  flex: 1,
                  background: C.bg,
                  border: `1px solid ${C.border}`,
                  borderRadius: 8,
                  padding: '0.55rem 0.875rem',
                  color: C.text,
                  fontSize: 14,
                  outline: 'none',
                }}
                onFocus={(e) => (e.target.style.borderColor = C.amber)}
                onBlur={(e) => (e.target.style.borderColor = C.border)}
              />
              {steps.length > 2 && (
                <button
                  onClick={() => removeStep(i)}
                  style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer', padding: 4 }}
                >
                  <X size={14} />
                </button>
              )}
            </div>
          ))}
          <button
            onClick={addStep}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              background: 'transparent',
              border: `1px dashed ${C.border}`,
              borderRadius: 8,
              color: C.muted,
              padding: '0.5rem 1rem',
              cursor: 'pointer',
              fontSize: 13,
              width: '100%',
              justifyContent: 'center',
              marginTop: 4,
            }}
          >
            <Plus size={14} /> Add step
          </button>
        </div>

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

function FunnelDetail({ analysis }: { analysis: FunnelAnalysis }) {
  const maxCount = Math.max(...(analysis.results?.map((r) => r.Count) ?? [1]))

  return (
    <div>
      <h2 style={{ fontSize: 20, fontWeight: 800, marginBottom: 4 }}>{analysis.funnel.name}</h2>
      <p style={{ color: C.muted, fontSize: 13, marginBottom: '1.5rem' }}>
        {analysis.from?.slice(0, 10)} → {analysis.to?.slice(0, 10)}
      </p>

      {analysis.results?.map((step, i) => (
        <div key={step.StepOrder} style={{ marginBottom: '1.25rem' }}>
          {/* Drop-off indicator between steps */}
          {i > 0 && step.DropOff > 0 && (
            <div style={{
              fontSize: 12,
              color: C.error,
              marginBottom: 6,
              paddingLeft: 40,
              display: 'flex',
              alignItems: 'center',
              gap: 4,
            }}>
              <span>▼</span>
              {(step.DropOff * 100).toFixed(1)}% dropped off
            </div>
          )}

          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{
              width: 32,
              height: 32,
              background: conversionColor(step.Conversion),
              color: '#0f1117',
              borderRadius: '50%',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontWeight: 800,
              fontSize: 13,
              flexShrink: 0,
            }}>
              {i + 1}
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 14, fontWeight: 600, color: C.text, marginBottom: 6 }}>
                {step.EventName}
              </div>
              <div style={{ height: 20, background: '#2a2d3a', borderRadius: 4, overflow: 'hidden', position: 'relative' }}>
                <div style={{
                  position: 'absolute',
                  inset: 0,
                  width: `${(step.Count / maxCount) * 100}%`,
                  background: conversionColor(step.Conversion),
                  borderRadius: 4,
                  display: 'flex',
                  alignItems: 'center',
                  paddingLeft: 8,
                  transition: 'width 0.5s',
                }}>
                  <span style={{ fontSize: 11, fontWeight: 700, color: '#0f1117', whiteSpace: 'nowrap' }}>
                    {(step.Conversion * 100).toFixed(1)}%
                  </span>
                </div>
              </div>
            </div>
            <div style={{ textAlign: 'right', minWidth: 70 }}>
              <div style={{ fontSize: 18, fontWeight: 800, color: C.text }}>{step.Count.toLocaleString()}</div>
              <div style={{ fontSize: 12, color: C.muted }}>users</div>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

export default function Funnels() {
  const { projectId } = useParams<{ projectId?: string }>()
  const [funnels, setFunnels] = useState<Funnel[]>([])
  const [selected, setSelected] = useState<Funnel | null>(null)
  const [analysis, setAnalysis] = useState<FunnelAnalysis | null>(null)
  const [analysisLoading, setAnalysisLoading] = useState(false)
  const [showCreate, setShowCreate] = useState(false)

  useEffect(() => {
    if (!projectId) return
    api.listFunnels(projectId)
      .then((d) => setFunnels(d.funnels || []))
      .catch(console.error)
  }, [projectId])

  const loadAnalysis = (funnel: Funnel) => {
    if (!projectId) return
    setSelected(funnel)
    setAnalysis(null)
    setAnalysisLoading(true)
    api.getFunnelAnalysis(projectId, funnel.id)
      .then(setAnalysis)
      .catch(console.error)
      .finally(() => setAnalysisLoading(false))
  }

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, padding: '2rem', textAlign: 'center' }}>
          Select a project to view funnels.
        </div>
      </Shell>
    )
  }

  return (
    <Shell projectId={projectId}>
      {showCreate && projectId && (
        <CreateFunnelModal
          projectId={projectId}
          onClose={() => setShowCreate(false)}
          onCreated={(f) => {
            setFunnels((prev) => [...prev, f])
            setShowCreate(false)
            loadAnalysis(f)
          }}
        />
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Funnels</h1>
        <button
          onClick={() => setShowCreate(true)}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            background: C.amber,
            border: 'none',
            borderRadius: 8,
            color: '#0f1117',
            padding: '0.6rem 1.1rem',
            cursor: 'pointer',
            fontSize: 14,
            fontWeight: 700,
          }}
        >
          <Plus size={16} />
          Create funnel
        </button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '1.5rem' }}>
        {/* Funnel list */}
        <div>
          {funnels.length === 0 ? (
            <div style={{
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 12,
              padding: '2rem',
              textAlign: 'center',
              color: C.muted,
            }}>
              <Layers size={32} style={{ opacity: 0.3, marginBottom: 8 }} />
              <div style={{ fontSize: 14 }}>No funnels yet</div>
              <button
                onClick={() => setShowCreate(true)}
                style={{
                  marginTop: 12,
                  background: 'transparent',
                  border: `1px solid ${C.border}`,
                  borderRadius: 7,
                  color: C.amber,
                  padding: '0.4rem 0.8rem',
                  cursor: 'pointer',
                  fontSize: 13,
                  fontWeight: 600,
                }}
              >
                Create one
              </button>
            </div>
          ) : (
            funnels.map((f) => (
              <div
                key={f.id}
                onClick={() => loadAnalysis(f)}
                style={{
                  padding: '0.875rem 1rem',
                  marginBottom: '0.5rem',
                  cursor: 'pointer',
                  background: selected?.id === f.id ? 'rgba(245,158,11,0.1)' : C.surface,
                  border: `1px solid ${selected?.id === f.id ? 'rgba(245,158,11,0.4)' : C.border}`,
                  borderRadius: 10,
                  transition: 'all 0.15s',
                }}
              >
                <div style={{ fontWeight: 700, fontSize: 14, color: C.text }}>{f.name}</div>
                <div style={{ fontSize: 12, color: C.muted, marginTop: 3 }}>
                  {f.steps?.length ?? 0} steps
                </div>
              </div>
            ))
          )}
        </div>

        {/* Analysis panel */}
        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '1.5rem',
          minHeight: 300,
        }}>
          {!selected && (
            <div style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: 300,
              color: C.muted,
              gap: 8,
            }}>
              <Layers size={40} opacity={0.3} />
              <div>Select a funnel to view analysis</div>
            </div>
          )}
          {analysisLoading && (
            <div style={{ color: C.muted, padding: '2rem' }}>Loading analysis…</div>
          )}
          {analysis && !analysisLoading && (
            <FunnelDetail analysis={analysis} />
          )}
        </div>
      </div>
    </Shell>
  )
}
