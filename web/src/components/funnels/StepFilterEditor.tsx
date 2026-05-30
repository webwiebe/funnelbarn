import { useState, useRef, useEffect } from 'react'
import { Plus, X, ChevronDown, ChevronRight } from 'lucide-react'
import { api } from '../../lib/api'

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

export interface StepWithFilters {
  event_name: string
  filters: { property: string; value: string }[]
}

// ─── EventNameInput ───────────────────────────────────────────────────────────

function EventNameInput({
  value,
  onChange,
  placeholder,
  eventNames,
}: {
  value: string
  onChange: (val: string) => void
  placeholder?: string
  eventNames: string[]
}) {
  const [open, setOpen] = useState(false)
  const [focused, setFocused] = useState(false)
  const wrapperRef = useRef<HTMLDivElement>(null)

  const filtered = eventNames.filter(
    (n) => n.toLowerCase().includes(value.toLowerCase()) && n !== value
  )

  return (
    <div ref={wrapperRef} style={{ position: 'relative', flex: 1 }}>
      <input
        value={value}
        onChange={(e) => { onChange(e.target.value); setOpen(true) }}
        onFocus={(e) => { setFocused(true); setOpen(true); e.target.style.borderColor = C.amber }}
        onBlur={(e) => { setFocused(false); e.target.style.borderColor = C.border; setTimeout(() => setOpen(false), 150) }}
        placeholder={placeholder || 'Event name, e.g. page_view'}
        style={{
          width: '100%',
          background: C.bg,
          border: `1px solid ${C.border}`,
          borderRadius: 8,
          padding: '0.55rem 0.875rem',
          color: C.text,
          fontSize: 14,
          outline: 'none',
          boxSizing: 'border-box',
        }}
      />
      {open && focused && filtered.length > 0 && (
        <div style={{
          position: 'absolute',
          top: '100%',
          left: 0,
          right: 0,
          marginTop: 4,
          background: C.bg,
          border: `1px solid ${C.border}`,
          borderRadius: 8,
          maxHeight: 160,
          overflowY: 'auto',
          zIndex: 100,
        }}>
          {filtered.slice(0, 10).map((n) => (
            <div
              key={n}
              onMouseDown={(e) => { e.preventDefault(); onChange(n); setOpen(false) }}
              style={{
                padding: '0.45rem 0.875rem',
                fontSize: 13,
                color: C.text,
                cursor: 'pointer',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(245,158,11,0.1)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              {n}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── AutocompleteInput ────────────────────────────────────────────────────────

function AutocompleteInput({
  value,
  onChange,
  placeholder,
  suggestions,
  width,
}: {
  value: string
  onChange: (val: string) => void
  placeholder?: string
  suggestions: string[]
  width?: number
}) {
  const [open, setOpen] = useState(false)
  const [focused, setFocused] = useState(false)

  const filtered = suggestions.filter(
    (s) => s.toLowerCase().includes(value.toLowerCase()) && s !== value
  )

  return (
    <div style={{ position: 'relative' }}>
      <input
        value={value}
        onChange={(e) => { onChange(e.target.value); setOpen(true) }}
        onFocus={() => { setFocused(true); setOpen(true) }}
        onBlur={() => { setFocused(false); setTimeout(() => setOpen(false), 150) }}
        placeholder={placeholder}
        style={{
          width: width ?? 120,
          background: C.bg,
          border: `1px solid ${C.border}`,
          borderRadius: 6,
          padding: '0.3rem 0.5rem',
          color: C.text,
          fontSize: 12,
          outline: 'none',
          boxSizing: 'border-box',
        }}
      />
      {open && focused && filtered.length > 0 && (
        <div style={{
          position: 'absolute',
          top: '100%',
          left: 0,
          marginTop: 2,
          background: C.bg,
          border: `1px solid ${C.border}`,
          borderRadius: 6,
          maxHeight: 120,
          overflowY: 'auto',
          zIndex: 200,
          minWidth: width ?? 120,
        }}>
          {filtered.slice(0, 10).map((s) => (
            <div
              key={s}
              onMouseDown={(e) => { e.preventDefault(); onChange(s); setOpen(false) }}
              style={{
                padding: '0.3rem 0.5rem',
                fontSize: 12,
                color: C.text,
                cursor: 'pointer',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(245,158,11,0.1)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              {s}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── AvailableEventsPanel ─────────────────────────────────────────────────────

function AvailableEventsPanel({
  eventNames,
  onSelect,
}: {
  eventNames: string[]
  onSelect: (name: string) => void
}) {
  const [expanded, setExpanded] = useState(false)

  if (eventNames.length === 0) return null

  return (
    <div style={{
      marginBottom: '1.25rem',
      background: C.bg,
      border: `1px solid ${C.border}`,
      borderRadius: 8,
    }}>
      <button
        onClick={() => setExpanded(!expanded)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          width: '100%',
          background: 'none',
          border: 'none',
          color: C.muted,
          cursor: 'pointer',
          padding: '0.6rem 0.75rem',
          fontSize: 13,
          fontWeight: 600,
          textAlign: 'left',
        }}
      >
        {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        Available events ({eventNames.length})
      </button>
      {expanded && (
        <div style={{
          padding: '0 0.75rem 0.6rem',
          display: 'flex',
          flexWrap: 'wrap',
          gap: 6,
        }}>
          {eventNames.map((name) => (
            <button
              key={name}
              onClick={() => onSelect(name)}
              style={{
                background: 'rgba(245,158,11,0.08)',
                border: `1px solid rgba(245,158,11,0.25)`,
                borderRadius: 6,
                color: C.amber,
                padding: '0.3rem 0.6rem',
                fontSize: 12,
                cursor: 'pointer',
                fontWeight: 500,
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.background = 'rgba(245,158,11,0.18)'
                e.currentTarget.style.borderColor = 'rgba(245,158,11,0.5)'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.background = 'rgba(245,158,11,0.08)'
                e.currentTarget.style.borderColor = 'rgba(245,158,11,0.25)'
              }}
            >
              {name}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── StepFilterEditor ─────────────────────────────────────────────────────────

export function StepFilterEditor({
  projectId,
  steps,
  onChange,
  minSteps = 1,
  prefetchPropertiesFor,
}: {
  projectId: string
  steps: StepWithFilters[]
  onChange: (steps: StepWithFilters[]) => void
  /** Minimum number of steps before the remove button appears (default 1) */
  minSteps?: number
  /** When set, pre-fetches properties for these step event names on mount (used by EditFunnelModal) */
  prefetchPropertiesFor?: string[]
}) {
  const [eventNames, setEventNames] = useState<string[]>([])
  const [stepProperties, setStepProperties] = useState<Record<number, string[]>>({})
  const [filterValues, setFilterValues] = useState<Record<string, string[]>>({})

  useEffect(() => {
    api.getEventNames(projectId).then((d) => setEventNames(d.event_names || [])).catch(() => {})
  }, [projectId])

  useEffect(() => {
    if (!prefetchPropertiesFor) return
    prefetchPropertiesFor.forEach((eventName, i) => {
      if (eventName) {
        api.getEventProperties(projectId, eventName)
          .then((d) => setStepProperties((prev) => ({ ...prev, [i]: d.properties || [] })))
          .catch(() => {})
      }
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const fetchProperties = (stepIdx: number, eventName: string) => {
    if (!eventName) return
    api.getEventProperties(projectId, eventName)
      .then((d) => setStepProperties((prev) => ({ ...prev, [stepIdx]: d.properties || [] })))
      .catch(() => {})
  }

  const fetchPropertyValues = (eventName: string, property: string) => {
    if (!eventName || !property) return
    const cacheKey = `${eventName}:${property}`
    if (filterValues[cacheKey]) return
    api.getEventPropertyValues(projectId, eventName, property)
      .then((d) => setFilterValues((prev) => ({ ...prev, [cacheKey]: d.values || [] })))
      .catch(() => {})
  }

  const addStep = () => onChange([...steps, { event_name: '', filters: [] }])

  const removeStep = (i: number) => onChange(steps.filter((_, idx) => idx !== i))

  const updateStep = (i: number, val: string) => {
    onChange(steps.map((st, idx) => (idx === i ? { ...st, event_name: val } : st)))
    fetchProperties(i, val)
  }

  const addFilter = (i: number) =>
    onChange(steps.map((st, idx) => idx === i ? { ...st, filters: [...st.filters, { property: '', value: '' }] } : st))

  const addFilterWithProperty = (stepIdx: number, property: string) => {
    onChange(steps.map((st, i) => i === stepIdx ? { ...st, filters: [...st.filters, { property, value: '' }] } : st))
    const eventName = steps[stepIdx]?.event_name
    if (eventName) fetchPropertyValues(eventName, property)
  }

  const removeFilter = (stepIdx: number, filterIdx: number) =>
    onChange(steps.map((st, i) => i === stepIdx ? { ...st, filters: st.filters.filter((_, fi) => fi !== filterIdx) } : st))

  const updateFilter = (stepIdx: number, filterIdx: number, field: 'property' | 'value', val: string) => {
    onChange(steps.map((st, i) => i === stepIdx ? { ...st, filters: st.filters.map((f, fi) => fi === filterIdx ? { ...f, [field]: val } : f) } : st))
    if (field === 'property') {
      const eventName = steps[stepIdx]?.event_name
      if (eventName && val) fetchPropertyValues(eventName, val)
    }
  }

  const handleAvailableEventSelect = (eName: string) => {
    const emptyIdx = steps.findIndex((s) => !s.event_name)
    if (emptyIdx >= 0) {
      updateStep(emptyIdx, eName)
    } else {
      onChange([...steps, { event_name: eName, filters: [] }])
      fetchProperties(steps.length, eName)
    }
  }

  return (
    <>
      <AvailableEventsPanel eventNames={eventNames} onSelect={handleAvailableEventSelect} />

      <div style={{ marginBottom: '1.25rem' }}>
        <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 10 }}>Steps</label>
        {steps.map((s, i) => (
          <div key={i} style={{ marginBottom: 10 }}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <div style={{
                width: 24,
                height: 24,
                background: C.amber,
                color: '#0f1117',
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 12,
                fontWeight: 800,
                flexShrink: 0,
              }}>
                {i + 1}
              </div>
              <EventNameInput
                value={s.event_name}
                onChange={(val) => updateStep(i, val)}
                eventNames={eventNames}
              />
              {steps.length > minSteps && (
                <button
                  onClick={() => removeStep(i)}
                  style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer', padding: 4 }}
                >
                  <X size={14} />
                </button>
              )}
            </div>
            <div style={{ marginLeft: 32, marginTop: 4 }}>
              {(stepProperties[i] || []).length > 0 && (
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginBottom: 4 }}>
                  {(stepProperties[i] || [])
                    .filter((p) => !s.filters.some((f) => f.property === p))
                    .map((prop) => (
                      <button
                        key={prop}
                        onClick={() => addFilterWithProperty(i, prop)}
                        style={{
                          background: 'rgba(148,163,184,0.08)',
                          border: `1px solid ${C.border}`,
                          borderRadius: 4,
                          color: C.muted,
                          padding: '0.15rem 0.4rem',
                          fontSize: 11,
                          cursor: 'pointer',
                        }}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.borderColor = C.amber
                          e.currentTarget.style.color = C.amber
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.borderColor = C.border
                          e.currentTarget.style.color = C.muted
                        }}
                      >
                        + {prop}
                      </button>
                    ))}
                </div>
              )}
              {s.filters.map((f, fi) => (
                <div key={fi} style={{ display: 'flex', gap: 4, alignItems: 'center', marginBottom: 4 }}>
                  <AutocompleteInput
                    value={f.property}
                    onChange={(val) => updateFilter(i, fi, 'property', val)}
                    placeholder="Property"
                    suggestions={stepProperties[i] || []}
                    width={120}
                  />
                  <AutocompleteInput
                    value={f.value}
                    onChange={(val) => updateFilter(i, fi, 'value', val)}
                    placeholder="Value"
                    suggestions={filterValues[`${s.event_name}:${f.property}`] || []}
                    width={120}
                  />
                  <button
                    onClick={() => removeFilter(i, fi)}
                    style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer', padding: 2 }}
                  >
                    <X size={12} />
                  </button>
                </div>
              ))}
              <button
                onClick={() => addFilter(i)}
                style={{
                  background: 'none',
                  border: 'none',
                  color: C.muted,
                  cursor: 'pointer',
                  fontSize: 12,
                  padding: '2px 0',
                }}
              >
                + Add filter
              </button>
            </div>
          </div>
        ))}
        <button
          onClick={addStep}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            background: 'transparent',
            border: `1px dashed ${C.border}`,
            borderRadius: 8,
            color: C.muted,
            padding: '0.5rem 1rem',
            cursor: 'pointer',
            fontSize: 13,
            width: '100%',
            justifyContent: 'center',
            marginTop: 4,
          }}
        >
          <Plus size={14} /> Add step
        </button>
      </div>
    </>
  )
}
