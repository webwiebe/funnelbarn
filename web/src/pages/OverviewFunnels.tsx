import { useEffect, useMemo, useState } from 'react'
import { Plus, Trash2, X } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api, CanonicalEvent, CanonicalFunnel, CanonicalFunnelAnalysis } from '../lib/api'
import { OverviewTabs } from '../components/OverviewTabs'
import { C } from '../lib/theme'

const SEGMENTS = [
  { label: 'All visitors', value: 'all' },
  { label: 'Logged in', value: 'logged_in' },
  { label: 'Anonymous', value: 'not_logged_in' },
  { label: 'Mobile', value: 'mobile' },
  { label: 'Desktop', value: 'desktop' },
  { label: 'Tablet', value: 'tablet' },
  { label: 'New visitor', value: 'new_visitor' },
  { label: 'Returning', value: 'returning' },
]

export default function OverviewFunnels() {
  const [catalog, setCatalog] = useState<CanonicalEvent[]>([])
  const [funnels, setFunnels] = useState<CanonicalFunnel[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [analysis, setAnalysis] = useState<CanonicalFunnelAnalysis | null>(null)
  const [segment, setSegment] = useState('all')
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  const labelFor = useMemo(() => {
    const m = new Map(catalog.map((c) => [c.key, c.label]))
    return (k: string) => m.get(k) ?? k
  }, [catalog])

  const loadFunnels = () => api.listCanonicalFunnels().then((r) => setFunnels(r.funnels))

  useEffect(() => {
    Promise.all([api.listCanonicalEvents(), loadFunnels()])
      .then(([c]) => setCatalog(c.canonical_events))
      .catch((e) => setError(e.message))
  }, [])

  // Run analysis whenever the selected funnel or segment changes.
  useEffect(() => {
    if (!selectedId) { setAnalysis(null); return }
    setError(null)
    api.analyzeCanonicalFunnel(selectedId, { segment })
      .then(setAnalysis)
      .catch((e) => setError(e.message || 'analysis failed'))
  }, [selectedId, segment])

  const del = (id: string) => {
    api.deleteCanonicalFunnel(id).then(() => {
      if (selectedId === id) setSelectedId(null)
      loadFunnels()
    }).catch((e) => setError(e.message))
  }

  return (
    <Shell>
      <OverviewTabs />
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.25rem', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: C.text, margin: 0 }}>Cross-site Funnels</h1>
          <div style={{ color: C.muted, fontSize: 13, marginTop: 4 }}>Drop-off over canonical events, aggregated across every site</div>
        </div>
        <button onClick={() => setCreating(true)} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, background: C.amber, color: '#1a1d27', border: 'none', borderRadius: 8, padding: '0.45rem 1rem', fontSize: 13, fontWeight: 600, cursor: 'pointer' }}>
          <Plus size={15} /> New funnel
        </button>
      </div>

      {error && <div style={{ background: 'rgba(239,68,68,0.1)', border: `1px solid ${C.error}`, color: C.error, borderRadius: 8, padding: '0.6rem 0.9rem', marginBottom: '1rem' }}>{error}</div>}

      {catalog.length === 0 && (
        <div style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 8, padding: '0.75rem 1rem', color: C.muted, fontSize: 13, marginBottom: '1rem' }}>
          Define canonical events and map your sites' raw events first (Event Normalization) — funnel steps are built from canonical events.
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(220px, 1fr) 2fr', gap: 16, alignItems: 'start' }}>
        {/* Funnel list */}
        <div style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, padding: '0.75rem' }}>
          {funnels.length === 0 && <div style={{ color: C.muted, fontSize: 13, padding: '0.5rem' }}>No funnels yet.</div>}
          {funnels.map((f) => (
            <div
              key={f.id}
              onClick={() => setSelectedId(f.id)}
              style={{
                display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8,
                padding: '0.6rem 0.75rem', borderRadius: 8, cursor: 'pointer',
                background: selectedId === f.id ? C.bg : 'transparent',
              }}
            >
              <div>
                <div style={{ color: C.text, fontSize: 14, fontWeight: 500 }}>{f.name}</div>
                <div style={{ color: C.muted, fontSize: 12 }}>{f.steps.length} steps</div>
              </div>
              <button onClick={(e) => { e.stopPropagation(); del(f.id) }} title="Delete" style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer', display: 'inline-flex' }}>
                <Trash2 size={14} />
              </button>
            </div>
          ))}
        </div>

        {/* Analysis */}
        <div style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, padding: '1.25rem', minHeight: 200 }}>
          {!analysis ? (
            <div style={{ color: C.muted, fontSize: 13 }}>Select a funnel to see cross-site drop-off.</div>
          ) : (
            <>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, marginBottom: '1rem', flexWrap: 'wrap' }}>
                <div style={{ fontSize: 16, fontWeight: 600, color: C.text }}>{analysis.funnel.name}</div>
                <select value={segment} onChange={(e) => setSegment(e.target.value)} style={{ background: C.bg, border: `1px solid ${C.border}`, color: C.text, borderRadius: 8, padding: '0.35rem 0.6rem', fontSize: 13 }}>
                  {SEGMENTS.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
                </select>
              </div>

              {/* Aggregate step bars */}
              <StepBars steps={analysis.result.steps.map((s) => ({ label: labelFor(s.event_name), count: s.count, conversion: s.conversion, drop_off: s.drop_off }))} />

              {/* Excluded projects */}
              {analysis.result.excluded_projects?.length > 0 && (
                <div style={{ marginTop: '1rem', fontSize: 12, color: C.muted }}>
                  <strong style={{ color: C.text }}>Excluded sites:</strong>{' '}
                  {analysis.result.excluded_projects.map((e) => `${e.project_name} (${e.reason})`).join('; ')}
                </div>
              )}

              {/* Per-project breakdown */}
              {analysis.result.by_project?.length > 0 && (
                <div style={{ marginTop: '1.5rem' }}>
                  <div style={{ fontSize: 13, fontWeight: 600, color: C.text, marginBottom: '0.5rem' }}>By site</div>
                  <div style={{ overflowX: 'auto' }}>
                    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
                      <thead>
                        <tr style={{ color: C.muted, textAlign: 'left' }}>
                          <th style={{ padding: '0.35rem 0.5rem' }}>Site</th>
                          {analysis.result.steps.map((s, i) => (
                            <th key={i} style={{ padding: '0.35rem 0.5rem', textAlign: 'right' }}>{labelFor(s.event_name)}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {analysis.result.by_project.map((bp) => (
                          <tr key={bp.project_id} style={{ borderTop: `1px solid ${C.border}`, color: C.text }}>
                            <td style={{ padding: '0.4rem 0.5rem' }}>{bp.project_name}</td>
                            {bp.steps.map((s, i) => (
                              <td key={i} style={{ padding: '0.4rem 0.5rem', textAlign: 'right' }}>{s.count.toLocaleString()}</td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </div>

      {creating && (
        <CreateFunnelModal
          catalog={catalog}
          onClose={() => setCreating(false)}
          onCreated={(id) => { setCreating(false); loadFunnels().then(() => setSelectedId(id)) }}
          onError={setError}
        />
      )}
    </Shell>
  )
}

function StepBars({ steps }: { steps: { label: string; count: number; conversion: number; drop_off: number }[] }) {
  const entry = steps[0]?.count ?? 0
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
      {steps.map((s, i) => (
        <div key={i}>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, color: C.text, marginBottom: 3 }}>
            <span>{i + 1}. {s.label}</span>
            <span style={{ color: C.muted }}>
              {s.count.toLocaleString()} · {(s.conversion * 100).toFixed(1)}%
              {i > 0 && <span style={{ color: C.error }}> (−{(s.drop_off * 100).toFixed(1)}%)</span>}
            </span>
          </div>
          <div style={{ height: 10, background: C.border, borderRadius: 4 }}>
            <div style={{ width: `${entry > 0 ? (s.count / entry) * 100 : 0}%`, height: '100%', background: C.amber, borderRadius: 4 }} />
          </div>
        </div>
      ))}
    </div>
  )
}

function CreateFunnelModal({ catalog, onClose, onCreated, onError }: {
  catalog: CanonicalEvent[]
  onClose: () => void
  onCreated: (id: string) => void
  onError: (m: string) => void
}) {
  const [name, setName] = useState('')
  const [steps, setSteps] = useState<string[]>(catalog.length ? [catalog[0].key] : [])
  const [scope, setScope] = useState<'session' | 'page_view'>('session')
  const [saving, setSaving] = useState(false)

  const addStep = () => setSteps((s) => [...s, catalog[0]?.key ?? ''])
  const setStep = (i: number, key: string) => setSteps((s) => s.map((v, idx) => (idx === i ? key : v)))
  const removeStep = (i: number) => setSteps((s) => s.filter((_, idx) => idx !== i))

  const submit = () => {
    if (!name.trim() || steps.length === 0) return
    setSaving(true)
    api.createCanonicalFunnel({ name: name.trim(), scope, steps: steps.map((k) => ({ canonical_key: k })) })
      .then((f) => onCreated(f.id))
      .catch((e) => { onError(e.message || 'create failed'); setSaving(false) })
  }

  const inputStyle: React.CSSProperties = { background: C.bg, border: `1px solid ${C.border}`, color: C.text, borderRadius: 8, padding: '0.45rem 0.6rem', fontSize: 13, width: '100%' }

  return (
    <>
      <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', zIndex: 200 }} />
      <div style={{ position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%, -50%)', width: 'min(480px, 92vw)', background: C.surface, border: `1px solid ${C.border}`, borderRadius: 14, padding: '1.5rem', zIndex: 201, maxHeight: '85vh', overflowY: 'auto' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <div style={{ fontSize: 16, fontWeight: 700, color: C.text }}>New cross-site funnel</div>
          <button onClick={onClose} style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer' }}><X size={18} /></button>
        </div>

        <label style={{ fontSize: 12, color: C.muted }}>Name</label>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Signup flow" style={{ ...inputStyle, margin: '4px 0 12px' }} />

        <label style={{ fontSize: 12, color: C.muted }}>Count by</label>
        <select value={scope} onChange={(e) => setScope(e.target.value as 'session' | 'page_view')} style={{ ...inputStyle, margin: '4px 0 12px' }}>
          <option value="session">Sessions</option>
          <option value="page_view">Page views</option>
        </select>

        <label style={{ fontSize: 12, color: C.muted }}>Steps</label>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, margin: '4px 0 12px' }}>
          {steps.map((key, i) => (
            <div key={i} style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
              <span style={{ color: C.muted, fontSize: 13, width: 16 }}>{i + 1}</span>
              <select value={key} onChange={(e) => setStep(i, e.target.value)} style={inputStyle}>
                {catalog.map((c) => <option key={c.key} value={c.key}>{c.label}</option>)}
              </select>
              <button onClick={() => removeStep(i)} disabled={steps.length === 1} style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer' }}><Trash2 size={14} /></button>
            </div>
          ))}
          <button onClick={addStep} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, background: 'transparent', border: `1px solid ${C.border}`, color: C.text, borderRadius: 8, padding: '0.4rem', fontSize: 13, cursor: 'pointer' }}>
            <Plus size={14} /> Add step
          </button>
        </div>

        <button onClick={submit} disabled={saving || !name.trim() || steps.length === 0} style={{ width: '100%', background: C.amber, color: '#1a1d27', border: 'none', borderRadius: 8, padding: '0.55rem', fontSize: 14, fontWeight: 600, cursor: saving ? 'default' : 'pointer', marginTop: 4 }}>
          {saving ? 'Creating…' : 'Create funnel'}
        </button>
      </div>
    </>
  )
}
