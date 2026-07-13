import { useEffect, useState, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Plus, Layers, Pencil, Trash2, ChevronDown, Video } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api, ApiError, Funnel, FunnelAnalysis, Segment } from '../lib/api'
import { useProjects } from '../lib/projects'
import { trackEvent } from '../lib/analytics'
import { reportError } from '../lib/bugbarn'
import { C } from '../lib/theme'
import { ImplementationSnippet } from '../components/funnels/ImplementationSnippet'
import { CreateFunnelModal } from '../components/funnels/CreateFunnelModal'
import { EditFunnelModal } from '../components/funnels/EditFunnelModal'
import { SegmentManager } from '../components/funnels/SegmentManager'
import {
  FUNNEL_TEMPLATES,
  FunnelTemplate,
  PRESET_SEGMENTS,
} from '../components/funnels/constants'

function conversionColor(pct: number) {
  if (pct >= 0.6) return C.success
  if (pct >= 0.3) return C.amber
  return C.error
}

// ─── Templates dropdown ───────────────────────────────────────────────────────

function TemplatesButton({
  onSelect,
}: {
  onSelect: (tpl: FunnelTemplate) => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button
        onClick={() => setOpen((o) => !o)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          background: 'transparent',
          border: `1px solid ${C.border}`,
          borderRadius: 8,
          color: C.muted,
          padding: '0.6rem 1.1rem',
          cursor: 'pointer',
          fontSize: 14,
          fontWeight: 600,
        }}
      >
        Templates
        <ChevronDown size={14} />
      </button>
      {open && (
        <div style={{
          position: 'absolute',
          top: 'calc(100% + 6px)',
          right: 0,
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 10,
          boxShadow: '0 12px 32px rgba(0,0,0,0.4)',
          zIndex: 300,
          minWidth: 260,
          overflow: 'hidden',
        }}>
          {FUNNEL_TEMPLATES.map((tpl) => (
            <button
              key={tpl.name}
              onClick={() => { onSelect(tpl); setOpen(false) }}
              style={{
                display: 'block',
                width: '100%',
                textAlign: 'left',
                background: 'none',
                border: 'none',
                borderBottom: `1px solid ${C.border}`,
                padding: '0.75rem 1rem',
                cursor: 'pointer',
                color: C.text,
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(245,158,11,0.08)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'none')}
            >
              <div style={{ fontWeight: 700, fontSize: 13, marginBottom: 2 }}>{tpl.name}</div>
              <div style={{ fontSize: 11, color: C.muted }}>
                {tpl.steps.join(' → ')}
                <span style={{
                  marginLeft: 6,
                  background: 'rgba(245,158,11,0.15)',
                  color: C.amber,
                  borderRadius: 4,
                  padding: '0.1rem 0.4rem',
                  fontSize: 10,
                  fontWeight: 600,
                }}>
                  {tpl.scope === 'page_view' ? 'page view' : 'session'}
                </span>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Funnel Detail ────────────────────────────────────────────────────────────

function FunnelDetail({ analysis, activeSegment, apiKey, onWatchRecordings }: { analysis: FunnelAnalysis; activeSegment: string; apiKey?: string; onWatchRecordings?: (step: number) => void }) {
  const hasData = analysis.results && analysis.results.length > 0 && analysis.results[0]?.count > 0
  const [showImpl, setShowImpl] = useState(false)

  if (!hasData) {
    return (
      <div>
        <div style={{ padding: '2rem', textAlign: 'center', color: C.muted, maxWidth: '100%', overflow: 'hidden' }}>
          <div style={{ fontSize: 40, marginBottom: 16 }}>📭</div>
          <div style={{ fontWeight: 700, fontSize: 16, color: C.text, marginBottom: 8, wordBreak: 'break-word' }}>
            No data yet for "{analysis.funnel?.name}"
          </div>
          <div style={{ fontSize: 14, lineHeight: 1.6, maxWidth: '100%', wordBreak: 'break-word' }}>
            Start sending events using your tracking snippet.<br />
            This funnel tracks: {analysis.funnel?.steps?.map(s => s.event_name).filter(Boolean).join(' → ') || '(no steps defined)'}
          </div>
        </div>
        <div style={{ marginTop: '1.5rem' }}>
          <button
            onClick={() => setShowImpl((v) => !v)}
            style={{
              background: 'transparent',
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              color: C.muted,
              padding: '0.5rem 1rem',
              cursor: 'pointer',
              fontSize: 13,
              display: 'flex',
              alignItems: 'center',
              gap: 6,
            }}
          >
            <span>{showImpl ? '▲' : '▼'}</span>
            How to implement
          </button>
          {showImpl && analysis.funnel && <ImplementationSnippet funnel={analysis.funnel} apiKey={apiKey} />}
        </div>
      </div>
    )
  }

  const maxCount = Math.max(...(analysis.results?.map((r) => r.count) ?? [1]))
  const segLabel = PRESET_SEGMENTS.find((s) => s.id === activeSegment)?.label ?? activeSegment
  const entryCount = analysis.results?.[0]?.count ?? 0

  return (
    <div>
      <h2 style={{ fontSize: 20, fontWeight: 800, marginBottom: 4 }}>{analysis.funnel.name}</h2>
      <p style={{ color: C.muted, fontSize: 13, marginBottom: '0.75rem' }}>
        {analysis.from?.slice(0, 10)} → {analysis.to?.slice(0, 10)}
      </p>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: '1.25rem', flexWrap: 'wrap' }}>
        <span style={{ color: C.muted, fontSize: 13 }}>
          {entryCount.toLocaleString()} {analysis.funnel?.scope === 'page_view' ? 'page views' : 'sessions'}
        </span>
        {activeSegment !== 'all' && (
          <span style={{
            fontSize: 12,
            fontWeight: 700,
            background: 'rgba(245,158,11,0.15)',
            color: C.amber,
            border: '1px solid rgba(245,158,11,0.35)',
            borderRadius: 99,
            padding: '0.15rem 0.6rem',
          }}>
            {segLabel}
          </span>
        )}
      </div>

      {analysis.results?.map((step, i) => (
        <div key={step.step_order} style={{ marginBottom: '1.25rem' }}>
          {/* Drop-off indicator between steps */}
          {i > 0 && step.drop_off > 0 && (
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
              {(step.drop_off * 100).toFixed(1)}% dropped off
            </div>
          )}

          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{
              width: 32,
              height: 32,
              background: conversionColor(step.conversion),
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
                {step.event_name}
              </div>
              <div style={{ height: 20, background: '#2a2d3a', borderRadius: 4, overflow: 'hidden', position: 'relative' }}>
                <div style={{
                  position: 'absolute',
                  inset: 0,
                  width: `${(step.count / maxCount) * 100}%`,
                  background: conversionColor(step.conversion),
                  borderRadius: 4,
                  display: 'flex',
                  alignItems: 'center',
                  paddingLeft: 8,
                  transition: 'width 0.5s',
                }}>
                  <span style={{ fontSize: 11, fontWeight: 700, color: '#0f1117', whiteSpace: 'nowrap' }}>
                    {(step.conversion * 100).toFixed(1)}%
                  </span>
                </div>
              </div>
            </div>
            <div style={{ textAlign: 'right', minWidth: 70 }}>
              <div style={{ fontSize: 18, fontWeight: 800, color: C.text }}>{step.count.toLocaleString()}</div>
              <div style={{ fontSize: 12, color: C.muted }}>users</div>
            </div>
            {onWatchRecordings && (
              <button
                title="Watch recordings for this step"
                onClick={() => onWatchRecordings(step.step_order)}
                style={{
                  background: 'none', border: `1px solid ${C.border}`, borderRadius: 6,
                  color: C.muted, cursor: 'pointer', padding: '4px 6px',
                  display: 'flex', alignItems: 'center',
                }}
              >
                <Video size={13} />
              </button>
            )}
          </div>
        </div>
      ))}

      <div style={{ marginTop: '1.5rem' }}>
        <button
          onClick={() => setShowImpl((v) => !v)}
          style={{
            background: 'transparent',
            border: `1px solid ${C.border}`,
            borderRadius: 8,
            color: C.muted,
            padding: '0.5rem 1rem',
            cursor: 'pointer',
            fontSize: 13,
            display: 'flex',
            alignItems: 'center',
            gap: 6,
          }}
        >
          <span>{showImpl ? '▲' : '▼'}</span>
          How to implement
        </button>
        {showImpl && analysis.funnel && <ImplementationSnippet funnel={analysis.funnel} apiKey={apiKey} />}
      </div>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function Funnels() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const navigate = useNavigate()
  const [funnels, setFunnels] = useState<Funnel[]>([])
  const [segments, setSegments] = useState<Segment[]>([])
  const [selected, setSelected] = useState<Funnel | null>(null)
  const [analysis, setAnalysis] = useState<FunnelAnalysis | null>(null)
  const [analysisLoading, setAnalysisLoading] = useState(false)
  const [analysisError, setAnalysisError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [showEdit, setShowEdit] = useState(false)
  const [showDetail, setShowDetail] = useState(false)
  const [activeSegment, setActiveSegment] = useState('all')
  // activeSegmentId is set when a stored segment is chosen; activeSegment stays 'all' in that case
  const [activeSegmentId, setActiveSegmentId] = useState<string | undefined>(undefined)
  const [ingestKey, setIngestKey] = useState<string | undefined>(undefined)
  const [deleting, setDeleting] = useState(false)
  const [pendingTemplate, setPendingTemplate] = useState<FunnelTemplate | undefined>(undefined)

  useEffect(() => {
    api.listApiKeys()
      .then((d) => {
        const key = (d.api_keys || []).find((k) => k.scope === 'ingest')
        if (key) setIngestKey(key.id)
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    if (!projectId) return
    setSelected(null)
    setAnalysis(null)
    setAnalysisError(null)
    setShowDetail(false)
    setActiveSegment('all')
    setActiveSegmentId(undefined)
    api.listFunnels(projectId)
      .then((d) => setFunnels(d.funnels || []))
      .catch((e) => { console.error(e); reportError(e, { source: 'Funnels.listFunnels' }) })
    api.listSegments(projectId)
      .then((d) => setSegments(d.segments || []))
      .catch(() => {})
  }, [projectId])

  // Re-fetch analysis whenever selected funnel or active segment changes.
  useEffect(() => {
    if (!selected || !projectId) return
    setAnalysis(null)
    setAnalysisError(null)
    setAnalysisLoading(true)
    api.getFunnelAnalysis(projectId, selected.id, activeSegmentId ? undefined : activeSegment, activeSegmentId)
      .then((a) => {
        setAnalysis(a)
        trackEvent('funnel_analyzed', {
          funnel_name: selected.name,
          scope: selected.scope,
          segment: activeSegmentId ? 'stored' : (activeSegment !== 'all' ? activeSegment : undefined),
        })
      })
      .catch((e) => {
        console.error(e)
        const isNoise = e instanceof ApiError && (e.status === 404 || e.status === 0)
        if (!isNoise) reportError(e, { source: 'Funnels.getFunnelAnalysis' })
        setAnalysisError(String(e))
      })
      .finally(() => setAnalysisLoading(false))
  }, [selected, projectId, activeSegment, activeSegmentId])

  const loadAnalysis = (funnel: Funnel) => {
    if (!projectId) return
    trackEvent('funnel_viewed', { funnel_name: funnel.name })
    setSelected(funnel)
    setShowDetail(true)
    // Analysis will be fetched by the effect above.
  }

  const handleDeleteFunnel = async () => {
    if (!selected || !projectId) return
    if (!window.confirm(`Delete funnel "${selected.name}"? This cannot be undone.`)) return
    setDeleting(true)
    try {
      await api.deleteFunnel(projectId, selected.id)
      trackEvent('funnel_deleted', { funnel_id: selected.id, funnel_name: selected.name })
      setFunnels((prev) => prev.filter((f) => f.id !== selected.id))
      setSelected(null)
      setAnalysis(null)
      setShowDetail(false)
    } catch (e) {
      console.error(e)
      reportError(e, { source: 'Funnels.deleteFunnel' })
      alert('Failed to delete funnel: ' + String(e))
    } finally {
      setDeleting(false)
    }
  }

  // Segment selection — preset vs stored
  const handleSegmentSelect = (presetId: string) => {
    setActiveSegment(presetId)
    setActiveSegmentId(undefined)
  }
  const handleStoredSegmentSelect = (seg: Segment) => {
    setActiveSegment('all') // reset preset
    setActiveSegmentId(seg.id)
  }

  // Determine label for the currently active segment in FunnelDetail
  const activeSegmentLabel: string = activeSegmentId
    ? (segments.find((s) => s.id === activeSegmentId)?.name ?? activeSegmentId)
    : activeSegment

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, padding: '2rem', textAlign: 'center' }}>
          Select a project to view funnels.
        </div>
      </Shell>
    )
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <style>{`
        @media (max-width: 767px) {
          .funnels-layout { grid-template-columns: 1fr !important; }
          .funnels-list.detail-open { display: none !important; }
          .funnels-detail.no-selection { display: none !important; }
          .back-btn { display: block !important; }
        }
        @media (min-width: 768px) {
          .back-btn { display: none !important; }
        }
        .funnels-detail {
          min-width: 0;
          max-width: 100%;
          box-sizing: border-box;
        }
        .funnels-detail > * {
          max-width: 100%;
          box-sizing: border-box;
        }
      `}</style>

      {showCreate && projectId && (
        <CreateFunnelModal
          projectId={projectId}
          initialTemplate={pendingTemplate}
          onClose={() => { setShowCreate(false); setPendingTemplate(undefined) }}
          onCreated={(f) => {
            setFunnels((prev) => [...prev, f])
            setShowCreate(false)
            setPendingTemplate(undefined)
            loadAnalysis(f)
          }}
        />
      )}

      {showEdit && selected && projectId && (
        <EditFunnelModal
          projectId={projectId}
          funnel={selected}
          onClose={() => setShowEdit(false)}
          onUpdated={(updated) => {
            setFunnels((prev) => prev.map((f) => f.id === updated.id ? updated : f))
            setSelected(updated)
            setShowEdit(false)
            setAnalysis(null)
          }}
        />
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem', gap: 8, flexWrap: 'wrap' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Funnels</h1>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <TemplatesButton
            onSelect={(tpl) => {
              setPendingTemplate(tpl)
              setShowCreate(true)
            }}
          />
          <button
            onClick={() => { setPendingTemplate(undefined); setShowCreate(true) }}
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
      </div>

      <div className="funnels-layout" style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '1.5rem' }}>
        {/* Funnel list */}
        <div className={`funnels-list${showDetail ? ' detail-open' : ''}`}>
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
                  {f.scope && f.scope !== 'session' && (
                    <span style={{
                      marginLeft: 6,
                      background: 'rgba(245,158,11,0.12)',
                      color: C.amber,
                      borderRadius: 4,
                      padding: '0.05rem 0.35rem',
                      fontSize: 10,
                      fontWeight: 600,
                    }}>
                      page view
                    </span>
                  )}
                </div>
              </div>
            ))
          )}
        </div>

        {/* Analysis panel */}
        <div className={`funnels-detail${!selected ? ' no-selection' : ''}`} style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '1.5rem',
          minHeight: 300,
          minWidth: 0,
          overflow: 'hidden',
          maxWidth: '100%',
          boxSizing: 'border-box',
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
          {selected && (
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1rem', gap: 8, flexWrap: 'wrap' }}>
              <div style={{ fontWeight: 700, fontSize: 16, color: C.text, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {selected.name}
              </div>
              <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
                <button
                  onClick={() => setShowEdit(true)}
                  title="Edit funnel"
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    background: 'transparent',
                    border: `1px solid ${C.border}`,
                    borderRadius: 7,
                    color: C.muted,
                    padding: '0.35rem 0.7rem',
                    cursor: 'pointer',
                    fontSize: 13,
                  }}
                >
                  <Pencil size={13} /> Edit
                </button>
                <button
                  onClick={handleDeleteFunnel}
                  disabled={deleting}
                  title="Delete funnel"
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    background: 'transparent',
                    border: `1px solid rgba(239,68,68,0.4)`,
                    borderRadius: 7,
                    color: C.error,
                    padding: '0.35rem 0.7rem',
                    cursor: deleting ? 'not-allowed' : 'pointer',
                    fontSize: 13,
                    opacity: deleting ? 0.5 : 1,
                  }}
                >
                  <Trash2 size={13} /> {deleting ? 'Deleting…' : 'Delete'}
                </button>
              </div>
            </div>
          )}
          {selected && (
            <div style={{ marginBottom: '1.25rem' }}>
              {/* Preset segment pills */}
              <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', maxWidth: '100%', boxSizing: 'border-box', marginBottom: segments.length > 0 ? 6 : 0 }}>
                {PRESET_SEGMENTS.map((seg) => {
                  const active = !activeSegmentId && activeSegment === seg.id
                  return (
                    <button
                      key={seg.id}
                      onClick={() => handleSegmentSelect(seg.id)}
                      title={seg.tip}
                      style={{
                        padding: '0.35rem 0.875rem',
                        borderRadius: 99,
                        border: `1px solid ${active ? C.amber : C.border}`,
                        background: active ? 'rgba(245,158,11,0.15)' : 'transparent',
                        color: active ? C.amber : C.muted,
                        cursor: 'pointer',
                        fontSize: 13,
                        fontWeight: active ? 600 : 400,
                        transition: 'all 0.15s',
                      }}
                    >
                      {seg.label}
                    </button>
                  )
                })}
              </div>
              {/* Stored segment pills */}
              {segments.length > 0 && (
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', maxWidth: '100%', boxSizing: 'border-box' }}>
                  {segments.map((seg) => {
                    const active = activeSegmentId === seg.id
                    return (
                      <button
                        key={seg.id}
                        onClick={() => handleStoredSegmentSelect(seg)}
                        title={`Segment: ${seg.name}`}
                        style={{
                          padding: '0.35rem 0.875rem',
                          borderRadius: 99,
                          border: `1px solid ${active ? C.amber : C.border}`,
                          background: active ? 'rgba(245,158,11,0.15)' : 'rgba(245,158,11,0.04)',
                          color: active ? C.amber : C.muted,
                          cursor: 'pointer',
                          fontSize: 13,
                          fontWeight: active ? 600 : 400,
                          transition: 'all 0.15s',
                        }}
                      >
                        {seg.name}
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          )}
          {analysisLoading && (
            <div style={{ color: C.muted, padding: '2rem' }}>Loading analysis…</div>
          )}
          {analysisError && !analysisLoading && (
            <div style={{
              background: 'rgba(239,68,68,0.1)',
              border: `1px solid rgba(239,68,68,0.3)`,
              borderRadius: 8,
              padding: '1rem 1.25rem',
              color: C.error,
              fontSize: 14,
            }}>
              Failed to load analysis: {analysisError}
            </div>
          )}
          {analysis && !analysisLoading && !analysisError && (
            <FunnelDetail
              analysis={analysis}
              activeSegment={activeSegmentLabel}
              apiKey={ingestKey}
              onWatchRecordings={(step) => projectId && analysis?.funnel?.id && navigate(
                `/sessions/${projectId}?funnel=${analysis.funnel.id}&step=${step}`
              )}
            />
          )}
        </div>
      </div>

      {/* Segment manager */}
      {projectId && (
        <SegmentManager
          projectId={projectId}
          segments={segments}
          onSegmentsChange={setSegments}
        />
      )}
    </Shell>
  )
}
