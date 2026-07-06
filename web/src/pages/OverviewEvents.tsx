import { useEffect, useMemo, useState } from 'react'
import Shell from '../components/shell/Shell'
import { api, OverviewEvent } from '../lib/api'
import { useProjects } from '../lib/projects'
import { OverviewTabs } from '../components/OverviewTabs'
import { C } from '../lib/theme'

export default function OverviewEvents() {
  const { projects } = useProjects()
  const [events, setEvents] = useState<OverviewEvent[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [projectFilter, setProjectFilter] = useState('')
  const [nameFilter, setNameFilter] = useState('')
  // Cursor stack for prev/next paging.
  const [cursors, setCursors] = useState<{ time: string; id: string }[]>([])
  const [nextCursor, setNextCursor] = useState<{ cursor_time: string; cursor_id: string } | null>(null)

  const nameFor = useMemo(() => {
    const m = new Map(projects.map((p) => [p.id, p.name]))
    return (id: string) => m.get(id) ?? id
  }, [projects])

  const load = (cursor?: { time: string; id: string }) => {
    setLoading(true)
    setError(null)
    api.getOverviewEvents({
      projectId: projectFilter || undefined,
      name: nameFilter || undefined,
      cursorTime: cursor?.time,
      cursorId: cursor?.id,
      limit: 50,
    })
      .then((res) => {
        setEvents(res.events)
        setNextCursor(res.next_cursor)
      })
      .catch((e) => setError(e.message || 'failed to load'))
      .finally(() => setLoading(false))
  }

  // Reload from the first page whenever a filter changes.
  useEffect(() => {
    setCursors([])
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectFilter, nameFilter])

  const goNext = () => {
    if (!nextCursor) return
    setCursors((c) => [...c, { time: nextCursor.cursor_time, id: nextCursor.cursor_id }])
    load({ time: nextCursor.cursor_time, id: nextCursor.cursor_id })
  }
  const goPrev = () => {
    const copy = [...cursors]
    copy.pop()
    const prev = copy[copy.length - 1]
    setCursors(copy)
    load(prev)
  }

  const inputStyle: React.CSSProperties = {
    background: C.bg, border: `1px solid ${C.border}`, color: C.text,
    borderRadius: 8, padding: '0.4rem 0.6rem', fontSize: 13,
  }

  return (
    <Shell>
      <OverviewTabs />
      <h1 style={{ fontSize: 22, fontWeight: 700, color: C.text, margin: '0 0 0.25rem' }}>All Events</h1>
      <div style={{ color: C.muted, fontSize: 13, marginBottom: '1.25rem' }}>Live event feed across every site</div>

      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: '1rem' }}>
        <select value={projectFilter} onChange={(e) => setProjectFilter(e.target.value)} style={inputStyle}>
          <option value="">All sites</option>
          {projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
        <input
          placeholder="Filter by event name"
          value={nameFilter}
          onChange={(e) => setNameFilter(e.target.value)}
          style={{ ...inputStyle, minWidth: 200 }}
        />
      </div>

      {error && (
        <div style={{ background: 'rgba(239,68,68,0.1)', border: `1px solid ${C.error}`, color: C.error, borderRadius: 8, padding: '0.75rem 1rem', marginBottom: '1rem' }}>{error}</div>
      )}

      <div style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, overflow: 'hidden' }}>
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ color: C.muted, textAlign: 'left', background: C.bg }}>
                <th style={{ padding: '0.5rem 0.75rem' }}>Time</th>
                <th style={{ padding: '0.5rem 0.75rem' }}>Site</th>
                <th style={{ padding: '0.5rem 0.75rem' }}>Event</th>
                <th style={{ padding: '0.5rem 0.75rem' }}>Page</th>
                <th style={{ padding: '0.5rem 0.75rem' }}>Country</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={e.id} style={{ borderTop: `1px solid ${C.border}`, color: C.text }}>
                  <td style={{ padding: '0.5rem 0.75rem', color: C.muted, whiteSpace: 'nowrap' }}>{new Date(e.occurred_at).toLocaleString()}</td>
                  <td style={{ padding: '0.5rem 0.75rem' }}>{nameFor(e.project_id)}</td>
                  <td style={{ padding: '0.5rem 0.75rem', fontWeight: 500 }}>{e.name}</td>
                  <td style={{ padding: '0.5rem 0.75rem', color: C.muted, maxWidth: 280, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{e.url}</td>
                  <td style={{ padding: '0.5rem 0.75rem', color: C.muted }}>{e.country_code || '—'}</td>
                </tr>
              ))}
              {!loading && events.length === 0 && (
                <tr><td colSpan={5} style={{ padding: '1.5rem', color: C.muted, textAlign: 'center' }}>No events found.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '1rem' }}>
        <button onClick={goPrev} disabled={cursors.length === 0 || loading} style={pagerStyle(cursors.length === 0 || loading)}>← Newer</button>
        <button onClick={goNext} disabled={!nextCursor || loading} style={pagerStyle(!nextCursor || loading)}>Older →</button>
      </div>
    </Shell>
  )
}

function pagerStyle(disabled: boolean): React.CSSProperties {
  return {
    background: 'transparent', border: `1px solid ${C.border}`, color: disabled ? C.border : C.text,
    borderRadius: 8, padding: '0.4rem 0.9rem', fontSize: 13, fontWeight: 600,
    cursor: disabled ? 'default' : 'pointer',
  }
}
