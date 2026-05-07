import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Flag, Plus, X, Pause, Play, Trash2 } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api } from '../lib/api'
import type { FeatureFlag, FlagAnalysis } from '../lib/api'
import { useProjects } from '../lib/projects'
import { reportError } from '../lib/bugbarn'
import { Skeleton } from '../components/ui/Skeleton'

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

function statusBadge(status: string) {
  const styles: Record<string, { bg: string; color: string }> = {
    active: { bg: 'rgba(16,185,129,0.1)', color: '#10b981' },
    paused: { bg: 'rgba(245,158,11,0.1)', color: '#f59e0b' },
  }
  const s = styles[status] ?? styles.active
  return (
    <span style={{
      fontSize: 11, fontWeight: 700,
      background: s.bg, color: s.color,
      border: `1px solid ${s.color}33`,
      borderRadius: 99, padding: '0.15rem 0.6rem',
      textTransform: 'uppercase', letterSpacing: '0.05em',
    }}>
      {status}
    </span>
  )
}

function StatCard({ label, sample, conversions, rate, winner }: {
  label: string; sample: number; conversions: number; rate: number; winner: boolean
}) {
  return (
    <div style={{
      flex: 1, background: C.bg,
      border: `1px solid ${winner ? 'rgba(16,185,129,0.4)' : C.border}`,
      borderRadius: 12, padding: '1.25rem', position: 'relative',
    }}>
      {winner && (
        <div style={{
          position: 'absolute', top: 10, right: 10,
          fontSize: 11, fontWeight: 700,
          background: 'rgba(16,185,129,0.15)', color: C.success,
          border: '1px solid rgba(16,185,129,0.3)',
          borderRadius: 99, padding: '0.15rem 0.6rem',
        }}>
          Winner
        </div>
      )}
      <div style={{ fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: '1rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        {label}
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.75rem' }}>
        <div>
          <div style={{ fontSize: 22, fontWeight: 800, color: C.text }}>{sample.toLocaleString()}</div>
          <div style={{ fontSize: 12, color: C.muted }}>evaluations</div>
        </div>
        <div>
          <div style={{ fontSize: 22, fontWeight: 800, color: C.text }}>{conversions.toLocaleString()}</div>
          <div style={{ fontSize: 12, color: C.muted }}>conversions</div>
        </div>
        <div style={{ gridColumn: '1 / -1' }}>
          <div style={{ fontSize: 28, fontWeight: 800, color: winner ? C.success : C.amber }}>
            {(rate * 100).toFixed(2)}%
          </div>
          <div style={{ fontSize: 12, color: C.muted }}>conversion rate</div>
        </div>
      </div>
    </div>
  )
}

function FlagDetail({ flag, projectId, onUpdated, onDeleted }: {
  flag: FeatureFlag; projectId: string
  onUpdated: (f: FeatureFlag) => void
  onDeleted: () => void
}) {
  const [analysis, setAnalysis] = useState<FlagAnalysis | null>(null)
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState(false)
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    setLoading(true)
    api.getFlagAnalysis(projectId, flag.id)
      .then(setAnalysis)
      .catch((e) => { console.error(e); reportError(e, { source: 'Flags' }) })
      .finally(() => setLoading(false))
  }, [projectId, flag.id])

  const toggleStatus = async () => {
    setToggling(true)
    try {
      const updated = await api.updateFlag(projectId, flag.id, {
        ...flag,
        status: flag.status === 'active' ? 'paused' : 'active',
      })
      onUpdated(updated)
    } catch (e) {
      reportError(e instanceof Error ? e : new Error(String(e)), { source: 'Flags.toggle' })
    } finally {
      setToggling(false)
    }
  }

  const handleDelete = async () => {
    if (!confirm('Delete this flag? This cannot be undone.')) return
    setDeleting(true)
    try {
      await api.deleteFlag(projectId, flag.id)
      onDeleted()
    } catch (e) {
      reportError(e instanceof Error ? e : new Error(String(e)), { source: 'Flags.delete' })
    } finally {
      setDeleting(false)
    }
  }

  const variants = (() => { try { return JSON.parse(flag.variants) as Record<string, unknown> } catch { return {} } })()
  const split = (() => { try { return JSON.parse(flag.split) as Record<string, number> } catch { return {} } })()

  const results = analysis?.results ?? []
  const bestRate = Math.max(...results.map(r => r.rate), 0)

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 16 }}>
        <div>
          <h2 style={{ fontSize: 20, fontWeight: 800, margin: '0 0 4px' }}>{flag.name}</h2>
          <code style={{ fontSize: 13, color: C.amber, background: 'rgba(245,158,11,0.1)', padding: '2px 8px', borderRadius: 4 }}>
            {flag.flag_key}
          </code>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {statusBadge(flag.status)}
          <button onClick={toggleStatus} disabled={toggling} title={flag.status === 'active' ? 'Pause' : 'Resume'}
            style={{ background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '4px 8px', display: 'flex', alignItems: 'center' }}>
            {flag.status === 'active' ? <Pause size={14} /> : <Play size={14} />}
          </button>
          <button onClick={handleDelete} disabled={deleting} title="Delete"
            style={{ background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 6, color: C.error, cursor: 'pointer', padding: '4px 8px', display: 'flex', alignItems: 'center' }}>
            <Trash2 size={14} />
          </button>
        </div>
      </div>

      <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', marginBottom: 16, fontSize: 13, color: C.muted }}>
        <div>Type: <span style={{ color: C.text }}>{flag.flag_type}</span></div>
        <div>Default: <span style={{ color: C.text }}>{flag.default_variant}</span></div>
        {flag.conversion_event && <div>Conversion: <span style={{ color: C.text }}>{flag.conversion_event}</span></div>}
      </div>

      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 24 }}>
        {Object.entries(split).map(([variant, pct]) => (
          <div key={variant} style={{
            background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8,
            padding: '8px 12px', fontSize: 13,
          }}>
            <span style={{ color: C.text, fontWeight: 600 }}>{variant}</span>
            <span style={{ color: C.muted }}> = {JSON.stringify(variants[variant])}</span>
            <span style={{ color: C.amber, marginLeft: 8 }}>{pct}%</span>
          </div>
        ))}
      </div>

      {loading ? (
        <div style={{ display: 'flex', gap: '1rem' }}>
          <Skeleton height={180} />
          <Skeleton height={180} />
        </div>
      ) : results.length === 0 ? (
        <div style={{ color: C.muted, fontSize: 14, textAlign: 'center', padding: '2rem' }}>
          No evaluations yet. Integrate the flag in your application to start collecting data.
        </div>
      ) : (
        <>
          <div style={{ display: 'flex', gap: '1rem', marginBottom: '1.5rem', flexWrap: 'wrap' }}>
            {results.map((r) => (
              <StatCard
                key={r.variant}
                label={r.variant}
                sample={r.sample}
                conversions={r.conversions}
                rate={r.rate}
                winner={analysis?.significant === true && r.rate === bestRate && results.length > 1}
              />
            ))}
          </div>

          {analysis && (
            <div style={{
              background: analysis.significant ? 'rgba(16,185,129,0.08)' : 'rgba(245,158,11,0.08)',
              border: `1px solid ${analysis.significant ? 'rgba(16,185,129,0.3)' : 'rgba(245,158,11,0.3)'}`,
              borderRadius: 10, padding: '1rem 1.25rem',
              fontSize: 14, color: analysis.significant ? C.success : C.amber, fontWeight: 600,
            }}>
              {analysis.significant
                ? `Significant (95% CI) — z = ${(analysis.z_score ?? 0).toFixed(2)}`
                : `Not significant yet${analysis.z_score ? ` — z = ${analysis.z_score.toFixed(2)}` : ''}`}
            </div>
          )}
        </>
      )}
    </div>
  )
}

function CreateFlagModal({ projectId, onClose, onCreated }: {
  projectId: string; onClose: () => void; onCreated: (f: FeatureFlag) => void
}) {
  const [flagKey, setFlagKey] = useState('')
  const [name, setName] = useState('')
  const [flagType, setFlagType] = useState<'boolean' | 'string'>('boolean')
  const [splitA, setSplitA] = useState(50)
  const [conversionEvent, setConversionEvent] = useState('')
  const [eventNames, setEventNames] = useState<string[]>([])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // String variants
  const [variantAKey, setVariantAKey] = useState('control')
  const [variantAVal, setVariantAVal] = useState('control')
  const [variantBKey, setVariantBKey] = useState('treatment')
  const [variantBVal, setVariantBVal] = useState('treatment')

  useEffect(() => {
    api.getEventNames(projectId).then((d) => setEventNames(d.event_names || [])).catch(() => {})
  }, [projectId])

  const autoKey = (n: string) => n.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')

  const handleCreate = async () => {
    if (!flagKey.trim() || !name.trim()) { setError('Key and name are required'); return }
    setSaving(true)
    setError(null)
    try {
      let variants: Record<string, unknown>
      let split: Record<string, number>
      if (flagType === 'boolean') {
        variants = { on: true, off: false }
        split = { on: splitA, off: 100 - splitA }
      } else {
        variants = { [variantAKey]: variantAVal, [variantBKey]: variantBVal }
        split = { [variantAKey]: splitA, [variantBKey]: 100 - splitA }
      }

      const f = await api.createFlag(projectId, {
        flag_key: flagKey,
        name,
        flag_type: flagType,
        variants: JSON.stringify(variants),
        default_variant: flagType === 'boolean' ? 'off' : variantAKey,
        split: JSON.stringify(split),
        conversion_event: conversionEvent,
      })
      onCreated(f)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  const inputStyle = {
    background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8,
    padding: '0.6rem 0.875rem', color: C.text, fontSize: 14,
    width: '100%', boxSizing: 'border-box' as const, outline: 'none',
  }

  return (
    <>
      <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', zIndex: 1000 }} />
      <div style={{
        position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%, -50%)',
        background: C.surface, border: `1px solid ${C.border}`, borderRadius: 16,
        width: '90%', maxWidth: 520, zIndex: 1001, maxHeight: '90vh', overflowY: 'auto',
      }}>
        <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}`, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>Create Flag</h2>
          <button onClick={onClose} style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer' }}><X size={18} /></button>
        </div>

        <div style={{ padding: '1.5rem' }}>
          {error && (
            <div style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: 8, padding: '0.6rem', color: C.error, fontSize: 13, marginBottom: '1rem' }}>
              {error}
            </div>
          )}

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Name</label>
          <input value={name} onChange={(e) => { setName(e.target.value); if (!flagKey || flagKey === autoKey(name)) setFlagKey(autoKey(e.target.value)) }}
            placeholder="e.g. Checkout Redesign" style={{ ...inputStyle, marginBottom: 16 }} />

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Key</label>
          <input value={flagKey} onChange={(e) => setFlagKey(e.target.value)}
            placeholder="e.g. checkout-redesign" style={{ ...inputStyle, marginBottom: 16, fontFamily: 'monospace' }} />

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Type</label>
          <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
            {(['boolean', 'string'] as const).map((t) => (
              <button key={t} onClick={() => setFlagType(t)} style={{
                flex: 1, padding: '0.5rem', border: `1px solid ${flagType === t ? C.amber : C.border}`,
                borderRadius: 8, background: flagType === t ? 'rgba(245,158,11,0.1)' : 'transparent',
                color: flagType === t ? C.amber : C.muted, fontSize: 13, fontWeight: 600, cursor: 'pointer',
              }}>
                {t}
              </button>
            ))}
          </div>

          {flagType === 'string' && (
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Variants</label>
              <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                <input value={variantAKey} onChange={(e) => setVariantAKey(e.target.value)} placeholder="key" style={{ ...inputStyle, flex: 1, fontFamily: 'monospace' }} />
                <input value={variantAVal} onChange={(e) => setVariantAVal(e.target.value)} placeholder="value" style={{ ...inputStyle, flex: 1 }} />
              </div>
              <div style={{ display: 'flex', gap: 8 }}>
                <input value={variantBKey} onChange={(e) => setVariantBKey(e.target.value)} placeholder="key" style={{ ...inputStyle, flex: 1, fontFamily: 'monospace' }} />
                <input value={variantBVal} onChange={(e) => setVariantBVal(e.target.value)} placeholder="value" style={{ ...inputStyle, flex: 1 }} />
              </div>
            </div>
          )}

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>
            Split: {flagType === 'boolean' ? `on ${splitA}% / off ${100 - splitA}%` : `${variantAKey} ${splitA}% / ${variantBKey} ${100 - splitA}%`}
          </label>
          <input type="range" min={0} max={100} value={splitA}
            onChange={(e) => setSplitA(Number(e.target.value))}
            style={{ width: '100%', marginBottom: 16, accentColor: C.amber }} />

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Conversion event</label>
          <select value={conversionEvent} onChange={(e) => setConversionEvent(e.target.value)}
            style={{ ...inputStyle, marginBottom: 8 }}>
            <option value="">Select an event (optional)...</option>
            {eventNames.map((n) => <option key={n} value={n}>{n}</option>)}
          </select>
        </div>

        <div style={{ padding: '1rem 1.5rem', borderTop: `1px solid ${C.border}`, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button onClick={onClose} style={{ padding: '0.5rem 1rem', background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 8, color: C.text, fontSize: 13, fontWeight: 600, cursor: 'pointer' }}>
            Cancel
          </button>
          <button onClick={handleCreate} disabled={saving || !flagKey || !name}
            style={{
              padding: '0.5rem 1rem', border: 'none', borderRadius: 8, fontSize: 13, fontWeight: 700, cursor: saving ? 'default' : 'pointer',
              background: saving || !flagKey || !name ? '#4a4d5a' : C.amber,
              color: saving || !flagKey || !name ? C.muted : '#000',
            }}>
            {saving ? 'Creating...' : 'Create Flag'}
          </button>
        </div>
      </div>
    </>
  )
}

export default function Flags() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const [flags, setFlags] = useState<FeatureFlag[]>([])
  const [selected, setSelected] = useState<FeatureFlag | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [showDetail, setShowDetail] = useState(false)

  useEffect(() => {
    if (!projectId) return
    setSelected(null)
    setShowDetail(false)
    api.listFlags(projectId)
      .then((d) => setFlags(d.flags || []))
      .catch((e) => { console.error(e); reportError(e, { source: 'Flags' }) })
  }, [projectId])

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, padding: '2rem', textAlign: 'center' }}>Select a project to manage flags.</div>
      </Shell>
    )
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <style>{`
        @media (max-width: 767px) {
          .flags-layout { grid-template-columns: 1fr !important; }
          .flags-list.detail-open { display: none !important; }
          .flags-detail.no-selection { display: none !important; }
          .back-btn { display: block !important; }
        }
        @media (min-width: 768px) {
          .back-btn { display: none !important; }
        }
      `}</style>

      {showCreate && (
        <CreateFlagModal projectId={projectId} onClose={() => setShowCreate(false)}
          onCreated={(f) => { setFlags((prev) => [f, ...prev]); setShowCreate(false); setSelected(f); setShowDetail(true) }} />
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Flags</h1>
        <button onClick={() => setShowCreate(true)} style={{
          display: 'flex', alignItems: 'center', gap: 6,
          background: C.amber, border: 'none', borderRadius: 8,
          color: '#0f1117', padding: '0.6rem 1.1rem', cursor: 'pointer', fontSize: 14, fontWeight: 700,
        }}>
          <Plus size={16} /> Create flag
        </button>
      </div>

      <div className="flags-layout" style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '1.5rem' }}>
        <div className={`flags-list${showDetail ? ' detail-open' : ''}`}>
          {flags.length === 0 ? (
            <div style={{
              background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12,
              padding: '2rem', textAlign: 'center', color: C.muted,
            }}>
              <Flag size={32} style={{ opacity: 0.3, marginBottom: 8 }} />
              <div style={{ fontSize: 14 }}>No flags yet</div>
              <button onClick={() => setShowCreate(true)} style={{
                marginTop: 12, background: 'transparent', border: `1px solid ${C.border}`,
                borderRadius: 7, color: C.amber, padding: '0.4rem 0.8rem', cursor: 'pointer', fontSize: 13, fontWeight: 600,
              }}>
                Create one
              </button>
            </div>
          ) : (
            flags.map((f) => (
              <div key={f.id} onClick={() => { setSelected(f); setShowDetail(true) }} style={{
                padding: '0.875rem 1rem', marginBottom: '0.5rem', cursor: 'pointer',
                background: selected?.id === f.id ? 'rgba(245,158,11,0.1)' : C.surface,
                border: `1px solid ${selected?.id === f.id ? 'rgba(245,158,11,0.4)' : C.border}`,
                borderRadius: 10, transition: 'all 0.15s',
              }}>
                <div style={{ fontWeight: 700, fontSize: 14, color: C.text, marginBottom: 4 }}>{f.name}</div>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <code style={{ fontSize: 12, color: C.muted }}>{f.flag_key}</code>
                  {statusBadge(f.status)}
                </div>
              </div>
            ))
          )}
        </div>

        <div className={`flags-detail${!selected ? ' no-selection' : ''}`} style={{
          background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12,
          padding: '1.5rem', minHeight: 300,
        }}>
          <button className="back-btn" onClick={() => setShowDetail(false)} style={{
            display: 'none', background: 'none', border: 'none', color: C.amber,
            cursor: 'pointer', fontSize: 14, fontWeight: 600, padding: '0 0 1rem 0',
          }}>
            ← Back
          </button>
          {!selected ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: 300, color: C.muted, gap: 8 }}>
              <Flag size={40} opacity={0.3} />
              <div>Select a flag to view results</div>
            </div>
          ) : (
            <FlagDetail flag={selected} projectId={projectId}
              onUpdated={(f) => { setSelected(f); setFlags((prev) => prev.map((x) => x.id === f.id ? f : x)) }}
              onDeleted={() => { setFlags((prev) => prev.filter((x) => x.id !== selected.id)); setSelected(null); setShowDetail(false) }}
            />
          )}
        </div>
      </div>
    </Shell>
  )
}
