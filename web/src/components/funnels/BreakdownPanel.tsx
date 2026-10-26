import { useEffect, useState } from 'react'
import { api, Funnel } from '../../lib/api'
import { FunnelBreakdown, getFunnelBreakdown } from '../../lib/api.funnelbreakdown'
import { C } from '../../lib/theme'
import { useFunnelLink } from './funnelLink'

const cell = { padding: '0.4rem 0.75rem', textAlign: 'right' as const, fontSize: 13 }

// "Break down by" for a funnel: the same funnel split by the value an event
// property has on each entry's first step, so surfaces or sources can be compared.
type Scope = { projectId: string; funnelId: string; environment: string; segment?: string; segmentId?: string }

// Properties the first step event carries; reset when the funnel changes.
function useEntryProperties(projectId: string, entryEvent: string) {
  const [properties, setProperties] = useState<string[]>([])
  useEffect(() => {
    if (!entryEvent) return setProperties([])
    api.getEventProperties(projectId, entryEvent)
      .then((d) => setProperties(d.properties ?? []))
      .catch(() => setProperties([]))
  }, [projectId, entryEvent])
  return properties
}

// The breakdown of one property; empty property means no breakdown.
function useBreakdown(scope: Scope, property: string) {
  const [state, setState] = useState<{ result: FunnelBreakdown | null; error: string; loading: boolean }>(
    { result: null, error: '', loading: false },
  )
  const { projectId, funnelId, environment, segment, segmentId } = scope
  useEffect(() => {
    if (!property) return setState({ result: null, error: '', loading: false })
    let current = true
    setState({ result: null, error: '', loading: true })
    getFunnelBreakdown(projectId, funnelId, property, { segment, segmentId, environment })
      .then((result) => current && setState({ result, error: '', loading: false }))
      .catch((e) => current && setState({ result: null, error: String(e), loading: false }))
    return () => { current = false }
  }, [projectId, funnelId, property, segment, segmentId, environment])
  return state
}

export function BreakdownPanel({
  projectId,
  funnel,
  environment,
  segment,
  segmentId,
}: {
  projectId: string
  funnel: Funnel
  environment: string
  segment?: string
  segmentId?: string
}) {
  const entryEvent = funnel.steps?.[0]?.event_name ?? ''
  const properties = useEntryProperties(projectId, entryEvent)
  // The breakdown lives in the URL (?breakdown=, ?group=), so a link opens it.
  // A property the entry event does not carry is ignored.
  const [link, updateLink] = useFunnelLink()
  const property = properties.includes(link.breakdown) ? link.breakdown : ''
  const setProperty = (p: string) => updateLink({ breakdown: p, group: '' })
  const { result, error, loading } = useBreakdown({ projectId, funnelId: funnel.id, environment, segment, segmentId }, property)

  if (!entryEvent || properties.length === 0) return null

  return (
    <div style={{ marginTop: '1.5rem' }}>
      <label style={{ fontSize: 13, color: C.muted, display: 'flex', alignItems: 'center', gap: 8 }}>
        Break down by
        <select
          aria-label="Break down by"
          value={property}
          onChange={(e) => setProperty(e.target.value)}
          style={{ background: C.surface, color: C.text, border: `1px solid ${C.border}`, borderRadius: 6, padding: '0.3rem 0.5rem' }}
        >
          <option value="">No breakdown</option>
          {properties.map((p) => <option key={p} value={p}>{p}</option>)}
        </select>
      </label>
      {property && (
        <div style={{ fontSize: 12, color: C.muted, marginTop: 6 }}>
          Entries are grouped by the {property} of their first step ({entryEvent}). Entries without it are shown as (none).
        </div>
      )}
      <BreakdownBody funnel={funnel} result={result} error={error} loading={loading} group={link.group} />
    </div>
  )
}

type BodyProps = { funnel: Funnel; result: FunnelBreakdown | null; error: string; loading: boolean; group: string }

function BreakdownBody({ funnel, result, error, loading, group }: BodyProps) {
  if (loading) return <div style={{ color: C.muted, padding: '1rem 0' }}>Loading breakdown…</div>
  if (error) return <div style={{ color: C.error, fontSize: 14, padding: '1rem 0' }}>Failed to load breakdown: {error}</div>
  return result ? <BreakdownTable funnel={funnel} breakdown={result} group={group} /> : null
}

// group is the value a link points at; its row is marked. An unknown value
// marks nothing.
function BreakdownTable({ funnel, breakdown, group }: { funnel: Funnel; breakdown: FunnelBreakdown; group: string }) {
  if (breakdown.groups.length === 0) {
    return <div style={{ color: C.muted, padding: '1rem 0', fontSize: 14 }}>No entries in this range.</div>
  }
  return (
    <div style={{ overflowX: 'auto', marginTop: 12 }}>
      <table style={{ borderCollapse: 'collapse', width: '100%', color: C.text }}>
        <thead>
          <tr style={{ borderBottom: `1px solid ${C.border}`, color: C.muted }}>
            <th style={{ ...cell, textAlign: 'left' }}>{breakdown.property}</th>
            {funnel.steps.map((s) => <th key={s.step_order} style={cell}>{s.event_name}</th>)}
          </tr>
        </thead>
        <tbody>
          {breakdown.groups.map((g) => (
            <BreakdownRow
              key={g.other ? '__other__' : `v:${g.value}`}
              group={g}
              linked={group !== '' && !g.other && g.value === group}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}

type Group = FunnelBreakdown['groups'][number]

function BreakdownRow({ group, linked }: { group: Group; linked: boolean }) {
  return (
    <tr
      aria-current={linked ? 'true' : undefined}
      style={{ borderBottom: `1px solid ${C.border}`, background: linked ? 'rgba(245,158,11,0.1)' : undefined }}
    >
      <td style={{ ...cell, textAlign: 'left', fontStyle: group.value === '' ? 'italic' : 'normal' }}>{group.label}</td>
      {group.results.map((r, i) => (
        <td key={i} style={cell}>
          {r.count.toLocaleString()}
          {i > 0 && <span style={{ color: C.muted }}> ({(r.conversion * 100).toFixed(1)}%)</span>}
        </td>
      ))}
    </tr>
  )
}
