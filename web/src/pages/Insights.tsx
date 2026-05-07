import { useEffect, useState, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import { Plus, Trash2, Lightbulb, X, Columns2, Columns3, Square } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api } from '../lib/api'
import type { DashboardWidget, PropertyBreakdown } from '../lib/api'
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
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '1rem' }}>
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

function isUrl(value: string): boolean {
  return /^https?:\/\//.test(value)
}

function BreakdownValue({ value }: { value: string }) {
  if (isUrl(value)) {
    return (
      <a
        href={value}
        target="_blank"
        rel="noopener noreferrer"
        style={{
          color: C.amber,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          maxWidth: '70%',
          textDecoration: 'none',
        }}
        title={value}
        onMouseOver={(e) => (e.currentTarget.style.textDecoration = 'underline')}
        onMouseOut={(e) => (e.currentTarget.style.textDecoration = 'none')}
      >
        {value}
      </a>
    )
  }
  return (
    <span style={{
      color: C.text,
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap',
      maxWidth: '70%',
    }} title={value}>
      {value}
    </span>
  )
}

const sizeIcons = { 1: Square, 2: Columns2, 3: Columns3 } as const
const sizeLabels = { 1: 'Small (1 col)', 2: 'Medium (2 cols)', 3: 'Wide (3 cols)' } as const

function WidgetCard({ widget, breakdown, onDelete, onResize }: {
  widget: DashboardWidget
  breakdown: PropertyBreakdown[]
  onDelete: () => void
  onResize: () => void
}) {
  const isCountOnly = !widget.property
  const maxCount = breakdown.length > 0 ? breakdown[0].count : 1
  const total = breakdown.reduce((sum, r) => sum + r.count, 0)
  const size = (widget.size || 1) as 1 | 2 | 3
  const nextSize = size >= 3 ? 1 : (size + 1) as 1 | 2 | 3
  const SizeIcon = sizeIcons[nextSize]

  const subtitle = widget.property
    ? `${widget.event_name} → ${widget.property}`
    : widget.event_name

  return (
    <div style={{
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
      overflow: 'hidden',
      gridColumn: `span ${size}`,
    }}>
      <div style={{
        padding: '1rem 1.25rem',
        borderBottom: `1px solid ${C.border}`,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}>
        <div>
          <div style={{ fontSize: 14, fontWeight: 700, color: C.text }}>
            {widget.title || subtitle}
          </div>
          {widget.title && (
            <div style={{ fontSize: 12, color: C.muted, marginTop: 2 }}>
              {subtitle}
            </div>
          )}
        </div>
        <div style={{ display: 'flex', gap: 4 }}>
          <button
            onClick={onResize}
            style={{
              background: 'transparent',
              border: 'none',
              color: C.muted,
              cursor: 'pointer',
              padding: 4,
              borderRadius: 4,
              display: 'flex',
            }}
            title={sizeLabels[nextSize]}
          >
            <SizeIcon size={14} />
          </button>
          <button
            onClick={onDelete}
            style={{
              background: 'transparent',
              border: 'none',
              color: C.muted,
              cursor: 'pointer',
              padding: 4,
              borderRadius: 4,
              display: 'flex',
            }}
            title="Delete widget"
          >
            <Trash2 size={14} />
          </button>
        </div>
      </div>

      <div style={{ padding: '1rem 1.25rem' }}>
        {breakdown.length === 0 ? (
          <div style={{ color: C.muted, fontSize: 13, textAlign: 'center', padding: '1rem 0' }}>
            No data yet
          </div>
        ) : isCountOnly ? (
          <div style={{ textAlign: 'center', padding: '1rem 0' }}>
            <div style={{ fontSize: 36, fontWeight: 800, color: C.amber }}>{breakdown[0].count.toLocaleString()}</div>
            <div style={{ fontSize: 13, color: C.muted, marginTop: 4 }}>events</div>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {breakdown.map((row) => (
              <div key={row.value}>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 4 }}>
                  <BreakdownValue value={row.value} />
                  <span style={{ color: C.muted, flexShrink: 0, marginLeft: 8 }}>
                    {row.count} <span style={{ fontSize: 11 }}>({total > 0 ? Math.round(row.count / total * 100) : 0}%)</span>
                  </span>
                </div>
                <div style={{
                  height: 6,
                  background: C.bg,
                  borderRadius: 3,
                  overflow: 'hidden',
                }}>
                  <div style={{
                    height: '100%',
                    width: `${(row.count / maxCount) * 100}%`,
                    background: C.amber,
                    borderRadius: 3,
                    transition: 'width 0.3s ease',
                  }} />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div style={{
        padding: '0.5rem 1.25rem',
        borderTop: `1px solid ${C.border}`,
        fontSize: 11,
        color: C.muted,
      }}>
        Rolling window: last 100 events
      </div>
    </div>
  )
}

function AddWidgetModal({ projectId, onClose, onAdded }: {
  projectId: string
  onClose: () => void
  onAdded: () => void
}) {
  const [eventNames, setEventNames] = useState<string[]>([])
  const [properties, setProperties] = useState<string[]>([])
  const [selectedEvent, setSelectedEvent] = useState('')
  const [selectedProperty, setSelectedProperty] = useState('')
  const [title, setTitle] = useState('')
  const [saving, setSaving] = useState(false)
  const [loadingEvents, setLoadingEvents] = useState(true)
  const [loadingProps, setLoadingProps] = useState(false)

  useEffect(() => {
    api.getEventNames(projectId)
      .then((d) => setEventNames(d.event_names))
      .catch(() => {})
      .finally(() => setLoadingEvents(false))
  }, [projectId])

  useEffect(() => {
    if (!selectedEvent) {
      setProperties([])
      return
    }
    setLoadingProps(true)
    setSelectedProperty('')
    api.getEventProperties(projectId, selectedEvent)
      .then((d) => setProperties(d.properties))
      .catch(() => {})
      .finally(() => setLoadingProps(false))
  }, [projectId, selectedEvent])

  const handleSave = async () => {
    if (!selectedEvent) return
    setSaving(true)
    try {
      const defaultTitle = selectedProperty
        ? `${selectedEvent} → ${selectedProperty}`
        : `${selectedEvent} (count)`
      await api.createWidget(projectId, {
        event_name: selectedEvent,
        property: selectedProperty,
        title: title || defaultTitle,
      })
      onAdded()
    } catch {
      // silent
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <div
        onClick={onClose}
        style={{
          position: 'fixed',
          inset: 0,
          background: 'rgba(0,0,0,0.6)',
          zIndex: 1000,
        }}
      />
      <div style={{
        position: 'fixed',
        top: '50%',
        left: '50%',
        transform: 'translate(-50%, -50%)',
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 16,
        width: '90%',
        maxWidth: 480,
        zIndex: 1001,
        overflow: 'hidden',
      }}>
        <div style={{
          padding: '1.25rem 1.5rem',
          borderBottom: `1px solid ${C.border}`,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>Add Widget</h2>
          <button
            onClick={onClose}
            style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer', padding: 4 }}
          >
            <X size={18} />
          </button>
        </div>

        <div style={{ padding: '1.5rem' }}>
          <label style={{ display: 'block', fontSize: 13, fontWeight: 600, color: C.muted, marginBottom: 6 }}>
            Event Name
          </label>
          {loadingEvents ? (
            <Skeleton height={40} />
          ) : (
            <select
              value={selectedEvent}
              onChange={(e) => setSelectedEvent(e.target.value)}
              style={{
                width: '100%',
                padding: '0.6rem 0.75rem',
                background: C.bg,
                border: `1px solid ${C.border}`,
                borderRadius: 8,
                color: C.text,
                fontSize: 14,
                boxSizing: 'border-box',
              }}
            >
              <option value="">Select an event...</option>
              {eventNames.map((n) => (
                <option key={n} value={n}>{n}</option>
              ))}
            </select>
          )}

          <label style={{ display: 'block', fontSize: 13, fontWeight: 600, color: C.muted, marginBottom: 6, marginTop: 16 }}>
            Property (optional)
          </label>
          {loadingProps ? (
            <Skeleton height={40} />
          ) : (
            <select
              value={selectedProperty}
              onChange={(e) => setSelectedProperty(e.target.value)}
              disabled={!selectedEvent}
              style={{
                width: '100%',
                padding: '0.6rem 0.75rem',
                background: C.bg,
                border: `1px solid ${C.border}`,
                borderRadius: 8,
                color: selectedEvent ? C.text : C.muted,
                fontSize: 14,
                boxSizing: 'border-box',
              }}
            >
              <option value="">{selectedEvent ? 'None (total count)' : 'Select an event first'}</option>
              {properties.map((p) => (
                <option key={p} value={p}>{p}</option>
              ))}
            </select>
          )}

          <label style={{ display: 'block', fontSize: 13, fontWeight: 600, color: C.muted, marginBottom: 6, marginTop: 16 }}>
            Title (optional)
          </label>
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={selectedEvent ? (selectedProperty ? `${selectedEvent} → ${selectedProperty}` : `${selectedEvent} (count)`) : 'Widget title'}
            style={{
              width: '100%',
              padding: '0.6rem 0.75rem',
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              color: C.text,
              fontSize: 14,
              boxSizing: 'border-box',
            }}
          />
        </div>

        <div style={{
          padding: '1rem 1.5rem',
          borderTop: `1px solid ${C.border}`,
          display: 'flex',
          justifyContent: 'flex-end',
          gap: 8,
        }}>
          <button
            onClick={onClose}
            style={{
              padding: '0.5rem 1rem',
              background: 'transparent',
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              color: C.text,
              fontSize: 13,
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={!selectedEvent || saving}
            style={{
              padding: '0.5rem 1rem',
              background: !selectedEvent || saving ? '#4a4d5a' : C.amber,
              border: 'none',
              borderRadius: 8,
              color: !selectedEvent || saving ? C.muted : '#000',
              fontSize: 13,
              fontWeight: 700,
              cursor: !selectedEvent || saving ? 'default' : 'pointer',
            }}
          >
            {saving ? 'Adding...' : 'Add Widget'}
          </button>
        </div>
      </div>
    </>
  )
}
