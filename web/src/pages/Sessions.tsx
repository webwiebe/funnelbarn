import { useCallback, useEffect, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { Video, VideoOff, Trash2, ChevronLeft, ChevronRight, Monitor, Smartphone, Tablet, Bot, User } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { SessionReplay } from '../components/sessions/SessionReplay'
import { api, type Recording } from '../lib/api'
import { useProjects } from '../lib/projects'
import { C } from '../lib/theme'

const PAGE_SIZE = 50

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

export default function Sessions() {
  const { projectId } = useParams<{ projectId?: string }>()
  const [searchParams] = useSearchParams()
  const { projects } = useProjects()
  const [recordings, setRecordings] = useState<Recording[]>([])
  const [loading, setLoading] = useState(true)
  const [offset, setOffset] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [selected, setSelected] = useState<Recording | null>(null)
  const [filters, setFilters] = useState<FilterState>({
    traffic: 'human',
    device: 'all',
    pageUrl: '',
    environment: '',
  })
  const [pageUrlInput, setPageUrlInput] = useState('')

  const funnelId = searchParams.get('funnel')
  const stepParam = searchParams.get('step')
  const pageParam = searchParams.get('page')

  const load = useCallback(async (off: number, f: FilterState) => {
    if (!projectId) return
    setLoading(true)
    try {
      let sessionIds: string[] | undefined

      if (funnelId && stepParam) {
        const step = parseInt(stepParam, 10)
        if (!isNaN(step)) {
          const r = await api.getFunnelStepSessions(projectId, funnelId, step)
          sessionIds = r.session_ids
        }
      } else if (pageParam) {
        const r = await api.getFlowPageSessions(projectId, pageParam)
        sessionIds = r.session_ids
      }

      const result = await api.listRecordings(projectId, {
        limit: PAGE_SIZE,
        offset: off,
        session_ids: sessionIds,
        device_type: f.device !== 'all' ? f.device : undefined,
        human_only: f.traffic === 'human',
        environment: f.environment || undefined,
        page_url: f.pageUrl || undefined,
      })
      setRecordings(result.recordings)
      setHasMore(result.recordings.length === PAGE_SIZE)
    } catch {
      setRecordings([])
    } finally {
      setLoading(false)
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
    } catch {
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
    } catch {
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

  const allSelected = recordings.length > 0 && selectedIds.size === recordings.length
  const someSelected = selectedIds.size > 0 && !allSelected

  const toggleSelectAll = () => {
    setPendingDeleteId(null)
    setSelectedIds(allSelected ? new Set() : new Set(recordings.map((r) => r.id)))
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  const filterLabel = funnelId && stepParam
    ? `Funnel step ${stepParam} drop-offs`
    : pageParam
    ? `Sessions on ${decodeURIComponent(pageParam)}`
    : null

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, textAlign: 'center', padding: '2rem' }}>
          Select a project to view recordings.
        </div>
      </Shell>
    )
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
          {bulkPendingDelete ? (
            <>
              <button
                onClick={() => { void handleBulkDelete() }}
                style={{
                  fontSize: 12, padding: '4px 14px', borderRadius: 6,
                  background: C.error, color: '#fff', border: 'none',
                  cursor: 'pointer', fontWeight: 600,
                }}
              >
                Confirm delete {selectedIds.size} recording{selectedIds.size !== 1 ? 's' : ''}
              </button>
              <button
                onClick={() => setBulkPendingDelete(false)}
                style={{
                  fontSize: 12, padding: '4px 14px', borderRadius: 6,
                  background: 'none', color: C.muted,
                  border: `1px solid ${C.border}`, cursor: 'pointer',
                }}
              >
                Cancel
              </button>
            </>
          ) : selectedIds.size > 0 ? (
            <>
              <span style={{ fontSize: 13, color: C.muted, fontWeight: 400 }}>
                {selectedIds.size} selected
              </span>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <button
                  onClick={() => { setSelectedIds(new Set()); setBulkPendingDelete(false) }}
                  style={{
                    fontSize: 12, padding: '4px 10px', borderRadius: 6,
                    background: 'none', color: C.muted,
                    border: `1px solid ${C.border}`, cursor: 'pointer',
                  }}
                >
                  Clear
                </button>
                <button
                  onClick={() => setBulkPendingDelete(true)}
                  style={{
                    fontSize: 12, padding: '4px 12px', borderRadius: 6,
                    background: 'none', color: C.error,
                    border: `1px solid ${C.error}`, cursor: 'pointer',
                    display: 'flex', alignItems: 'center', gap: 6, fontWeight: 600,
                  }}
                >
                  <Trash2 size={12} /> Delete {selectedIds.size}
                </button>
              </div>
            </>
          ) : (
            <>
              <span>Recordings</span>
              <span style={{ fontSize: 12, color: C.muted, fontWeight: 400 }}>
                {loading ? 'Loading…' : `${recordings.length} shown`}
              </span>
            </>
          )}
        </div>

        {loading ? (
          <div style={{ padding: '3rem', textAlign: 'center', color: C.muted }}>Loading…</div>
        ) : recordings.length === 0 ? (
          <div style={{
            padding: '3rem', textAlign: 'center', color: C.muted,
            display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8,
          }}>
            <Video size={32} opacity={0.3} />
            <div>No recordings found</div>
            <div style={{ fontSize: 13 }}>
              Enable recording by passing <code style={{ background: C.bg, padding: '1px 6px', borderRadius: 4 }}>record: true</code> to the SDK
            </div>
          </div>
        ) : (
          <>
            {/* Table header */}
            <div style={{
              display: 'grid',
<<<<<<< HEAD
              gridTemplateColumns: isMobile ? '32px 1fr auto' : '32px 1fr 90px 90px 110px 90px 110px',
=======
              gridTemplateColumns: '32px 1fr 90px 90px 110px 90px 110px',
>>>>>>> origin/main
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
                  onChange={toggleSelectAll}
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

            {recordings.map((rec) => {
              if (pendingDeleteId === rec.id) {
                return (
                  <div
                    key={rec.id}
                    style={{
                      display: 'flex', alignItems: 'center', gap: 12,
                      padding: '0.75rem 1.5rem',
                      borderBottom: `1px solid ${C.border}`,
                      background: 'rgba(239,68,68,0.04)',
                    }}
                  >
                    <button
                      onClick={(e) => { e.stopPropagation(); void handleDelete(rec) }}
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
                      onClick={(e) => { e.stopPropagation(); setPendingDeleteId(null) }}
                      style={{
                        marginLeft: 'auto', fontSize: 12, padding: '4px 14px', borderRadius: 6,
                        background: 'none', color: C.muted,
                        border: `1px solid ${C.border}`, cursor: 'pointer', flexShrink: 0,
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                )
              }

<<<<<<< HEAD
              const isSelected = selectedIds.has(rec.id)
              const replayCell = (
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
                    onClick={(e) => { e.stopPropagation(); setPendingDeleteId(rec.id) }}
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

=======
>>>>>>> origin/main
              return (
                <div
                  key={rec.id}
                  onClick={() => { if (rec.has_snapshot) setSelected(rec) }}
                  style={{
                    display: 'grid',
<<<<<<< HEAD
                    gridTemplateColumns: isMobile ? '32px 1fr auto' : '32px 1fr 90px 90px 110px 90px 110px',
=======
                    gridTemplateColumns: '32px 1fr 90px 90px 110px 90px 110px',
>>>>>>> origin/main
                    padding: '0.75rem 1.5rem',
                    borderBottom: `1px solid ${C.border}`,
                    cursor: rec.has_snapshot ? 'pointer' : 'default',
                    transition: 'background 0.1s',
                    alignItems: 'center',
<<<<<<< HEAD
                    background: isSelected ? 'rgba(245,158,11,0.04)' : undefined,
                  }}
                  onMouseEnter={(e) => { if (!isSelected) (e.currentTarget as HTMLElement).style.background = 'rgba(255,255,255,0.03)' }}
                  onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.background = isSelected ? 'rgba(245,158,11,0.04)' : 'transparent' }}
                >
                  <div style={{ display: 'flex', alignItems: 'center' }}>
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => toggleSelect(rec.id)}
                      onClick={(e) => e.stopPropagation()}
                      style={{ cursor: 'pointer', accentColor: C.amber }}
                    />
                  </div>
                  {isMobile ? (
                    <div>
                      <div style={{ fontSize: 13, fontWeight: 600 }}>{formatDate(rec.started_at)}</div>
                      <div style={{ fontSize: 11, color: C.muted, marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {rec.page_url ? new URL(rec.page_url).pathname : rec.session_id.slice(0, 12) + '…'}
                      </div>
                      <div style={{ fontSize: 11, color: C.muted, marginTop: 2, display: 'flex', alignItems: 'center', gap: 6 }}>
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
                  ) : (
                    <div>
                      <div style={{ fontSize: 13, fontWeight: 600 }}>{formatDate(rec.started_at)}</div>
                      <div style={{ fontSize: 11, color: C.muted, marginTop: 2 }}>
                        {rec.page_url ? new URL(rec.page_url).pathname : rec.session_id.slice(0, 12) + '…'}
                      </div>
                    </div>
                  )}
                  {!isMobile && (
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
                  )}
                  {replayCell}
=======
                    background: selectedIds.has(rec.id) ? 'rgba(245,158,11,0.04)' : undefined,
                  }}
                  onMouseEnter={(e) => { if (!selectedIds.has(rec.id)) (e.currentTarget as HTMLElement).style.background = 'rgba(255,255,255,0.03)' }}
                  onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.background = selectedIds.has(rec.id) ? 'rgba(245,158,11,0.04)' : 'transparent' }}
                >
                  <div
                    style={{ display: 'flex', alignItems: 'center' }}
                    onClick={(e) => { e.stopPropagation(); toggleSelect(rec.id) }}
                  >
                    <input
                      type="checkbox"
                      checked={selectedIds.has(rec.id)}
                      onChange={() => toggleSelect(rec.id)}
                      style={{ cursor: 'pointer', accentColor: C.amber }}
                    />
                  </div>
                  <div>
                    <div style={{ fontSize: 13, fontWeight: 600 }}>{formatDate(rec.started_at)}</div>
                    <div style={{ fontSize: 11, color: C.muted, marginTop: 2 }}>
                      {rec.page_url ? new URL(rec.page_url).pathname : rec.session_id.slice(0, 12) + '…'}
                    </div>
                  </div>
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
                        <VideoOff size={14} /> No replay
                      </span>
                    )}
                    <button
                      onClick={(e) => { e.stopPropagation(); setPendingDeleteId(rec.id) }}
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
>>>>>>> origin/main
                </div>
              )
            })}

            {/* Pagination */}
            <div style={{
              display: 'flex', alignItems: 'center', justifyContent: 'flex-end',
              gap: 8, padding: '0.75rem 1.5rem',
            }}>
              <button
                disabled={offset === 0}
                onClick={() => { const next = Math.max(0, offset - PAGE_SIZE); setOffset(next); setSelectedIds(new Set()); setBulkPendingDelete(false); void load(next, filters) }}
                style={{
                  display: 'flex', alignItems: 'center', gap: 4,
                  background: 'none', border: `1px solid ${C.border}`, borderRadius: 6,
                  color: offset === 0 ? C.muted : C.text, cursor: offset === 0 ? 'not-allowed' : 'pointer',
                  padding: '4px 10px', fontSize: 12,
                }}
              >
                <ChevronLeft size={14} /> Prev
              </button>
              <button
                disabled={!hasMore}
                onClick={() => { const next = offset + PAGE_SIZE; setOffset(next); setSelectedIds(new Set()); setBulkPendingDelete(false); void load(next, filters) }}
                style={{
                  display: 'flex', alignItems: 'center', gap: 4,
                  background: 'none', border: `1px solid ${C.border}`, borderRadius: 6,
                  color: !hasMore ? C.muted : C.text, cursor: !hasMore ? 'not-allowed' : 'pointer',
                  padding: '4px 10px', fontSize: 12,
                }}
              >
                Next <ChevronRight size={14} />
              </button>
            </div>
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
