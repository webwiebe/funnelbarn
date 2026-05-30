import { useEffect, useState, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import { Plus, Lightbulb } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api } from '../lib/api'
import type { DashboardWidget, PropertyBreakdown, DistributionEntry } from '../lib/api'
import { useProjects } from '../lib/projects'
import { Skeleton } from '../components/ui/Skeleton'
import { C } from '../lib/theme'
import { WidgetCard } from '../components/insights/WidgetCard'
import { AddWidgetModal } from '../components/insights/AddWidgetModal'
import { VisitorBreakdown } from '../components/insights/VisitorBreakdown'

interface WidgetWithData {
  widget: DashboardWidget
  breakdown: PropertyBreakdown[]
}

export default function Insights() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const [widgets, setWidgets] = useState<WidgetWithData[]>([])
  const [loading, setLoading] = useState(true)
  const [showAddModal, setShowAddModal] = useState(false)
  const [distributions, setDistributions] = useState<Record<string, DistributionEntry[]>>({})

  const fetchAll = useCallback(async () => {
    if (!projectId) return
    try {
      const data = await api.getBatchBreakdowns(projectId)
      setWidgets(data.results)
    } catch {
      // silent
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    fetchAll()
  }, [fetchAll])

  useEffect(() => {
    if (!projectId) return
    api.getSessionDistributions(projectId)
      .then((d) => setDistributions(d.distributions ?? {}))
      .catch(() => {})
  }, [projectId])

  const handleDelete = async (widgetId: string) => {
    if (!projectId) return
    try {
      await api.deleteWidget(projectId, widgetId)
      setWidgets((prev) => prev.filter((w) => w.widget.id !== widgetId))
    } catch {
      // silent
    }
  }

  const handleResize = async (widgetId: string) => {
    if (!projectId) return
    const entry = widgets.find((w) => w.widget.id === widgetId)
    if (!entry) return
    const currentSize = entry.widget.size || 1
    const nextSize = currentSize >= 3 ? 1 : currentSize + 1
    try {
      const updated = await api.updateWidget(projectId, widgetId, {
        event_name: entry.widget.event_name,
        property: entry.widget.property,
        title: entry.widget.title,
        position: entry.widget.position,
        size: nextSize,
      })
      setWidgets((prev) =>
        prev.map((w) => w.widget.id === widgetId ? { ...w, widget: updated } : w)
      )
    } catch {
      // silent
    }
  }

  const handleAdded = () => {
    setShowAddModal(false)
    setLoading(true)
    fetchAll()
  }

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, textAlign: 'center', padding: '2rem' }}>
          Select a project to view insights.
        </div>
      </Shell>
    )
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Insights</h1>
        <button
          onClick={() => setShowAddModal(true)}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            background: C.amber,
            color: '#000',
            border: 'none',
            borderRadius: 8,
            padding: '0.5rem 1rem',
            fontSize: 13,
            fontWeight: 700,
            cursor: 'pointer',
          }}
        >
          <Plus size={16} />
          Add widget
        </button>
      </div>

      {loading ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))', gap: '1rem' }}>
          {[1, 2, 3].map((i) => (
            <div key={i} style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, padding: '1.5rem' }}>
              <Skeleton height={20} width={160} />
              <div style={{ marginTop: 16 }}>
                <Skeleton height={24} />
                <Skeleton height={24} />
                <Skeleton height={24} />
              </div>
            </div>
          ))}
        </div>
      ) : widgets.length === 0 ? (
        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '3rem',
          textAlign: 'center',
          color: C.muted,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          gap: 12,
        }}>
          <Lightbulb size={40} opacity={0.3} />
          <div style={{ fontSize: 16, fontWeight: 600, color: C.text }}>No widgets yet</div>
          <div style={{ fontSize: 14, maxWidth: 400 }}>
            Add a widget to see top-N breakdowns of your event properties.
            For example: which pages are visited most, or which form fields get the most interaction.
          </div>
          <button
            onClick={() => setShowAddModal(true)}
            style={{
              marginTop: 8,
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              background: C.amber,
              color: '#000',
              border: 'none',
              borderRadius: 8,
              padding: '0.5rem 1.25rem',
              fontSize: 13,
              fontWeight: 700,
              cursor: 'pointer',
            }}
          >
            <Plus size={16} />
            Add your first widget
          </button>
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 300px), 1fr))', gap: '1rem' }}>
          {widgets.map(({ widget, breakdown }) => (
            <WidgetCard
              key={widget.id}
              widget={widget}
              breakdown={breakdown}
              onDelete={() => handleDelete(widget.id)}
              onResize={() => handleResize(widget.id)}
            />
          ))}
        </div>
      )}

      <VisitorBreakdown distributions={distributions} />

      {showAddModal && projectId && (
        <AddWidgetModal
          projectId={projectId}
          onClose={() => setShowAddModal(false)}
          onAdded={handleAdded}
        />
      )}
    </Shell>
  )
}
