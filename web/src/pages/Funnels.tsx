import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import Shell from '../components/shell/Shell'
import { api, ApiError, Funnel, FunnelAnalysis, Segment } from '../lib/api'
import { useProjects } from '../lib/projects'
import { trackEvent } from '../lib/analytics'
import { reportError } from '../lib/bugbarn'
import { C } from '../lib/theme'
import { AnalysisResult, CreateFunnelButton, FunnelModals, DetailHeader, FunnelList, NoSelection, SegmentPills } from '../components/funnels/FunnelPanels'
import { SegmentManager } from '../components/funnels/SegmentManager'
import { EnvironmentSwitch } from '../components/funnels/EnvironmentSwitch'
import { useFunnelDeepLink } from '../components/funnels/funnelLink'
import { TemplatesButton } from '../components/funnels/TemplatesButton'
import { FunnelTemplate, watchRecordingsPath } from '../components/funnels/constants'

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function Funnels() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects, selectedEnvironment, setSelectedEnvironment } = useProjects()
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

  // Scoped to this route's project: an unfiltered list would happily hand the
  // snippet another project's ingest key.
  useEffect(() => {
    if (!projectId) return
    api.listApiKeys(projectId)
      .then((d) => {
        const key = (d.api_keys || []).find((k) => k.scope === 'ingest')
        setIngestKey(key?.id)
      })
      .catch(() => {})
  }, [projectId])

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
    // A response for a funnel, segment or environment the user already left
    // must not land on the one now shown.
    let cancelled = false
    setAnalysis(null)
    setAnalysisError(null)
    setAnalysisLoading(true)
    api.getFunnelAnalysis(projectId, selected.id, activeSegmentId ? undefined : activeSegment, activeSegmentId, selectedEnvironment)
      .then((a) => {
        if (cancelled) return
        setAnalysis(a)
        trackEvent('funnel_analyzed', {
          funnel_name: selected.name,
          scope: selected.scope,
          segment: activeSegmentId ? 'stored' : (activeSegment !== 'all' ? activeSegment : undefined), environment: selectedEnvironment || undefined,
        })
      })
      .catch((e) => {
        if (cancelled) return
        console.error(e)
        // A 404 here is a funnel deleted in another tab — specific to this
        // view, so it stays local. Network blips and expired sessions are
        // filtered inside reportError.
        const isStale = e instanceof ApiError && e.status === 404
        if (!isStale) reportError(e, { source: 'Funnels.getFunnelAnalysis' })
        setAnalysisError(String(e))
      })
      .finally(() => { if (!cancelled) setAnalysisLoading(false) })
    return () => { cancelled = true }
  }, [selected, projectId, activeSegment, activeSegmentId, selectedEnvironment])

  const loadAnalysis = (funnel: Funnel) => {
    if (!projectId) return
    trackEvent('funnel_viewed', { funnel_name: funnel.name })
    setSelected(funnel)
    setShowDetail(true)
    // Analysis will be fetched by the effect above.
  }
  // ?funnel= opens a funnel on load and follows the selection afterwards.
  useFunnelDeepLink(funnels, selected, loadAnalysis)

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

      <FunnelModals
        projectId={projectId}
        showCreate={showCreate}
        showEdit={showEdit}
        selected={selected}
        pendingTemplate={pendingTemplate}
        onCloseCreate={() => { setShowCreate(false); setPendingTemplate(undefined) }}
        onCloseEdit={() => setShowEdit(false)}
        onCreated={(f) => {
          setFunnels((prev) => [...prev, f])
          setShowCreate(false)
          setPendingTemplate(undefined)
          loadAnalysis(f)
        }}
        onUpdated={(updated) => {
          setFunnels((prev) => prev.map((f) => f.id === updated.id ? updated : f))
          setSelected(updated)
          setShowEdit(false)
          setAnalysis(null)
        }}
      />

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem', gap: 8, flexWrap: 'wrap' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Funnels</h1>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <TemplatesButton
            onSelect={(tpl) => {
              setPendingTemplate(tpl)
              setShowCreate(true)
            }}
          />
          <CreateFunnelButton onClick={() => { setPendingTemplate(undefined); setShowCreate(true) }} />
        </div>
      </div>

      <div className="funnels-layout" style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '1.5rem' }}>
        <FunnelList
          funnels={funnels}
          selectedId={selected?.id}
          detailOpen={showDetail}
          onSelect={loadAnalysis}
          onCreate={() => setShowCreate(true)}
        />

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
          {!selected && <NoSelection />}
          {selected && (
            <DetailHeader
              funnel={selected}
              deleting={deleting}
              onEdit={() => setShowEdit(true)}
              onDelete={handleDeleteFunnel}
            />
          )}
          {selected && (
            <SegmentPills
              segments={segments}
              activeSegment={activeSegment}
              activeSegmentId={activeSegmentId}
              onPreset={handleSegmentSelect}
              onStored={handleStoredSegmentSelect}
            />
          )}
          <EnvironmentSwitch projectId={projectId} value={selectedEnvironment} onChange={setSelectedEnvironment} />
          <AnalysisResult
            projectId={projectId}
            analysis={analysis}
            loading={analysisLoading}
            error={analysisError}
            segmentLabel={activeSegmentLabel}
            activeSegment={activeSegment}
            activeSegmentId={activeSegmentId}
            environment={selectedEnvironment}
            apiKey={ingestKey}
            onWatchRecordings={(step) => analysis?.funnel?.id && navigate(watchRecordingsPath(projectId, analysis.funnel.id, step, selectedEnvironment))}
          />
        </div>
      </div>

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
