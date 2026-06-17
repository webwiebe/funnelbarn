import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import { api } from '../../lib/api'
import { Skeleton } from '../ui/Skeleton'
import { C } from '../../lib/theme'

export function AddWidgetModal({ projectId, onClose, onAdded }: {
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
      .then((d) => setEventNames(d.event_names ?? []))
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
      .then((d) => setProperties(d.properties ?? []))
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
