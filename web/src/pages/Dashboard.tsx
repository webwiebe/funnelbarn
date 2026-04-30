import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  PieChart, Pie, Cell, Legend,
} from 'recharts'
import { Activity, Users, TrendingDown, MousePointer, ArrowUpRight, ArrowDownRight, X, Plus } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api, DashboardData } from '../lib/api'
import { useProjects } from '../lib/projects'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  success: '#10b981',
  error: '#ef4444',
}

const PIE_COLORS = ['#f59e0b', '#10b981', '#6366f1', '#ec4899', '#3b82f6', '#8b5cf6']

const RANGES = [
  { label: '24h', value: '24h' },
  { label: '7d', value: '7d' },
  { label: '30d', value: '30d' },
]

function Skeleton({ width = '100%', height = 20 }: { width?: string | number; height?: number | string }) {
  return (
    <div style={{
      width,
      height,
      background: 'linear-gradient(90deg, #1a1d27 25%, #22263a 50%, #1a1d27 75%)',
      backgroundSize: '200% 100%',
      animation: 'shimmer 1.5s infinite',
      borderRadius: 6,
    }} />
  )
}

function StatCard({
  label,
  value,
  icon,
  trend,
  loading,
}: {
  label: string
  value: string
  icon: React.ReactNode
  trend?: number
  loading?: boolean
}) {
  return (
    <div style={{
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
      padding: '1.25rem 1.5rem',
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 12 }}>
        <div style={{ fontSize: 13, color: C.muted, fontWeight: 500 }}>{label}</div>
        <div style={{ color: C.amber, opacity: 0.8 }}>{icon}</div>
      </div>
      {loading ? (
        <Skeleton height={36} />
      ) : (
        <div style={{ display: 'flex', alignItems: 'flex-end', gap: 8 }}>
          <div style={{ fontSize: 32, fontWeight: 800, letterSpacing: '-1px', color: C.text }}>{value}</div>
          {trend !== undefined && (
            <div style={{
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              fontSize: 13,
              fontWeight: 600,
              color: trend >= 0 ? C.success : C.error,
              paddingBottom: 4,
            }}>
              {trend >= 0 ? <ArrowUpRight size={14} /> : <ArrowDownRight size={14} />}
              {Math.abs(trend).toFixed(1)}%
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export default function Dashboard() {
  const { projectId } = useParams<{ projectId?: string }>()
  const navigate = useNavigate()
  const { projects, refetch: refetchProjects } = useProjects()
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [range, setRange] = useState('7d')
  const [topFunnelRate, setTopFunnelRate] = useState<number | null>(null)
  const [showCreateProject, setShowCreateProject] = useState(false)
  const [newProjectName, setNewProjectName] = useState('')
  const [creatingProject, setCreatingProject] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const handleCreateProject = async () => {
    if (!newProjectName.trim()) return
    setCreatingProject(true)
    setCreateError(null)
    try {
      const p = await api.createProject(newProjectName, '')
      refetchProjects()
      setShowCreateProject(false)
      setNewProjectName('')
      navigate(`/dashboard/${p.id}`)
    } catch (e) {
      setCreateError(String(e))
    } finally {
      setCreatingProject(false)
    }
  }

  useEffect(() => {
    if (!projectId) return
    setLoading(true)
    setError(null)
    api.getDashboard(projectId, range)
      .then(setData)
      .catch((e: unknown) => setError(String(e)))
      .finally(() => setLoading(false))
  }, [projectId, range])

  useEffect(() => {
    if (!projectId) return
    setTopFunnelRate(null)
    api.listFunnels(projectId)
      .then(async (d) => {
        const funnels = d.funnels || []
        if (funnels.length === 0) return
        const analysis = await api.getFunnelAnalysis(projectId, funnels[0].id)
        const steps = analysis.results || []
        if (steps.length < 2) return
        const first = steps[0]?.count ?? 0
        const last = steps[steps.length - 1]?.count ?? 0
        if (first === 0) return
        setTopFunnelRate((last / first) * 100)
      })
      .catch(() => setTopFunnelRate(null))
  }, [projectId])

  // Format time series for chart
  const chartData = data?.events_time_series?.map((pt) => ({
    time: new Date(pt.Time).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
    events: pt.Count,
  })) ?? []

  // Referrer data for pie chart
  const referrerData = data?.top_referrers?.slice(0, 6).map((r) => ({
    name: r.Domain || 'Direct',
    value: r.Visits,
  })) ?? []

  // Max views for inline bar
  const maxViews = data?.top_pages ? Math.max(...data.top_pages.map((p) => p.Views)) : 1

  // Compute real trends from time-series data (first vs last half of period)
  const eventsTrend = (() => {
    const ts = data?.events_time_series
    if (!ts || ts.length < 2) return undefined
    const mid = Math.floor(ts.length / 2)
    const prev = ts.slice(0, mid).reduce((s, p) => s + p.Count, 0)
    const curr = ts.slice(mid).reduce((s, p) => s + p.Count, 0)
    if (prev === 0) return undefined
    return ((curr - prev) / prev) * 100
  })()

  const sessionsTrend = (() => {
    const ts = (data as (DashboardData & { sessions_time_series?: { Time: string; Count: number }[] }) | null)?.sessions_time_series
    if (!ts || ts.length < 2) return undefined
    const mid = Math.floor(ts.length / 2)
    const prev = ts.slice(0, mid).reduce((s, p) => s + p.Count, 0)
    const curr = ts.slice(mid).reduce((s, p) => s + p.Count, 0)
    if (prev === 0) return undefined
    return ((curr - prev) / prev) * 100
  })()

  const createModal = showCreateProject ? (
    <div style={{
      position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      padding: 16, zIndex: 1000,
    }}>
      <div style={{
        background: C.surface, borderRadius: 16, padding: '2rem',
        width: '100%', maxWidth: 400, border: `1px solid ${C.border}`,
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
          <h2 style={{ margin: 0, fontSize: 20, color: C.text }}>New Project</h2>
          <button onClick={() => setShowCreateProject(false)}
            style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer', padding: 4 }}>
            <X size={20} />
          </button>
        </div>
        <input
          autoFocus
          placeholder="My Awesome App"
          value={newProjectName}
          onChange={(e) => setNewProjectName(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleCreateProject()}
          style={{
            width: '100%', boxSizing: 'border-box',
            background: C.bg, border: `1px solid ${C.border}`,
            borderRadius: 8, padding: '0.65rem 0.875rem',
            color: C.text, fontSize: 15, marginBottom: 12,
          }}
        />
        {createError && <div style={{ color: C.error, fontSize: 13, marginBottom: 10 }}>{createError}</div>}
        <button
          onClick={handleCreateProject}
          disabled={creatingProject || !newProjectName.trim()}
          style={{
            width: '100%', background: creatingProject ? '#78481a' : C.amber,
            border: 'none', borderRadius: 8, color: '#0f1117',
            padding: '0.7rem', fontWeight: 700, fontSize: 15,
            cursor: creatingProject ? 'wait' : 'pointer',
          }}
        >
          {creatingProject ? 'Creating…' : 'Create Project'}
        </button>
      </div>
    </div>
  ) : null

  if (!projectId) {
    if (projects.length > 0) {
      return (
        <>
          <Shell>
            <div style={{ maxWidth: 900, margin: '0 auto' }}>
              <div style={{
                display: 'flex', justifyContent: 'space-between',
                alignItems: 'center', marginBottom: '1.5rem',
              }}>
                <h1 style={{ fontSize: 24, fontWeight: 800, margin: 0 }}>Your Projects</h1>
                <button onClick={() => setShowCreateProject(true)} style={{
                  background: C.amber, border: 'none', borderRadius: 8,
                  color: '#0f1117', padding: '0.5rem 1rem',
                  fontWeight: 700, fontSize: 14, cursor: 'pointer',
                  display: 'flex', alignItems: 'center', gap: 6,
                }}>
                  <Plus size={16} /> New Project
                </button>
              </div>
              <div style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))',
                gap: 16,
              }}>
                {projects.map((p) => (
                  <div
                    key={p.id}
                    onClick={() => navigate(`/dashboard/${p.id}`)}
                    style={{
                      background: C.surface,
                      border: `1px solid ${C.border}`,
                      borderRadius: 12,
                      padding: '1.25rem',
                      cursor: 'pointer',
                      transition: 'border-color 0.15s',
                    }}
                    onMouseEnter={(e) => (e.currentTarget.style.borderColor = C.amber)}
                    onMouseLeave={(e) => (e.currentTarget.style.borderColor = C.border)}
                  >
                    <div style={{ fontSize: 18, fontWeight: 700, marginBottom: 8 }}>{p.name}</div>
                    <div style={{ fontSize: 12, color: C.muted }}>
                      {p.domain || 'No domain set'}
                    </div>
                    <div style={{
                      marginTop: 16,
                      display: 'flex',
                      alignItems: 'center',
                      gap: 6,
                      fontSize: 13,
                      color: C.amber,
                      fontWeight: 600,
                    }}>
                      View dashboard →
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </Shell>
          {createModal}
        </>
      )
    }

    return (
      <>
        <Shell>
          <div style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            minHeight: 400,
            gap: 16,
            color: C.muted,
          }}>
            <Activity size={48} opacity={0.3} />
            <div style={{ fontSize: 20, fontWeight: 700, color: C.text }}>No project selected</div>
            <div>Select a project from the dropdown above to view its dashboard.</div>
            <button
              onClick={() => setShowCreateProject(true)}
              style={{
                background: C.amber,
                color: '#0f1117',
                border: 'none',
                borderRadius: 8,
                padding: '0.6rem 1.25rem',
                fontWeight: 700,
                cursor: 'pointer',
                marginTop: 8,
              }}
            >
              Create a project
            </button>
          </div>
        </Shell>
        {createModal}
      </>
    )
  }

  return (
    <Shell projectId={projectId}>
      <style>{`
        @keyframes shimmer {
          0% { background-position: 200% 0; }
          100% { background-position: -200% 0; }
        }
        @media (max-width: 640px) {
          .stat-cards-grid { grid-template-columns: 1fr 1fr !important; }
          .bottom-row-grid { grid-template-columns: 1fr !important; }
        }
        @media (max-width: 400px) {
          .stat-cards-grid { grid-template-columns: 1fr !important; }
        }
      `}</style>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Overview</h1>
        <div style={{ display: 'flex', gap: 4, background: C.surface, border: `1px solid ${C.border}`, borderRadius: 8, padding: 4 }}>
          {RANGES.map((r) => (
            <button
              key={r.value}
              onClick={() => setRange(r.value)}
              style={{
                background: range === r.value ? C.amber : 'transparent',
                color: range === r.value ? '#0f1117' : C.muted,
                border: 'none',
                borderRadius: 5,
                padding: '0.35rem 0.85rem',
                cursor: 'pointer',
                fontSize: 13,
                fontWeight: 600,
                transition: 'all 0.15s',
              }}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>

      {error && (
        <div style={{
          background: 'rgba(239,68,68,0.1)',
          border: `1px solid rgba(239,68,68,0.3)`,
          borderRadius: 8,
          padding: '0.75rem 1rem',
          color: C.error,
          marginBottom: '1.5rem',
        }}>
          {error}
        </div>
      )}

      {/* Stat cards */}
      <div className="stat-cards-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '1rem', marginBottom: '1.5rem' }}>
        <StatCard
          label="Total Events"
          value={loading ? '—' : (data?.total_events ?? 0).toLocaleString()}
          icon={<Activity size={18} />}
          trend={eventsTrend}
          loading={loading}
        />
        <StatCard
          label="Unique Sessions"
          value={loading ? '—' : (data?.unique_sessions ?? 0).toLocaleString()}
          icon={<Users size={18} />}
          trend={sessionsTrend}
          loading={loading}
        />
        <StatCard
          label="Top Funnel"
          value={loading ? '—' : topFunnelRate !== null ? `${topFunnelRate.toFixed(1)}%` : '—'}
          icon={<MousePointer size={18} />}
          loading={loading}
        />
        <StatCard
          label="Bounce Rate"
          value={loading ? '—' : `${((data?.bounce_rate ?? 0) * 100).toFixed(1)}%`}
          icon={<TrendingDown size={18} />}
          loading={loading}
        />
      </div>

      {/* Events chart */}
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 12,
        padding: '1.5rem',
        marginBottom: '1.5rem',
      }}>
        <div style={{ fontSize: 15, fontWeight: 700, marginBottom: '1rem' }}>Events over time</div>
        <div style={{ height: 'clamp(160px, 40vw, 240px)' }}>
        {loading ? (
          <Skeleton height="100%" />
        ) : chartData.length === 0 ? (
          <div style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: C.muted }}>
            No data for this period
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData} margin={{ top: 4, right: 4, left: -20, bottom: 0 }}>
              <defs>
                <linearGradient id="amberGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#f59e0b" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#f59e0b" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="#2a2d3a" />
              <XAxis dataKey="time" tick={{ fill: '#94a3b8', fontSize: 12 }} axisLine={false} tickLine={false} />
              <YAxis tick={{ fill: '#94a3b8', fontSize: 12 }} axisLine={false} tickLine={false} />
              <Tooltip
                contentStyle={{ background: '#1a1d27', border: '1px solid #2a2d3a', borderRadius: 8 }}
                labelStyle={{ color: '#e2e8f0' }}
                itemStyle={{ color: '#f59e0b' }}
              />
              <Area
                type="monotone"
                dataKey="events"
                stroke="#f59e0b"
                strokeWidth={2}
                fill="url(#amberGrad)"
                dot={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        )}
        </div>
      </div>

      {/* Bottom row: Top pages + Referrers */}
      <div className="bottom-row-grid" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem' }}>
        {/* Top pages */}
        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '1.5rem',
        }}>
          <div style={{ fontSize: 15, fontWeight: 700, marginBottom: '1rem' }}>Top pages</div>
          {loading ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {[1, 2, 3, 4].map((i) => <Skeleton key={i} height={36} />)}
            </div>
          ) : !data?.top_pages?.length ? (
            <div style={{ color: C.muted, fontSize: 14 }}>No page data</div>
          ) : (
            <div>
              {data.top_pages.slice(0, 8).map((p) => (
                <div key={p.URL} style={{ marginBottom: '0.75rem' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                    <div style={{
                      fontSize: 13,
                      color: C.text,
                      maxWidth: '70%',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}>
                      {p.URL}
                    </div>
                    <div style={{ fontSize: 13, fontWeight: 700, color: C.amber }}>{p.Views.toLocaleString()}</div>
                  </div>
                  <div style={{ height: 4, background: '#2a2d3a', borderRadius: 2 }}>
                    <div style={{
                      height: '100%',
                      width: `${(p.Views / maxViews) * 100}%`,
                      background: C.amber,
                      borderRadius: 2,
                    }} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Referrer donut */}
        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '1.5rem',
        }}>
          <div style={{ fontSize: 15, fontWeight: 700, marginBottom: '1rem' }}>Referrer breakdown</div>
          {loading ? (
            <Skeleton height={200} />
          ) : referrerData.length === 0 ? (
            <div style={{ color: C.muted, fontSize: 14 }}>No referrer data</div>
          ) : (
            <ResponsiveContainer width="100%" height={Math.min(220, window.innerWidth * 0.5) || 220}>
              <PieChart>
                <Pie
                  data={referrerData}
                  cx="50%"
                  cy="50%"
                  innerRadius={55}
                  outerRadius={85}
                  dataKey="value"
                  paddingAngle={2}
                >
                  {referrerData.map((_entry, index) => (
                    <Cell key={`cell-${index}`} fill={PIE_COLORS[index % PIE_COLORS.length]} />
                  ))}
                </Pie>
                <Legend
                  formatter={(value) => <span style={{ color: C.muted, fontSize: 12 }}>{value}</span>}
                />
                <Tooltip
                  contentStyle={{ background: '#1a1d27', border: '1px solid #2a2d3a', borderRadius: 8 }}
                  labelStyle={{ color: '#e2e8f0' }}
                />
              </PieChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>

      {createModal}
    </Shell>
  )
}
