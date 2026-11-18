import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import Shell from '../components/shell/Shell'
import { SessionReplay } from '../components/sessions/SessionReplay'
import { PAGE_SIZE, loadRecordingPage } from '../components/sessions/loadRecordings'
import {
  BulkBar, EmptyRecordings, Pagination, PendingDeleteRow, RecordingRow, RecordingsHeader,
} from '../components/sessions/RecordingRows'
import { api, type Recording } from '../lib/api'
import { useProjects } from '../lib/projects'
import { reportError } from '../lib/bugbarn'
import { trackEvent } from '../lib/analytics'
import { C } from '../lib/theme'

type TrafficFilter = 'all' | 'human' | 'bot'
type DeviceFilter = 'all' | 'desktop' | 'mobile' | 'tablet'

interface FilterState {
  traffic: TrafficFilter
  device: DeviceFilter
  pageUrl: string
  environment: string
}

function FilterPills<T extends string>({
  label, options, value, onChange
}: {
  label: string
  options: { value: T; label: string }[]
  value: T
  onChange: (v: T) => void
}) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
      <span style={{ fontSize: 12, color: C.muted, fontWeight: 600 }}>{label}</span>
      {options.map((o) => (
        <button
          key={o.value}
          onClick={() => onChange(o.value)}
          style={{
            fontSize: 12, padding: '3px 10px', borderRadius: 99,
            border: `1px solid ${value === o.value ? C.amber : C.border}`,
            background: value === o.value ? 'rgba(245,158,11,0.1)' : 'none',
            color: value === o.value ? C.amber : C.muted,
            cursor: 'pointer', fontWeight: value === o.value ? 700 : 400,
          }}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

function filterLabelFor(funnelId: string | null, step: string | null, page: string | null): string | null {
  if (funnelId && step) return `Funnel step ${step} drop-offs`
  return page ? `Sessions on ${decodeURIComponent(page)}` : null
}

function selectionState(total: number, selected: number) {
  const allSelected = total > 0 && selected === total
  return { allSelected, someSelected: selected > 0 && !allSelected }
}

export default function Sessions() {
  const { projectId } = useParams<{ projectId?: string }>()
  const [searchParams] = useSearchParams()
  const { projects } = useProjects()
  const [recordings, setRecordings] = useState<Recording[]>([])
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  // Each load takes a number; only the newest one may write its result, so a
  // slow older request cannot replace the list the current filters asked for.
  const latestLoad = useRef(0)
  const [offset, setOffset] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [selected, setSelected] = useState<Recording | null>(null)
  const [filters, setFilters] = useState<FilterState>({
    traffic: 'human',
    device: 'all',
    pageUrl: '',
    environment: searchParams.get('environment') ?? '',
  })
  const [pageUrlInput, setPageUrlInput] = useState('')

  const funnelId = searchParams.get('funnel')
  const stepParam = searchParams.get('step')
  const pageParam = searchParams.get('page')

  const load = useCallback(async (off: number, f: FilterState) => {
    if (!projectId) return
    const mine = ++latestLoad.current
    setLoading(true)
    try {
      const page = await loadRecordingPage(projectId, off, f, { funnelId, step: stepParam, page: pageParam })
      if (mine !== latestLoad.current) return
      setRecordings(page)
      setHasMore(page.length === PAGE_SIZE)
      setLoadFailed(false)
    } catch (e) {
      if (mine !== latestLoad.current) return
      reportError(e, { source: 'Sessions.load' })
      setRecordings([])
      setLoadFailed(true)
    } finally {
      if (mine === latestLoad.current) setLoading(false)
    }
  }, [projectId, funnelId, stepParam, pageParam])

  useEffect(() => {
    setOffset(0)
    void load(0, filters)
  }, [load, filters])

  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [bulkPendingDelete, setBulkPendingDelete] = useState(false)
  const [isMobile, setIsMobile] = useState(() => window.innerWidth < 640)

  useEffect(() => {
    const handler = () => setIsMobile(window.innerWidth < 640)
    window.addEventListener('resize', handler)
    return () => window.removeEventListener('resize', handler)
  }, [])

  const handleDelete = useCallback(async (rec: Recording) => {
    if (!projectId) return
    setPendingDeleteId(null)
    // Optimistically drop the row; reload from the server on failure so the UI
    // never lies about what's stored.
    setRecordings((prev) => prev.filter((r) => r.id !== rec.id))
    try {
      await api.deleteRecording(projectId, rec.id)
      trackEvent('recording_deleted', { recording_id: rec.id, session_id: rec.session_id })
    } catch (e) {
      reportError(e, { source: 'Sessions.handleDelete' })
      void load(offset, filters)
    }
  }, [projectId, load, offset, filters])

  const handleBulkDelete = useCallback(async () => {
    if (!projectId) return
    const ids = [...selectedIds]
    setBulkPendingDelete(false)
    setSelectedIds(new Set())
    setRecordings((prev) => prev.filter((r) => !ids.includes(r.id)))
    try {
      await Promise.all(ids.map((id) => api.deleteRecording(projectId, id)))
      trackEvent('recording_deleted', { recording_ids: ids, count: ids.length })
    } catch (e) {
      reportError(e, { source: 'Sessions.handleBulkDelete' })
      void load(offset, filters)
    }
  }, [projectId, selectedIds, load, offset, filters])

  const toggleSelect = (id: string) => {
    setPendingDeleteId(null)
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const { allSelected, someSelected } = selectionState(recordings.length, selectedIds.size)

  const toggleSelectAll = () => {
    setPendingDeleteId(null)
    setSelectedIds(allSelected ? new Set() : new Set(recordings.map((r) => r.id)))
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  const filterLabel = filterLabelFor(funnelId, stepParam, pageParam)

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, textAlign: 'center', padding: '2rem' }}>
          Select a project to view recordings.
        </div>
      </Shell>
    )
  }

  const goToPage = (next: number) => {
    setOffset(next)
    setSelectedIds(new Set())
    setBulkPendingDelete(false)
    void load(next, filters)
  }

  const updateFilter = <K extends keyof FilterState>(key: K, value: FilterState[K]) => {
    setFilters((prev) => ({ ...prev, [key]: value }))
    setOffset(0)
    setSelectedIds(new Set())
    setBulkPendingDelete(false)
  }

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Sessions</h1>
        {filterLabel && (
          <span style={{
            fontSize: 12, fontWeight: 600, padding: '3px 10px', borderRadius: 99,
            background: 'rgba(245,158,11,0.1)', color: C.amber,
            border: `1px solid rgba(245,158,11,0.25)`,
          }}>
            {filterLabel}
          </span>
        )}
      </div>

      {/* Filter bar */}
      <div style={{
        background: C.surface, border: `1px solid ${C.border}`, borderRadius: 10,
        padding: '0.75rem 1.25rem', marginBottom: '1rem',
        display: 'flex', alignItems: 'center', gap: '1.5rem', flexWrap: 'wrap',
      }}>
        <FilterPills
          label="Traffic"
          options={[
            { value: 'all' as TrafficFilter, label: 'All' },
            { value: 'human' as TrafficFilter, label: 'Human' },
            { value: 'bot' as TrafficFilter, label: 'Bot' },
          ]}
          value={filters.traffic}
          onChange={(v) => updateFilter('traffic', v)}
        />
        <FilterPills
          label="Device"
          options={[
            { value: 'all' as DeviceFilter, label: 'All' },
            { value: 'desktop' as DeviceFilter, label: 'Desktop' },
            { value: 'mobile' as DeviceFilter, label: 'Mobile' },
            { value: 'tablet' as DeviceFilter, label: 'Tablet' },
          ]}
          value={filters.device}
          onChange={(v) => updateFilter('device', v)}
        />
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, flex: 1, minWidth: 200 }}>
          <span style={{ fontSize: 12, color: C.muted, fontWeight: 600, whiteSpace: 'nowrap' }}>Page URL</span>
          <input
            value={pageUrlInput}
            onChange={(e) => setPageUrlInput(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && updateFilter('pageUrl', pageUrlInput.trim())}
            onBlur={() => updateFilter('pageUrl', pageUrlInput.trim())}
            placeholder="Filter by page visited…"
            style={{
              background: C.bg, border: `1px solid ${C.border}`, borderRadius: 6,
              color: C.text, fontSize: 12, padding: '4px 10px', flex: 1,
              outline: 'none',
            }}
          />
        </div>
      </div>

      <div style={{
        background: C.surface, border: `1px solid ${C.border}`,
        borderRadius: 12, overflow: 'hidden',
      }}>
        <div style={{
          padding: '1rem 1.5rem', borderBottom: `1px solid ${C.border}`,
          fontSize: 14, fontWeight: 700,
          display: 'flex', justifyContent: 'space-between', alignItems: 'center',
          background: bulkPendingDelete ? 'rgba(239,68,68,0.04)' : undefined,
        }}>
          <BulkBar
            bulkPendingDelete={bulkPendingDelete} selectedCount={selectedIds.size} loading={loading} shown={recordings.length}
            onConfirm={() => { void handleBulkDelete() }} onCancel={() => setBulkPendingDelete(false)}
            onClear={() => { setSelectedIds(new Set()); setBulkPendingDelete(false) }} onAskDelete={() => setBulkPendingDelete(true)}
          />
        </div>

        {loading ? (
          <div style={{ padding: '3rem', textAlign: 'center', color: C.muted }}>Loading…</div>
        ) : recordings.length === 0 ? (
          <EmptyRecordings loadFailed={loadFailed} />
        ) : (
          <>
            <RecordingsHeader isMobile={isMobile} allSelected={allSelected} someSelected={someSelected} onToggleAll={toggleSelectAll} />

            {recordings.map((rec) => pendingDeleteId === rec.id ? (
              <PendingDeleteRow key={rec.id} onConfirm={() => { void handleDelete(rec) }} onCancel={() => setPendingDeleteId(null)} />
            ) : (
              <RecordingRow
                key={rec.id} rec={rec} isMobile={isMobile} isSelected={selectedIds.has(rec.id)}
                onToggleSelect={() => toggleSelect(rec.id)} onAskDelete={() => setPendingDeleteId(rec.id)}
                onOpen={() => {
                  if (!rec.has_snapshot) return
                  trackEvent('recording_viewed', { recording_id: rec.id, session_id: rec.session_id })
                  setSelected(rec)
                }}
              />
            ))}

            <Pagination
              offset={offset} hasMore={hasMore}
              onPrev={() => goToPage(Math.max(0, offset - PAGE_SIZE))}
              onNext={() => goToPage(offset + PAGE_SIZE)}
            />
          </>
        )}
      </div>

      {selected && (
        <SessionReplay
          projectId={projectId}
          recording={selected}
          onClose={() => setSelected(null)}
        />
      )}
    </Shell>
  )
}
