import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { X, Flag, Play, Pause, SkipBack, SkipForward } from 'lucide-react'
import { Replayer, ReplayerEvents } from 'rrweb'
import 'rrweb/dist/style.css'
import { api, type Recording, type FlagEvaluationEntry } from '../../lib/api'
import { C } from '../../lib/theme'

interface Props {
  projectId: string
  recording: Recording
  onClose: () => void
}

// rrweb IncrementalSource values that represent genuine user interaction
const INTERACTION_SOURCES = new Set([1, 2, 3, 5, 6, 12]) // MouseMove, MouseInteraction, Scroll, Input, TouchMove, Drag

function computeSkipGaps(events: unknown[], threshold: number): Array<{ start: number; end: number }> {
  if (events.length === 0) return []
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const firstTs = (events[0] as any).timestamp as number
  const activityMs: number[] = []
  for (const e of events) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const ev = e as any
    if (ev.type === 3 && INTERACTION_SOURCES.has(ev.data?.source)) {
      activityMs.push(ev.timestamp - firstTs)
    }
  }
  if (activityMs.length === 0) return []
  activityMs.sort((a, b) => a - b)
  const gaps: Array<{ start: number; end: number }> = []
  if (activityMs[0] > threshold) {
    gaps.push({ start: threshold, end: activityMs[0] })
  }
  for (let i = 0; i < activityMs.length - 1; i++) {
    if (activityMs[i + 1] - activityMs[i] > threshold) {
      gaps.push({ start: activityMs[i] + threshold, end: activityMs[i + 1] })
    }
  }
  return gaps
}

function formatDuration(ms: number): string {
  const s = Math.floor(ms / 1000)
  const m = Math.floor(s / 60)
  return m > 0 ? `${m}m ${s % 60}s` : `${s}s`
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleString()
}

// m:ss clock for the scrubber (distinct from formatDuration's "1m 5s" style).
function formatClock(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const m = Math.floor(total / 60)
  const s = total % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

export function SessionReplay({ projectId, recording, onClose }: Props) {
  const playerRef = useRef<HTMLDivElement>(null)
  const replayerRef = useRef<Replayer | null>(null)
  const skipGapsRef = useRef<Array<{ start: number; end: number }>>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [flags, setFlags] = useState<FlagEvaluationEntry[]>([])
  const [progress, setProgress] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [currentMs, setCurrentMs] = useState(0)
  const [totalMs, setTotalMs] = useState(0)
  const [speed, setSpeed] = useState(1)
  const [skipInactive, setSkipInactive] = useState(true)

  useEffect(() => {
    let cancelled = false
    let ro: ResizeObserver | null = null

    async function loadAndPlay() {
      if (!playerRef.current) return
      try {
        const allEvents: unknown[] = []
        // Iterate the explicit stored span [first, last] inclusive. Relying on
        // first + chunk_count breaks when chunks arrive out of order, retry, or
        // leave gaps; last_chunk_index is the true upper bound. Fall back to the
        // count-based span for legacy rows predating last_chunk_index.
        const start = recording.first_chunk_index ?? 0
        const end = recording.last_chunk_index || (start + recording.chunk_count - 1)
        const span = Math.max(end - start + 1, 1)
        for (let i = start; i <= end; i++) {
          setProgress(Math.round(((i - start) / span) * 100))
          try {
            const chunk = await api.getRecordingChunk(projectId, recording.id, i)
            if (cancelled) return
            allEvents.push(...chunk)
          } catch {
            // chunk missing in storage — skip and continue
          }
        }
        setProgress(100)

        if (!playerRef.current || cancelled) return

        if (allEvents.length === 0) {
          setError('Recording data not found — chunks may still be uploading or were never stored.')
          return
        }

        // rrweb requires a full-snapshot event (type 2) to reconstruct the page
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const hasSnapshot = allEvents.some((e: any) => e?.type === 2)
        if (!hasSnapshot) {
          setError('Recording is incomplete — the initial page snapshot is missing.')
          return
        }

        // Read recorded viewport dimensions from the meta event (type 4).
        // rrweb v2's raw Replayer sets the iframe to these exact pixel dimensions
        // with no built-in scaling, so we must scale manually.
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const metaEvent = allEvents.find((e: any) => e?.type === 4) as any
        const recordedWidth: number = metaEvent?.data?.width ?? 1280
        const recordedHeight: number = metaEvent?.data?.height ?? 720

        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const replayer = new Replayer(allEvents as any, {
          root: playerRef.current,
          speed: 1,
          skipInactive: true,
        })
        replayerRef.current = replayer

        function applyScale() {
          const container = playerRef.current
          if (!container) return
          const wrapper = container.querySelector<HTMLElement>('.replayer-wrapper')
          if (!wrapper) return
          const scale = Math.min(
            container.clientWidth / recordedWidth,
            container.clientHeight / recordedHeight,
          )
          wrapper.style.transformOrigin = 'top left'
          wrapper.style.transform = `scale(${scale})`
          wrapper.style.position = 'absolute'
          wrapper.style.top = '0'
          wrapper.style.left = '0'
        }

        // Give rrweb one frame to build the .replayer-wrapper DOM before scaling
        requestAnimationFrame(() => setTimeout(applyScale, 0))
        ro = new ResizeObserver(applyScale)
        ro.observe(playerRef.current)

        const meta = replayer.getMetaData()
        setTotalMs(meta.totalTime)
        skipGapsRef.current = computeSkipGaps(allEvents, 5000)
        replayer.on(ReplayerEvents.Finish, () => {
          setPlaying(false)
          setCurrentMs(meta.totalTime)
        })

        replayer.play()
        setPlaying(true)
        setLoading(false)
      } catch {
        if (!cancelled) setError('Failed to load recording')
      }
    }

    void loadAndPlay()

    api.getRecordingFlags(projectId, recording.id)
      .then((r) => { if (!cancelled) setFlags(r.evaluations) })
      .catch(() => {})

    return () => {
      cancelled = true
      replayerRef.current?.pause()
      ro?.disconnect()
      skipGapsRef.current = []
    }
  }, [projectId, recording])

  // Advance the scrubber while playing. 100ms is smooth enough for a progress
  // bar without re-rendering the modal on every animation frame.
  useEffect(() => {
    if (!playing) return
    const id = setInterval(() => {
      const r = replayerRef.current
      if (!r) return
      const cur = r.getCurrentTime()
      if (skipInactive && skipGapsRef.current.length > 0) {
        const gap = skipGapsRef.current.find(g => cur >= g.start && cur <= g.end)
        if (gap) {
          r.play(gap.end)
          setCurrentMs(gap.end)
          return
        }
      }
      setCurrentMs(Math.min(cur, totalMs))
    }, 100)
    return () => clearInterval(id)
  }, [playing, totalMs, skipInactive])

  function seek(ms: number) {
    const r = replayerRef.current
    if (!r) return
    const clamped = Math.max(0, Math.min(ms, totalMs))
    // play(offset) seeks-and-plays; pause(offset) seeks-and-holds the frame.
    if (playing) r.play(clamped)
    else r.pause(clamped)
    setCurrentMs(clamped)
  }

  function togglePlay() {
    const r = replayerRef.current
    if (!r) return
    if (playing) {
      r.pause()
      setPlaying(false)
    } else {
      // Restart from the beginning if we're sitting at the end.
      const from = currentMs >= totalMs ? 0 : currentMs
      r.play(from)
      setPlaying(true)
    }
  }

  function changeSpeed(s: number) {
    replayerRef.current?.setConfig({ speed: s })
    setSpeed(s)
  }

  const iconBtn: CSSProperties = {
    background: 'none', border: 'none', cursor: 'pointer', color: C.text,
    padding: 4, display: 'flex', alignItems: 'center', borderRadius: 6,
  }

  return (
    <div
      style={{
        position: 'fixed', inset: 0, zIndex: 1000,
        background: 'rgba(0,0,0,0.85)',
        display: 'flex', alignItems: 'stretch', justifyContent: 'center',
      }}
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div style={{
        display: 'flex', flexDirection: 'column', width: '100%', maxWidth: 1200,
        margin: '2rem auto', background: C.bg, borderRadius: 12,
        border: `1px solid ${C.border}`, overflow: 'hidden',
      }}>
        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '0.75rem 1.25rem', borderBottom: `1px solid ${C.border}`,
          flexShrink: 0,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <span style={{ fontSize: 14, fontWeight: 700 }}>Session replay</span>
            <span style={{ fontSize: 12, color: C.muted }}>
              {formatTime(recording.started_at)} · {formatDuration(recording.duration_ms)}
            </span>
            {recording.environment && (
              <span style={{
                fontSize: 11, fontWeight: 600, padding: '2px 8px', borderRadius: 99,
                background: 'rgba(245,158,11,0.1)', color: C.amber,
                border: `1px solid rgba(245,158,11,0.25)`,
              }}>
                {recording.environment}
              </span>
            )}
          </div>
          <button
            onClick={onClose}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.muted, padding: 4 }}
          >
            <X size={18} />
          </button>
        </div>

        {/* Body */}
        <div style={{ display: 'flex', flex: 1, overflow: 'hidden', minHeight: 0 }}>
          {/* Player column: viewport + controls */}
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
            <div style={{ flex: 1, position: 'relative', background: '#000', overflow: 'hidden' }}>
              {(loading || error) && (
                <div style={{
                  position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column',
                  alignItems: 'center', justifyContent: 'center', gap: 16, color: C.muted,
                  zIndex: 1,
                }}>
                  {!error && (
                    <div style={{
                      width: 200, height: 4, background: C.surface, borderRadius: 2, overflow: 'hidden',
                    }}>
                      <div style={{
                        height: '100%', background: C.amber, borderRadius: 2,
                        width: `${progress}%`, transition: 'width 0.2s',
                      }} />
                    </div>
                  )}
                  <span style={{ fontSize: 13, maxWidth: 320, textAlign: 'center' }}>
                    {error ?? `Loading chunks… ${progress}%`}
                  </span>
                </div>
              )}
              <div ref={playerRef} style={{ width: '100%', height: '100%', position: 'relative' }} />
            </div>

            {/* Playback controls */}
            {!loading && !error && (
              <div style={{
                display: 'flex', alignItems: 'center', gap: 12,
                padding: '0.5rem 1rem', borderTop: `1px solid ${C.border}`,
                background: C.surface, flexShrink: 0,
              }}>
                <button onClick={() => seek(currentMs - 10000)} title="Back 10s" style={iconBtn}>
                  <SkipBack size={16} />
                </button>
                <button onClick={togglePlay} title={playing ? 'Pause' : 'Play'} style={iconBtn}>
                  {playing ? <Pause size={18} /> : <Play size={18} />}
                </button>
                <button onClick={() => seek(currentMs + 10000)} title="Forward 10s" style={iconBtn}>
                  <SkipForward size={16} />
                </button>
                <span style={{
                  fontSize: 12, color: C.muted, fontVariantNumeric: 'tabular-nums',
                  minWidth: 92, textAlign: 'center',
                }}>
                  {formatClock(currentMs)} / {formatClock(totalMs)}
                </span>
                <input
                  type="range" min={0} max={totalMs || 0} value={Math.min(currentMs, totalMs || 0)}
                  onChange={(e) => seek(Number(e.target.value))}
                  style={{ flex: 1, accentColor: C.amber, cursor: 'pointer' }}
                />
                <select
                  value={speed}
                  onChange={(e) => changeSpeed(Number(e.target.value))}
                  title="Playback speed"
                  style={{
                    background: C.bg, color: C.text, border: `1px solid ${C.border}`,
                    borderRadius: 6, fontSize: 12, padding: '3px 6px', cursor: 'pointer',
                  }}
                >
                  {[0.5, 1, 2, 4, 8].map((s) => <option key={s} value={s}>{s}x</option>)}
                </select>
                <button
                  onClick={() => setSkipInactive(!skipInactive)}
                  title="Skip periods with no user activity"
                  style={{
                    background: skipInactive ? 'rgba(245,158,11,0.1)' : 'none',
                    border: `1px solid ${skipInactive ? C.amber : C.border}`,
                    cursor: 'pointer',
                    color: skipInactive ? C.amber : C.muted,
                    fontSize: 11, padding: '3px 8px', borderRadius: 6, whiteSpace: 'nowrap',
                  }}
                >
                  Skip inactive
                </button>
              </div>
            )}
          </div>

          {/* Sidebar — flag evaluations */}
          {flags.length > 0 && (
            <div style={{
              width: 240, borderLeft: `1px solid ${C.border}`, overflowY: 'auto',
              background: C.surface, flexShrink: 0,
            }}>
              <div style={{
                padding: '0.75rem 1rem', borderBottom: `1px solid ${C.border}`,
                fontSize: 12, fontWeight: 700, color: C.muted,
                display: 'flex', alignItems: 'center', gap: 6,
              }}>
                <Flag size={12} />
                Flag evaluations
              </div>
              {flags.map((f, i) => (
                <div key={i} style={{
                  padding: '0.6rem 1rem', borderBottom: `1px solid ${C.border}`,
                  fontSize: 12,
                }}>
                  <div style={{ fontWeight: 600, marginBottom: 2 }}>{f.flag_name}</div>
                  <div style={{ color: C.amber, marginBottom: 2 }}>{f.variant}</div>
                  <div style={{ color: C.muted }}>
                    {new Date(f.evaluated_at).toLocaleTimeString()}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
