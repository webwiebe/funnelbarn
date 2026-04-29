import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { FlaskConical, Plus, X } from 'lucide-react'
import Shell from '../components/Shell'
import { api, ABTest, ABTestAnalysis } from '../lib/api'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  success: '#10b981',
  error: '#ef4444',
  blue: '#60a5fa',
}

function statusBadge(status: string) {
  const styles: Record<string, { bg: string; color: string }> = {
    running: { bg: 'rgba(16,185,129,0.1)', color: '#10b981' },
    paused: { bg: 'rgba(245,158,11,0.1)', color: '#f59e0b' },
    concluded: { bg: 'rgba(100,116,139,0.1)', color: '#94a3b8' },
  }
  const s = styles[status] ?? styles.running
  return (
    <span style={{
      fontSize: 11,
      fontWeight: 700,
      background: s.bg,
      color: s.color,
      border: `1px solid ${s.color}33`,
      borderRadius: 99,
      padding: '0.15rem 0.6rem',
      textTransform: 'uppercase',
      letterSpacing: '0.05em',
    }}>
      {status}
    </span>
  )
}

function CreateTestModal({
  projectId,
  onClose,
  onCreated,
}: {
  projectId: string
  onClose: () => void
  onCreated: (t: ABTest) => void
}) {
  const [name, setName] = useState('')
  const [conversionEvent, setConversionEvent] = useState('')
  const [controlProp, setControlProp] = useState('')
  const [controlVal, setControlVal] = useState('')
  const [variantProp, setVariantProp] = useState('')
  const [variantVal, setVariantVal] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCreate = async () => {
    if (!name.trim()) { setError('Name is required'); return }
    if (!conversionEvent.trim()) { setError('Conversion event is required'); return }
    setSaving(true)
    setError(null)
    try {
      const t = await api.createABTest(projectId, {
        name,
        conversion_event: conversionEvent,
        control_filter: { property: controlProp, value: controlVal },
        variant_filter: { property: variantProp, value: variantVal },
      })
      onCreated(t)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  const inputStyle = {
    background: C.bg,
    border: `1px solid ${C.border}`,
    borderRadius: 8,
    padding: '0.6rem 0.875rem',
    color: C.text,
    fontSize: 14,
    outline: 'none',
    width: '100%',
    boxSizing: 'border-box' as const,
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
        maxWidth: 540,
        boxShadow: '0 25px 60px rgba(0,0,0,0.5)',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>Create A/B test</h2>
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
            fontSize: 13,
            marginBottom: '1rem',
          }}>
            {error}
          </div>
        )}

        <div style={{ marginBottom: '1rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>Test name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Checkout button color"
            style={inputStyle}
            onFocus={(e) => (e.target.style.borderColor = C.amber)}
            onBlur={(e) => (e.target.style.borderColor = C.border)}
          />
        </div>

        <div style={{ marginBottom: '1.25rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>Conversion event</label>
          <input
            value={conversionEvent}
            onChange={(e) => setConversionEvent(e.target.value)}
            placeholder="e.g. purchase"
            style={inputStyle}
            onFocus={(e) => (e.target.style.borderColor = C.amber)}
            onBlur={(e) => (e.target.style.borderColor = C.border)}
          />
        </div>

        {/* Control filter */}
        <div style={{ marginBottom: '1rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 8 }}>
            Control filter <span style={{ color: C.border }}>(property = value)</span>
          </label>
          <div style={{ display: 'flex', gap: 8 }}>
            <input
              value={controlProp}
              onChange={(e) => setControlProp(e.target.value)}
              placeholder="property (e.g. variant)"
              style={{ ...inputStyle, flex: 1 }}
              onFocus={(e) => (e.target.style.borderColor = C.amber)}
              onBlur={(e) => (e.target.style.borderColor = C.border)}
            />
            <input
              value={controlVal}
              onChange={(e) => setControlVal(e.target.value)}
              placeholder="value (e.g. control)"
              style={{ ...inputStyle, flex: 1 }}
              onFocus={(e) => (e.target.style.borderColor = C.amber)}
              onBlur={(e) => (e.target.style.borderColor = C.border)}
            />
          </div>
        </div>

        {/* Variant filter */}
        <div style={{ marginBottom: '1.5rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 8 }}>
            Variant filter <span style={{ color: C.border }}>(property = value)</span>
          </label>
          <div style={{ display: 'flex', gap: 8 }}>
            <input
              value={variantProp}
              onChange={(e) => setVariantProp(e.target.value)}
              placeholder="property (e.g. variant)"
              style={{ ...inputStyle, flex: 1 }}
              onFocus={(e) => (e.target.style.borderColor = C.amber)}
              onBlur={(e) => (e.target.style.borderColor = C.border)}
            />
            <input
              value={variantVal}
              onChange={(e) => setVariantVal(e.target.value)}
              placeholder="value (e.g. treatment)"
              style={{ ...inputStyle, flex: 1 }}
              onFocus={(e) => (e.target.style.borderColor = C.amber)}
              onBlur={(e) => (e.target.style.borderColor = C.border)}
            />
          </div>
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
            {saving ? 'Creating…' : 'Create test'}
          </button>
        </div>
      </div>
    </div>
  )
}

function StatCard({
  label,
  sampleSize,
  conversions,
  rate,
  winner,
}: {
  label: string
  sampleSize: number
  conversions: number
  rate: number
  winner: boolean
}) {
  return (
    <div style={{
      flex: 1,
      background: C.bg,
      border: `1px solid ${winner ? 'rgba(16,185,129,0.4)' : C.border}`,
      borderRadius: 12,
      padding: '1.25rem',
      position: 'relative',
    }}>
      {winner && (
        <div style={{
          position: 'absolute',
          top: 10,
          right: 10,
          fontSize: 11,
          fontWeight: 700,
          background: 'rgba(16,185,129,0.15)',
          color: C.success,
          border: `1px solid rgba(16,185,129,0.3)`,
          borderRadius: 99,
          padding: '0.15rem 0.6rem',
        }}>
          Winner 🏆
        </div>
      )}
      <div style={{ fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: '1rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        {label}
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.75rem' }}>
        <div>
          <div style={{ fontSize: 22, fontWeight: 800, color: C.text }}>{sampleSize.toLocaleString()}</div>
          <div style={{ fontSize: 12, color: C.muted }}>sample size</div>
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

function TestDetail({ test, projectId }: { test: ABTest; projectId: string }) {
  const [analysis, setAnalysis] = useState<ABTestAnalysis | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.getABTestAnalysis(projectId, test.id)
      .then(setAnalysis)
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [projectId, test.id])

  if (loading) return <div style={{ color: C.muted, fontSize: 14 }}>Loading analysis…</div>
  if (!analysis) return <div style={{ color: C.error, fontSize: 14 }}>Failed to load analysis.</div>

  const controlRate = analysis.control_sample > 0 ? analysis.control_conversions / analysis.control_sample : 0
  const variantRate = analysis.variant_sample > 0 ? analysis.variant_conversions / analysis.variant_sample : 0
  const controlWins = analysis.significant && controlRate > variantRate
  const variantWins = analysis.significant && variantRate > controlRate

  return (
    <div>
      <h2 style={{ fontSize: 20, fontWeight: 800, marginBottom: 4 }}>{test.name}</h2>
      <p style={{ color: C.muted, fontSize: 13, marginBottom: '1.5rem' }}>
        Conversion event: <span style={{ color: C.text, fontWeight: 600 }}>{test.conversion_event}</span>
      </p>

      <div style={{ display: 'flex', gap: '1rem', marginBottom: '1.5rem' }}>
        <StatCard
          label="Control"
          sampleSize={analysis.control_sample}
          conversions={analysis.control_conversions}
          rate={controlRate}
          winner={controlWins}
        />
        <StatCard
          label="Variant"
          sampleSize={analysis.variant_sample}
          conversions={analysis.variant_conversions}
          rate={variantRate}
          winner={variantWins}
        />
      </div>

      <div style={{
        background: analysis.significant ? 'rgba(16,185,129,0.08)' : 'rgba(245,158,11,0.08)',
        border: `1px solid ${analysis.significant ? 'rgba(16,185,129,0.3)' : 'rgba(245,158,11,0.3)'}`,
        borderRadius: 10,
        padding: '1rem 1.25rem',
        fontSize: 14,
        color: analysis.significant ? C.success : C.amber,
        fontWeight: 600,
      }}>
        {analysis.significant
          ? `✓ Significant result (${((analysis.confidence ?? 0) * 100).toFixed(1)}% confidence)`
          : '⏳ Not yet significant — need more data'}
      </div>
    </div>
  )
}

export default function ABTests() {
  const { projectId } = useParams<{ projectId?: string }>()
  const [tests, setTests] = useState<ABTest[]>([])
  const [selected, setSelected] = useState<ABTest | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [showDetail, setShowDetail] = useState(false)

  useEffect(() => {
    if (!projectId) return
    api.getABTests(projectId)
      .then((d) => setTests(d.tests || []))
      .catch(console.error)
  }, [projectId])

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, padding: '2rem', textAlign: 'center' }}>
          Select a project to view A/B tests.
        </div>
      </Shell>
    )
  }

  return (
    <Shell projectId={projectId}>
      <style>{`
        @media (max-width: 767px) {
          .abtests-layout { grid-template-columns: 1fr !important; }
          .abtests-list.detail-open { display: none !important; }
          .abtests-detail.no-selection { display: none !important; }
          .back-btn { display: block !important; }
        }
        @media (min-width: 768px) {
          .back-btn { display: none !important; }
        }
      `}</style>

      {showCreate && (
        <CreateTestModal
          projectId={projectId}
          onClose={() => setShowCreate(false)}
          onCreated={(t) => {
            setTests((prev) => [...prev, t])
            setShowCreate(false)
            setSelected(t)
            setShowDetail(true)
          }}
        />
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>A/B Tests</h1>
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
          Create test
        </button>
      </div>

      <div className="abtests-layout" style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '1.5rem' }}>
        {/* Test list */}
        <div className={`abtests-list${showDetail ? ' detail-open' : ''}`}>
          {tests.length === 0 ? (
            <div style={{
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 12,
              padding: '2rem',
              textAlign: 'center',
              color: C.muted,
            }}>
              <FlaskConical size={32} style={{ opacity: 0.3, marginBottom: 8 }} />
              <div style={{ fontSize: 14 }}>No tests yet</div>
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
            tests.map((t) => (
              <div
                key={t.id}
                onClick={() => { setSelected(t); setShowDetail(true) }}
                style={{
                  padding: '0.875rem 1rem',
                  marginBottom: '0.5rem',
                  cursor: 'pointer',
                  background: selected?.id === t.id ? 'rgba(245,158,11,0.1)' : C.surface,
                  border: `1px solid ${selected?.id === t.id ? 'rgba(245,158,11,0.4)' : C.border}`,
                  borderRadius: 10,
                  transition: 'all 0.15s',
                }}
              >
                <div style={{ fontWeight: 700, fontSize: 14, color: C.text, marginBottom: 4 }}>{t.name}</div>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div style={{ fontSize: 12, color: C.muted }}>{t.conversion_event}</div>
                  {statusBadge(t.status ?? 'running')}
                </div>
                <div style={{ fontSize: 11, color: C.muted, marginTop: 4 }}>
                  {new Date(t.created_at).toLocaleDateString()}
                </div>
              </div>
            ))
          )}
        </div>

        {/* Detail panel */}
        <div className={`abtests-detail${!selected ? ' no-selection' : ''}`} style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '1.5rem',
          minHeight: 300,
        }}>
          <button
            className="back-btn"
            onClick={() => setShowDetail(false)}
            style={{
              display: 'none',
              background: 'none',
              border: 'none',
              color: C.amber,
              cursor: 'pointer',
              fontSize: 14,
              fontWeight: 600,
              padding: '0 0 1rem 0',
            }}
          >
            ← Back
          </button>
          {!selected ? (
            <div style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: 300,
              color: C.muted,
              gap: 8,
            }}>
              <FlaskConical size={40} opacity={0.3} />
              <div>Select a test to view results</div>
            </div>
          ) : (
            <TestDetail test={selected} projectId={projectId} />
          )}
        </div>
      </div>
    </Shell>
  )
}
