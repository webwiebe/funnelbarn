import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { LineChart, Line, ResponsiveContainer, Tooltip } from 'recharts'
import { Radio } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api } from '../lib/api'
import { useProjects } from '../lib/projects'
import { Skeleton } from '../components/ui/Skeleton'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  success: '#10b981',
}

interface LiveEvent {
  id: string
  name: string
  url: string
  timestamp: string
}

interface SparkPoint {
  t: number
  count: number
}

const MAX_EVENTS = 50
const MAX_SPARKPOINTS = 60

function timeAgo(ts: string): string {
  const diff = Date.now() - new Date(ts).getTime()
  if (diff < 60000) return `${Math.floor(diff / 1000)}s ago`
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  return `${Math.floor(diff / 3600000)}h ago`
}

export default function Live() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const [events, setEvents] = useState<LiveEvent[]>([])
  const [sparkData, setSparkData] = useState<SparkPoint[]>([])
  const [activeSessions, setActiveSessions] = useState(0)
  const [sessionsLoading, setSessionsLoading] = useState(true)
  const [connected, setConnected] = useState(false)
  const eventRef = useRef<LiveEvent[]>([])
  const counterRef = useRef(0)

  // Accumulate spark data — one point per second
  useEffect(() => {
    const interval = setInterval(() => {
      const count = counterRef.current
      counterRef.current = 0
      setSparkData((prev) => {
        const next = [...prev, { t: Date.now(), count }]
        return next.slice(-MAX_SPARKPOINTS)
      })
    }, 1000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    if (!projectId) return

    // Try SSE stream first
    let es: EventSource | null = null
    let polling: ReturnType<typeof setInterval> | null = null

    const trySSE = () => {
      try {
        es = new EventSource(`/api/v1/projects/${projectId}/events?stream=true`)
        es.onopen = () => setConnected(true)
        es.onerror = () => {
          setConnected(false)
          es?.close()
          startPolling()
        }
        es.onmessage = (e) => {
          try {
            const ev: LiveEvent = JSON.parse(e.data)
            addEvent(ev)
          } catch {
            // ignore parse errors
          }
        }
      } catch {
        startPolling()
      }
    }

    const addEvent = (ev: LiveEvent) => {
      counterRef.current++
      eventRef.current = [ev, ...eventRef.current].slice(0, MAX_EVENTS)
      setEvents([...eventRef.current])
    }

    const startPolling = () => {
      setConnected(true) // optimistic
      polling = setInterval(async () => {
        try {
          const res = await fetch(`/api/v1/projects/${projectId}/events?limit=5`, { credentials: 'include' })
          if (!res.ok) return
          const data = await res.json()
          const incoming: LiveEvent[] = data.events || []
          if (incoming.length > 0) {
            // Only add truly new events (not already in list)
            const existing = new Set(eventRef.current.map((e) => e.id))
            const newOnes = incoming.filter((e) => !existing.has(e.id))
            newOnes.forEach(addEvent)
          }
        } catch {
          // ignore
        }
      }, 3000)
    }

    trySSE()

    return () => {
      es?.close()
      if (polling) clearInterval(polling)
      setConnected(false)
    }
  }, [projectId])

  // Poll active sessions from the backend every 30 seconds
  useEffect(() => {
    if (!projectId) return
    const poll = (isFirst = false) => {
      api.getActiveSessions(projectId)
        .then((d) => setActiveSessions(d.active_sessions))
        .catch(() => {}) // silent fail — keep last known value
        .finally(() => { if (isFirst) setSessionsLoading(false) })
    }
    poll(true) // immediate first fetch
    const interval = setInterval(() => poll(false), 30_000)
    return () => clearInterval(interval)
  }, [projectId])

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, textAlign: 'center', padding: '2rem' }}>
          Select a project to view live stats.
        </div>
      </Shell>
    )
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Live</h1>
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          fontSize: 12,
          fontWeight: 600,
          color: connected ? C.success : C.muted,
          background: connected ? 'rgba(16,185,129,0.1)' : 'rgba(148,163,184,0.1)',
          border: `1px solid ${connected ? 'rgba(16,185,129,0.3)' : C.border}`,
          borderRadius: 99,
          padding: '0.25rem 0.75rem',
        }}>
          <Radio size={12} />
          {connected ? 'Connected' : 'Connecting…'}
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 2fr', gap: '1.5rem', marginBottom: '1.5rem' }}>
        {/* Active sessions */}
        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '2rem',
          textAlign: 'center',
        }}>
          <div style={{ fontSize: 13, color: C.muted, fontWeight: 500, marginBottom: 12 }}>
            Active sessions
          </div>
          {sessionsLoading ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, alignItems: 'center' }}>
              <Skeleton height={72} width={80} />
            </div>
          ) : (
            <div style={{
              fontSize: 72,
              fontWeight: 900,
              letterSpacing: '-3px',
              color: C.success,
              lineHeight: 1,
              marginBottom: 8,
            }}>
              {activeSessions}
            </div>
          )}
          <div style={{ fontSize: 13, color: C.muted }}>active in last 5 min</div>
        </div>

        {/* Sparkline */}
        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '1.5rem',
        }}>
          <div style={{ fontSize: 13, color: C.muted, fontWeight: 500, marginBottom: 12 }}>
            Events per second (last 60s)
          </div>
          <ResponsiveContainer width="100%" height={100}>
            <LineChart data={sparkData}>
              <Line
                type="monotone"
                dataKey="count"
                stroke={C.amber}
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
              />
              <Tooltip
                contentStyle={{ background: '#1a1d27', border: '1px solid #2a2d3a', borderRadius: 8, fontSize: 12 }}
                labelFormatter={() => ''}
                formatter={(v) => [`${v ?? 0}`, 'events'] as [string, string]}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>

      {/* Event stream */}
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 12,
        overflow: 'hidden',
      }}>
        <div style={{
          padding: '1rem 1.5rem',
          borderBottom: `1px solid ${C.border}`,
          fontSize: 14,
          fontWeight: 700,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}>
          <span>Event stream</span>
          <span style={{ fontSize: 12, color: C.muted, fontWeight: 400 }}>
            {events.length} events
          </span>
        </div>

        {events.length === 0 ? (
          <div style={{
            padding: '3rem',
            textAlign: 'center',
            color: C.muted,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 8,
          }}>
            <Radio size={32} opacity={0.3} />
            <div>Waiting for events…</div>
            <div style={{ fontSize: 13 }}>Events will appear here as they come in</div>
          </div>
        ) : (
          <div style={{ maxHeight: 500, overflowY: 'auto' }}>
            {events.map((ev, i) => (
              <div
                key={ev.id || i}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '0.75rem 1.5rem',
                  borderBottom: `1px solid ${C.border}`,
                  animation: i === 0 ? 'slideIn 0.2s ease' : undefined,
                  background: i === 0 ? 'rgba(245,158,11,0.04)' : 'transparent',
                }}
              >
                <div style={{
                  width: 8,
                  height: 8,
                  borderRadius: '50%',
                  background: C.amber,
                  flexShrink: 0,
                  opacity: i === 0 ? 1 : 0.3,
                }} />
                <div style={{
                  fontSize: 13,
                  fontWeight: 700,
                  color: C.amber,
                  minWidth: 120,
                  flexShrink: 0,
                }}>
                  {ev.name}
                </div>
                <div style={{
                  fontSize: 13,
                  color: C.muted,
                  flex: 1,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}>
                  {ev.url}
                </div>
                <div style={{ fontSize: 12, color: C.muted, flexShrink: 0 }}>
                  {timeAgo(ev.timestamp)}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <style>{`
        @keyframes slideIn {
          from { opacity: 0; transform: translateY(-8px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </Shell>
  )
}
