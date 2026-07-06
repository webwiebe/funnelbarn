import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from 'recharts'
import { Activity, Users, Layers, ArrowUpRight } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api, OverviewData } from '../lib/api'
import { useProjects } from '../lib/projects'
import { OverviewTabs } from '../components/OverviewTabs'
import { C } from '../lib/theme'

const LINE_COLORS = ['#f59e0b', '#10b981', '#6366f1', '#ec4899', '#3b82f6', '#8b5cf6', '#14b8a6', '#f43f5e']

const RANGES = [
  { label: '24h', value: '24h' },
  { label: '7d', value: '7d' },
  { label: '30d', value: '30d' },
]

function Tile({ label, value, icon }: { label: string; value: string; icon: React.ReactNode }) {
  return (
    <div style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, padding: '1rem 1.25rem', flex: 1, minWidth: 160 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: C.muted, fontSize: 13 }}>
        {icon}{label}
      </div>
      <div style={{ fontSize: 28, fontWeight: 700, color: C.text, marginTop: 6 }}>{value}</div>
    </div>
  )
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, padding: '1.25rem' }}>
      <div style={{ fontSize: 14, fontWeight: 600, color: C.text, marginBottom: '0.75rem' }}>{title}</div>
      {children}
    </div>
  )
}

export default function Overview() {
  const { projects, selectedEnvironment } = useProjects()
  const [data, setData] = useState<OverviewData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [range, setRange] = useState('7d')

  const nameFor = useMemo(() => {
    const m = new Map(projects.map((p) => [p.id, p.name]))
    return (id: string) => m.get(id) ?? id
  }, [projects])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    api.getOverview(range, selectedEnvironment || '', 'device_type')
      .then((d) => { if (!cancelled) setData(d) })
      .catch((e) => { if (!cancelled) setError(e.message || 'failed to load') })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [range, selectedEnvironment])

  // Pivot visitors_by_project into per-day rows keyed by project id for the
  // multi-line chart. Only the top projects by total events get a line to keep
  // it readable.
  const { chartRows, chartProjectIds } = useMemo(() => {
    if (!data) return { chartRows: [], chartProjectIds: [] as string[] }
    const topIds = (data.projects ?? []).slice(0, 6).map((p) => p.project_id)
    const byDay = new Map<string, Record<string, number | string>>()
    for (const row of data.visitors_by_project ?? []) {
      if (!topIds.includes(row.project_id)) continue
      const r = byDay.get(row.day) ?? { day: row.day }
      r[row.project_id] = row.count
      byDay.set(row.day, r)
    }
    const rows = Array.from(byDay.values()).sort((a, b) => String(a.day).localeCompare(String(b.day)))
    return { chartRows: rows, chartProjectIds: topIds }
  }, [data])

  return (
    <Shell>
      <OverviewTabs />
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.25rem', flexWrap: 'wrap', gap: 12 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: C.text, margin: 0 }}>All Projects</h1>
          <div style={{ color: C.muted, fontSize: 13, marginTop: 4 }}>Instance-wide analytics across every site</div>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          {RANGES.map((r) => (
            <button
              key={r.value}
              onClick={() => setRange(r.value)}
              style={{
                background: range === r.value ? C.amber : 'transparent',
                color: range === r.value ? '#1a1d27' : C.muted,
                border: `1px solid ${range === r.value ? C.amber : C.border}`,
                borderRadius: 8, padding: '0.35rem 0.75rem', fontSize: 13, fontWeight: 600, cursor: 'pointer',
              }}
            >{r.label}</button>
          ))}
        </div>
      </div>

      {error && (
        <div style={{ background: 'rgba(239,68,68,0.1)', border: `1px solid ${C.error}`, color: C.error, borderRadius: 8, padding: '0.75rem 1rem', marginBottom: '1rem' }}>{error}</div>
      )}

      {/* KPI tiles */}
      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', marginBottom: '1.25rem' }}>
        <Tile label="Total events" value={loading ? '…' : (data?.total_events ?? 0).toLocaleString()} icon={<Activity size={15} />} />
        <Tile label="Unique visitors" value={loading ? '…' : (data?.unique_sessions ?? 0).toLocaleString()} icon={<Users size={15} />} />
        <Tile label="Active sites" value={loading ? '…' : String((data?.projects ?? []).length)} icon={<Layers size={15} />} />
      </div>

      {/* Visitors per site */}
      <div style={{ marginBottom: '1.25rem' }}>
        <Card title="Visitors per site">
          <div style={{ height: 260 }}>
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartRows} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={C.border} />
                <XAxis dataKey="day" stroke={C.muted} fontSize={11} />
                <YAxis stroke={C.muted} fontSize={11} allowDecimals={false} />
                <Tooltip contentStyle={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 8, color: C.text }} />
                <Legend formatter={(v) => nameFor(String(v))} />
                {chartProjectIds.map((pid, i) => (
                  <Line key={pid} type="monotone" dataKey={pid} name={pid} stroke={LINE_COLORS[i % LINE_COLORS.length]} dot={false} strokeWidth={2} />
                ))}
              </LineChart>
            </ResponsiveContainer>
          </div>
        </Card>
      </div>

      {/* Per-project performance table */}
      <div style={{ marginBottom: '1.25rem' }}>
        <Card title="Per-site performance">
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
              <thead>
                <tr style={{ color: C.muted, textAlign: 'left' }}>
                  <th style={{ padding: '0.4rem 0.5rem' }}>Site</th>
                  <th style={{ padding: '0.4rem 0.5rem', textAlign: 'right' }}>Events</th>
                  <th style={{ padding: '0.4rem 0.5rem', textAlign: 'right' }}>Visitors</th>
                  <th style={{ padding: '0.4rem 0.5rem', width: 32 }}></th>
                </tr>
              </thead>
              <tbody>
                {(data?.projects ?? []).map((p) => (
                  <tr key={p.project_id} style={{ borderTop: `1px solid ${C.border}`, color: C.text }}>
                    <td style={{ padding: '0.5rem' }}>{nameFor(p.project_id)}</td>
                    <td style={{ padding: '0.5rem', textAlign: 'right' }}>{p.events.toLocaleString()}</td>
                    <td style={{ padding: '0.5rem', textAlign: 'right' }}>{p.unique_sessions.toLocaleString()}</td>
                    <td style={{ padding: '0.5rem', textAlign: 'right' }}>
                      <Link to={`/dashboard/${p.project_id}`} style={{ color: C.amber, display: 'inline-flex' }} title="Open site dashboard">
                        <ArrowUpRight size={16} />
                      </Link>
                    </td>
                  </tr>
                ))}
                {!loading && (data?.projects ?? []).length === 0 && (
                  <tr><td colSpan={4} style={{ padding: '1rem', color: C.muted, textAlign: 'center' }}>No events in this range.</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </Card>
      </div>

      {/* Breakdown row */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 12 }}>
        <Card title="Top pages">
          <RankList rows={(data?.top_pages ?? []).map((r) => ({ label: r.url, sub: nameFor(r.project_id), value: r.views }))} />
        </Card>
        <Card title="Top referrers">
          <RankList rows={(data?.top_referrers ?? []).map((r) => ({ label: r.domain, sub: nameFor(r.project_id), value: r.visits }))} />
        </Card>
        <Card title="Top countries">
          <RankList rows={(data?.top_countries ?? []).map((r) => ({ label: r.country_code, sub: nameFor(r.project_id), value: r.count }))} />
        </Card>
        <Card title="By device">
          <RankList rows={(data?.dimension_breakdown ?? []).map((r) => ({ label: r.value, value: r.count }))} />
        </Card>
      </div>
    </Shell>
  )
}

function RankList({ rows }: { rows: { label: string; sub?: string; value: number }[] }) {
  if (rows.length === 0) return <div style={{ color: C.muted, fontSize: 13 }}>No data.</div>
  const max = Math.max(...rows.map((r) => r.value), 1)
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {rows.slice(0, 8).map((r, i) => (
        <div key={i}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, fontSize: 13, color: C.text }}>
            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {r.label || '(none)'}{r.sub ? <span style={{ color: C.muted }}> · {r.sub}</span> : null}
            </span>
            <span style={{ color: C.muted, flexShrink: 0 }}>{r.value.toLocaleString()}</span>
          </div>
          <div style={{ height: 4, background: C.border, borderRadius: 2, marginTop: 3 }}>
            <div style={{ width: `${(r.value / max) * 100}%`, height: '100%', background: C.amber, borderRadius: 2 }} />
          </div>
        </div>
      ))}
    </div>
  )
}
