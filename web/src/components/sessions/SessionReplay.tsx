import { useEffect, useRef, useState } from 'react'
import { X, Flag } from 'lucide-react'
import { Replayer } from 'rrweb'
import { api, type Recording, type FlagEvaluationEntry } from '../../lib/api'
import { C } from '../../lib/theme'

interface Props {
  projectId: string
  recording: Recording
  onClose: () => void
}

function formatDuration(ms: number): string {
  const s = Math.floor(ms / 1000)
  const m = Math.floor(s / 60)
  return m > 0 ? `${m}m ${s % 60}s` : `${s}s`
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleString()
}

export function SessionReplay({ projectId, recording, onClose }: Props) {
  const playerRef = useRef<HTMLDivElement>(null)
  const replayerRef = useRef<Replayer | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [flags, setFlags] = useState<FlagEvaluationEntry[]>([])
  const [progress, setProgress] = useState(0)

  useEffect(() => {
    let cancelled = false

    async function loadAndPlay() {
      if (!playerRef.current) return
      try {
        const allEvents: unknown[] = []
        const start = recording.first_chunk_index ?? 0
        const end = start + recording.chunk_count
        for (let i = start; i < end; i++) {
          setProgress(Math.round(((i - start) / recording.chunk_count) * 100))
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

        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const replayer = new Replayer(allEvents as any, {
          root: playerRef.current,
          speed: 1,
          skipInactive: true,
        })
        replayerRef.current = replayer
        replayer.play()
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
    }
  }, [projectId, recording])

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
          {/* Player area */}
          <div style={{ flex: 1, position: 'relative', background: '#000', overflow: 'hidden' }}>
            {loading && (
              <div style={{
                position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column',
                alignItems: 'center', justifyContent: 'center', gap: 16, color: C.muted,
              }}>
                <div style={{
                  width: 200, height: 4, background: C.surface, borderRadius: 2, overflow: 'hidden',
                }}>
                  <div style={{
                    height: '100%', background: C.amber, borderRadius: 2,
                    width: `${progress}%`, transition: 'width 0.2s',
                  }} />
                </div>
                <span style={{ fontSize: 13 }}>
                  {error ?? `Loading chunks… ${progress}%`}
                </span>
              </div>
            )}
            <div ref={playerRef} style={{ width: '100%', height: '100%' }} />
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
