import { Video, VideoOff, Trash2, ChevronLeft, ChevronRight, Monitor, Smartphone, Tablet, Bot, User } from 'lucide-react'
import type { Recording } from '../../lib/api'
import { C } from '../../lib/theme'
import { recordingPath } from './loadRecordings'

function formatDuration(ms: number): string {
  const s = Math.floor(ms / 1000)
  const m = Math.floor(s / 60)
  return m > 0 ? `${m}m ${s % 60}s` : `${s}s`
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

function DeviceIcon({ type }: { type: string }) {
  if (type === 'mobile') return <Smartphone size={12} />
  if (type === 'tablet') return <Tablet size={12} />
  return <Monitor size={12} />
}

const GRID_MOBILE = '32px 1fr auto'
const GRID_DESKTOP = '32px 1fr 90px 90px 110px 90px 110px'

const outlineBtn = {
  fontSize: 12, borderRadius: 6, background: 'none', color: C.muted,
  border: `1px solid ${C.border}`, cursor: 'pointer',
} as const

export function BulkBar({ bulkPendingDelete, selectedCount, loading, shown, onConfirm, onCancel, onClear, onAskDelete }: {
  bulkPendingDelete: boolean
  selectedCount: number
  loading: boolean
  shown: number
  onConfirm: () => void
  onCancel: () => void
  onClear: () => void
  onAskDelete: () => void
}) {
  if (bulkPendingDelete) {
    return (
      <>
        <button
          onClick={onConfirm}
          style={{
            fontSize: 12, padding: '4px 14px', borderRadius: 6,
            background: C.error, color: '#fff', border: 'none',
            cursor: 'pointer', fontWeight: 600,
          }}
        >
          Confirm delete {selectedCount} recording{selectedCount !== 1 ? 's' : ''}
        </button>
        <button onClick={onCancel} style={{ ...outlineBtn, padding: '4px 14px' }}>
          Cancel
        </button>
      </>
    )
  }
  if (selectedCount > 0) {
    return (
      <>
        <span style={{ fontSize: 13, color: C.muted, fontWeight: 400 }}>
          {selectedCount} selected
        </span>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <button onClick={onClear} style={{ ...outlineBtn, padding: '4px 10px' }}>
            Clear
          </button>
          <button
            onClick={onAskDelete}
            style={{
              fontSize: 12, padding: '4px 12px', borderRadius: 6,
              background: 'none', color: C.error,
              border: `1px solid ${C.error}`, cursor: 'pointer',
              display: 'flex', alignItems: 'center', gap: 6, fontWeight: 600,
            }}
          >
            <Trash2 size={12} /> Delete {selectedCount}
          </button>
        </div>
      </>
    )
  }
  return (
    <>
      <span>Recordings</span>
      <span style={{ fontSize: 12, color: C.muted, fontWeight: 400 }}>
        {loading ? 'Loading…' : `${shown} shown`}
      </span>
    </>
  )
}

export function EmptyRecordings({ loadFailed }: { loadFailed: boolean }) {
  return (
    <div style={{
      padding: '3rem', textAlign: 'center', color: C.muted,
      display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8,
    }}>
      <Video size={32} opacity={0.3} />
      {loadFailed ? (
        <>
          <div>Could not load recordings</div>
          <div style={{ fontSize: 13 }}>Change a filter or reload the page to try again.</div>
        </>
      ) : (
        <>
          <div>No recordings found</div>
          <div style={{ fontSize: 13 }}>
            Enable recording by passing <code style={{ background: C.bg, padding: '1px 6px', borderRadius: 4 }}>record: true</code> to the SDK
          </div>
        </>
      )}
    </div>
  )
}

export function RecordingsHeader({ isMobile, allSelected, someSelected, onToggleAll }: {
  isMobile: boolean
  allSelected: boolean
  someSelected: boolean
  onToggleAll: () => void
}) {
  return (
    <div style={{
      display: 'grid',
      gridTemplateColumns: isMobile ? GRID_MOBILE : GRID_DESKTOP,
      padding: '0.5rem 1.5rem',
      fontSize: 11, fontWeight: 700, color: C.muted, textTransform: 'uppercase',
      borderBottom: `1px solid ${C.border}`,
      alignItems: 'center',
    }}>
      <div style={{ display: 'flex', alignItems: 'center' }}>
        <input
          type="checkbox"
          checked={allSelected}
          ref={(el) => { if (el) el.indeterminate = someSelected }}
          onChange={onToggleAll}
          style={{ cursor: 'pointer', accentColor: C.amber }}
        />
      </div>
      <span>Started</span>
      {isMobile ? (
        <span>Replay</span>
      ) : (
        <>
          <span>Duration</span>
          <span>Device</span>
          <span>Environment</span>
          <span>Traffic</span>
          <span>Replay</span>
        </>
      )}
    </div>
  )
}

export function PendingDeleteRow({ onConfirm, onCancel }: { onConfirm: () => void; onCancel: () => void }) {
  return (
    <div
      style={{
        display: 'flex', alignItems: 'center', gap: 12,
        padding: '0.75rem 1.5rem',
        borderBottom: `1px solid ${C.border}`,
        background: 'rgba(239,68,68,0.04)',
      }}
    >
      <button
        onClick={(e) => { e.stopPropagation(); onConfirm() }}
        style={{
          fontSize: 12, padding: '4px 14px', borderRadius: 6,
          background: C.error, color: '#fff', border: 'none',
          cursor: 'pointer', fontWeight: 600, flexShrink: 0,
        }}
      >
        Confirm delete
      </button>
      <span style={{ fontSize: 12, color: C.muted }}>
        Delete this recording? This cannot be undone.
      </span>
      <button
        onClick={(e) => { e.stopPropagation(); onCancel() }}
        style={{ ...outlineBtn, marginLeft: 'auto', padding: '4px 14px', flexShrink: 0 }}
      >
        Cancel
      </button>
    </div>
  )
}

function ReplayCell({ rec, isMobile, onAskDelete }: { rec: Recording; isMobile: boolean; onAskDelete: () => void }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      {rec.has_snapshot ? (
        <span style={{ display: 'flex', alignItems: 'center', gap: 4, color: C.muted, fontSize: 12 }}>
          <Video size={14} /> Play
        </span>
      ) : (
        <span
          title="No initial snapshot was captured — this recording can't be replayed."
          style={{ display: 'flex', alignItems: 'center', gap: 4, color: C.muted, fontSize: 11, opacity: 0.7 }}
        >
          <VideoOff size={14} />
          {!isMobile && ' No replay'}
        </span>
      )}
      <button
        onClick={(e) => { e.stopPropagation(); onAskDelete() }}
        title="Delete recording"
        style={{
          background: 'none', border: 'none', cursor: 'pointer',
          color: C.muted, padding: 2, display: 'flex', alignItems: 'center',
        }}
        onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.color = C.error }}
        onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.color = C.muted }}
      >
        <Trash2 size={14} />
      </button>
    </div>
  )
}

function pagePathLabel(rec: Recording): string {
  return rec.page_url ? recordingPath(rec.page_url) : rec.session_id.slice(0, 12) + '…'
}

function MobileSummary({ rec }: { rec: Recording }) {
  return (
    <div style={{ minWidth: 0 }}>
      <div style={{ fontSize: 13, fontWeight: 600 }}>{formatDate(rec.started_at)}</div>
      <div style={{ fontSize: 11, color: C.muted, marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {pagePathLabel(rec)}
      </div>
      <div style={{ fontSize: 11, color: C.muted, marginTop: 2, display: 'flex', alignItems: 'center', gap: 6, overflow: 'hidden' }}>
        <span>{formatDuration(rec.duration_ms)}</span>
        <span>·</span>
        <DeviceIcon type={rec.device_type} />
        <span style={{ textTransform: 'capitalize' }}>{rec.device_type || 'desktop'}</span>
        {rec.environment && (
          <>
            <span>·</span>
            <span style={{ color: C.amber }}>{rec.environment}</span>
          </>
        )}
      </div>
    </div>
  )
}

function DesktopCells({ rec }: { rec: Recording }) {
  return (
    <>
      <div style={{ fontSize: 13, color: C.text }}>{formatDuration(rec.duration_ms)}</div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: C.muted }}>
        <DeviceIcon type={rec.device_type} />
        <span style={{ textTransform: 'capitalize' }}>{rec.device_type || 'desktop'}</span>
      </div>
      <div>
        {rec.environment ? (
          <span style={{
            fontSize: 11, fontWeight: 600, padding: '2px 8px', borderRadius: 99,
            background: 'rgba(245,158,11,0.08)', color: C.amber,
            border: `1px solid rgba(245,158,11,0.2)`,
          }}>
            {rec.environment}
          </span>
        ) : (
          <span style={{ color: C.muted, fontSize: 12 }}>—</span>
        )}
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 12, color: C.muted }}>
        {rec.is_bot ? <Bot size={12} /> : <User size={12} />}
        <span>{rec.is_bot ? 'Bot' : 'Human'}</span>
      </div>
    </>
  )
}

export function RecordingRow({ rec, isMobile, isSelected, onToggleSelect, onAskDelete, onOpen }: {
  rec: Recording
  isMobile: boolean
  isSelected: boolean
  onToggleSelect: () => void
  onAskDelete: () => void
  onOpen: () => void
}) {
  return (
    <div
      onClick={onOpen}
      style={{
        display: 'grid',
        gridTemplateColumns: isMobile ? GRID_MOBILE : GRID_DESKTOP,
        padding: '0.75rem 1.5rem',
        borderBottom: `1px solid ${C.border}`,
        cursor: rec.has_snapshot ? 'pointer' : 'default',
        transition: 'background 0.1s',
        alignItems: 'center',
        background: isSelected ? 'rgba(245,158,11,0.04)' : undefined,
      }}
      onMouseEnter={(e) => { if (!isSelected) (e.currentTarget as HTMLElement).style.background = 'rgba(255,255,255,0.03)' }}
      onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.background = isSelected ? 'rgba(245,158,11,0.04)' : 'transparent' }}
    >
      <div style={{ display: 'flex', alignItems: 'center' }}>
        <input
          type="checkbox"
          checked={isSelected}
          onChange={onToggleSelect}
          onClick={(e) => e.stopPropagation()}
          style={{ cursor: 'pointer', accentColor: C.amber }}
        />
      </div>
      {isMobile ? (
        <MobileSummary rec={rec} />
      ) : (
        <div>
          <div style={{ fontSize: 13, fontWeight: 600 }}>{formatDate(rec.started_at)}</div>
          <div style={{ fontSize: 11, color: C.muted, marginTop: 2 }}>{pagePathLabel(rec)}</div>
        </div>
      )}
      {!isMobile && <DesktopCells rec={rec} />}
      <ReplayCell rec={rec} isMobile={isMobile} onAskDelete={onAskDelete} />
    </div>
  )
}

const pagerBtn = {
  display: 'flex', alignItems: 'center', gap: 4,
  background: 'none', border: `1px solid ${C.border}`, borderRadius: 6,
  padding: '4px 10px', fontSize: 12,
} as const

export function Pagination({ offset, hasMore, onPrev, onNext }: {
  offset: number
  hasMore: boolean
  onPrev: () => void
  onNext: () => void
}) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', justifyContent: 'flex-end',
      gap: 8, padding: '0.75rem 1.5rem',
    }}>
      <button
        disabled={offset === 0}
        onClick={onPrev}
        style={{ ...pagerBtn, color: offset === 0 ? C.muted : C.text, cursor: offset === 0 ? 'not-allowed' : 'pointer' }}
      >
        <ChevronLeft size={14} /> Prev
      </button>
      <button
        disabled={!hasMore}
        onClick={onNext}
        style={{ ...pagerBtn, color: !hasMore ? C.muted : C.text, cursor: !hasMore ? 'not-allowed' : 'pointer' }}
      >
        Next <ChevronRight size={14} />
      </button>
    </div>
  )
}
