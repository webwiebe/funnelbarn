import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import Shell from '../components/shell/Shell'
import { api, DashboardData } from '../lib/api'
import { useProjects } from '../lib/projects'
import { trackEvent } from '../lib/analytics'
import { reportError } from '../lib/bugbarn'
import { C } from '../lib/theme'
import { useTopFunnelRate } from '../lib/useTopFunnelRate'
import {
  CreateProjectModal, EventsChartCard, NoProjectState, ProjectGrid, RangeTabs, ReferrerCard,
  StatCards, TopPagesCard, buildChartData, buildReferrerData, calcTrend,
} from '../components/dashboard/DashboardParts'

export default function Dashboard() {
  const { projectId } = useParams<{ projectId?: string }>()
  const navigate = useNavigate()
  const { projects, refetch: refetchProjects, selectedEnvironment } = useProjects()
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [range, setRange] = useState('7d')
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
      trackEvent('project_created', { project_id: p.id, project_name: p.name })
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
    api.getDashboard(projectId, range, selectedEnvironment)
      .then(setData)
      .catch((e: unknown) => { reportError(e, { source: 'Dashboard.getDashboard' }); setError(String(e)) })
      .finally(() => setLoading(false))
  }, [projectId, range, selectedEnvironment])

  const topFunnelRate = useTopFunnelRate(projectId, selectedEnvironment)

  const chartData = buildChartData(data, range)
  const referrerData = buildReferrerData(data)

  // Compute real trends from time-series data (first vs last half of period)
  const eventsTrend = calcTrend(data?.events_time_series)
  const sessionsTrend = calcTrend(
    (data as (DashboardData & { sessions_time_series?: { time: string; count: number }[] }) | null)?.sessions_time_series
  )

  const createModal = (
    <CreateProjectModal open={showCreateProject} onClose={() => setShowCreateProject(false)} onCreate={handleCreateProject} name={newProjectName} setName={setNewProjectName} creating={creatingProject} error={createError} />
  )

  if (!projectId) {
    if (projects.length > 0) {
      return (
        <>
          <Shell>
            <ProjectGrid projects={projects} onOpen={(id) => navigate(`/dashboard/${id}`)} onNew={() => setShowCreateProject(true)} />
          </Shell>
          {createModal}
        </>
      )
    }

    return (
      <>
        <Shell>
          <NoProjectState onCreate={() => setShowCreateProject(true)} />
        </Shell>
        {createModal}
      </>
    )
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <style>{`
        @media (max-width: 767px) {
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
        <RangeTabs range={range} onChange={setRange} />
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
      <StatCards data={data} loading={loading} topFunnelRate={topFunnelRate} eventsTrend={eventsTrend} sessionsTrend={sessionsTrend} />

      {/* Events chart */}
      <EventsChartCard loading={loading} chartData={chartData} />

      {/* Bottom row: Top pages + Referrers */}
      <div className="bottom-row-grid" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem' }}>
        {/* Top pages */}
        <TopPagesCard loading={loading} pages={data?.top_pages} />

        {/* Referrer donut */}
        <ReferrerCard loading={loading} referrerData={referrerData} />
      </div>

      {createModal}
    </Shell>
  )
}
