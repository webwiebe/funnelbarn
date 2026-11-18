import { Layers, Pencil, Plus, Trash2 } from 'lucide-react'
import { Funnel, FunnelAnalysis, Segment } from '../../lib/api'
import { C } from '../../lib/theme'
import { CreateFunnelModal } from './CreateFunnelModal'
import { EditFunnelModal } from './EditFunnelModal'
import { BreakdownPanel } from './BreakdownPanel'
import { FunnelDetail } from './FunnelDetail'
import { FunnelTemplate, PRESET_SEGMENTS } from './constants'

export function CreateFunnelButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      onClick={onClick}
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
  )
}

function EmptyList({ onCreate }: { onCreate: () => void }) {
  return (
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
        onClick={onCreate}
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
  )
}

const badgeStyle = {
  marginLeft: 6,
  borderRadius: 4,
  padding: '0.05rem 0.35rem',
  fontSize: 10,
  fontWeight: 600,
}

function FunnelBadges({ funnel }: { funnel: Funnel }) {
  const unmatched = funnel.unmatched_steps ?? []
  return (
    <>
      {unmatched.length > 0 && (
        <span
          title={`Never received: ${unmatched.join(', ')}`}
          style={{ ...badgeStyle, background: 'rgba(239,68,68,0.12)', color: C.error }}
        >
          not configured
        </span>
      )}
      {funnel.scope && funnel.scope !== 'session' && (
        <span style={{ ...badgeStyle, background: 'rgba(245,158,11,0.12)', color: C.amber }}>
          {funnel.scope === 'user' ? 'user' : 'page view'}
        </span>
      )}
    </>
  )
}

function FunnelListItem({ funnel, active, onSelect }: { funnel: Funnel; active: boolean; onSelect: (f: Funnel) => void }) {
  return (
    <div
      onClick={() => onSelect(funnel)}
      style={{
        padding: '0.875rem 1rem',
        marginBottom: '0.5rem',
        cursor: 'pointer',
        background: active ? 'rgba(245,158,11,0.1)' : C.surface,
        border: `1px solid ${active ? 'rgba(245,158,11,0.4)' : C.border}`,
        borderRadius: 10,
        transition: 'all 0.15s',
      }}
    >
      <div style={{ fontWeight: 700, fontSize: 14, color: C.text }}>{funnel.name}</div>
      <div style={{ fontSize: 12, color: C.muted, marginTop: 3 }}>
        {funnel.steps?.length ?? 0} steps
        <FunnelBadges funnel={funnel} />
      </div>
    </div>
  )
}

export function FunnelList({ funnels, selectedId, detailOpen, onSelect, onCreate }: {
  funnels: Funnel[]
  selectedId?: string
  detailOpen: boolean
  onSelect: (f: Funnel) => void
  onCreate: () => void
}) {
  return (
    <div className={`funnels-list${detailOpen ? ' detail-open' : ''}`}>
      {funnels.length === 0 ? (
        <EmptyList onCreate={onCreate} />
      ) : (
        funnels.map((f) => (
          <FunnelListItem key={f.id} funnel={f} active={selectedId === f.id} onSelect={onSelect} />
        ))
      )}
    </div>
  )
}

export function NoSelection() {
  return (
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
  )
}

export function DetailHeader({ funnel, deleting, onEdit, onDelete }: {
  funnel: Funnel
  deleting: boolean
  onEdit: () => void
  onDelete: () => void
}) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1rem', gap: 8, flexWrap: 'wrap' }}>
      <div style={{ fontWeight: 700, fontSize: 16, color: C.text, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {funnel.name}
      </div>
      <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
        <button
          onClick={onEdit}
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
          onClick={onDelete}
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
  )
}

function pillStyle(active: boolean, idleBackground: string) {
  return {
    padding: '0.35rem 0.875rem',
    borderRadius: 99,
    border: `1px solid ${active ? C.amber : C.border}`,
    background: active ? 'rgba(245,158,11,0.15)' : idleBackground,
    color: active ? C.amber : C.muted,
    cursor: 'pointer',
    fontSize: 13,
    fontWeight: active ? 600 : 400,
    transition: 'all 0.15s',
  }
}

const pillRowStyle = { display: 'flex', gap: 6, flexWrap: 'wrap', maxWidth: '100%', boxSizing: 'border-box' } as const

export function SegmentPills({ segments, activeSegment, activeSegmentId, onPreset, onStored }: {
  segments: Segment[]
  activeSegment: string
  activeSegmentId?: string
  onPreset: (id: string) => void
  onStored: (seg: Segment) => void
}) {
  return (
    <div style={{ marginBottom: '1.25rem' }}>
      <div style={{ ...pillRowStyle, marginBottom: segments.length > 0 ? 6 : 0 }}>
        {PRESET_SEGMENTS.map((seg) => (
          <button
            key={seg.id}
            onClick={() => onPreset(seg.id)}
            title={seg.tip}
            style={pillStyle(!activeSegmentId && activeSegment === seg.id, 'transparent')}
          >
            {seg.label}
          </button>
        ))}
      </div>
      {segments.length > 0 && (
        <div style={pillRowStyle}>
          {segments.map((seg) => (
            <button
              key={seg.id}
              onClick={() => onStored(seg)}
              title={`Segment: ${seg.name}`}
              style={pillStyle(activeSegmentId === seg.id, 'rgba(245,158,11,0.04)')}
            >
              {seg.name}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function AnalysisError({ message }: { message: string }) {
  return (
    <div style={{
      background: 'rgba(239,68,68,0.1)',
      border: `1px solid rgba(239,68,68,0.3)`,
      borderRadius: 8, padding: '1rem 1.25rem', color: C.error, fontSize: 14,
    }}>
      Failed to load analysis: {message}
    </div>
  )
}

export function AnalysisResult({ projectId, analysis, loading, error, segmentLabel, activeSegment, activeSegmentId, environment, apiKey, onWatchRecordings }: {
  projectId: string
  analysis: FunnelAnalysis | null
  loading: boolean
  error: string | null
  segmentLabel: string
  activeSegment: string
  activeSegmentId?: string
  environment: string
  apiKey?: string
  onWatchRecordings: (step: number) => void
}) {
  if (loading) return <div style={{ color: C.muted, padding: '2rem' }}>Loading analysis…</div>
  if (error) return <AnalysisError message={error} />
  if (!analysis) return null
  return (
    <>
      <FunnelDetail analysis={analysis} activeSegment={segmentLabel} apiKey={apiKey} onWatchRecordings={onWatchRecordings} />
      <BreakdownPanel
        projectId={projectId}
        funnel={analysis.funnel}
        environment={environment}
        segment={activeSegmentId ? undefined : activeSegment}
        segmentId={activeSegmentId}
      />
    </>
  )
}

export function FunnelModals({ projectId, showCreate, showEdit, selected, pendingTemplate, onCloseCreate, onCloseEdit, onCreated, onUpdated }: {
  projectId: string
  showCreate: boolean
  showEdit: boolean
  selected: Funnel | null
  pendingTemplate?: FunnelTemplate
  onCloseCreate: () => void
  onCloseEdit: () => void
  onCreated: (f: Funnel) => void
  onUpdated: (f: Funnel) => void
}) {
  return (
    <>
      {showCreate && (
        <CreateFunnelModal projectId={projectId} initialTemplate={pendingTemplate} onClose={onCloseCreate} onCreated={onCreated} />
      )}
      {showEdit && selected && (
        <EditFunnelModal projectId={projectId} funnel={selected} onClose={onCloseEdit} onUpdated={onUpdated} />
      )}
    </>
  )
}
