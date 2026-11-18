import { useState } from 'react'
import { Pause, Play, Pencil, Trash2 } from 'lucide-react'
import { api } from '../../lib/api'
import type { FeatureFlag } from '../../lib/api'
import { reportError } from '../../lib/bugbarn'
import { trackEvent } from '../../lib/analytics'
import { Skeleton } from '../ui/Skeleton'
import { C, statusBadge, originBadge, StatCard } from './shared'
import { flagDate } from './flagListQuery'
import { toggleConfigBoolean } from './shape'
import type { useFlagAnalysis } from './useFlagAnalysis'

// alignBooleanSplit produces a new split where the *new default* is always
// the majority bucket. This both preserves intentional rollouts and
// self-heals legacy flags whose split contradicted their default.
//
// Examples:
// - {on:100, off:0} (any default) → newDefault gets 100, the other gets 0.
//   Self-heals the "Default: off but always returns on" inconsistency.
// - {on:80, off:20}, toggling default off→on → {on:80, off:20}. Rollout
//   already pointed the right way, no change needed.
// - {on:80, off:20}, toggling default on→off → {off:80, on:20}. Rollout
//   percentage preserved, sides swapped.
export function alignBooleanSplit(splitJSON: string, newDefault: 'on' | 'off'): string {
  let current: Record<string, number>
  try { current = JSON.parse(splitJSON) as Record<string, number> } catch { current = {} }
  const a = current.on ?? 0
  const b = current.off ?? 0
  const majority = Math.max(a, b)
  const minority = Math.min(a, b)
  const opposite = newDefault === 'on' ? 'off' : 'on'
  return JSON.stringify({ [newDefault]: majority, [opposite]: minority })
}

// DefaultToggle is the on/off switch for boolean (deploy) flags. Clicking it
// flips the flag's default value — the most common operation on a release
// flag once it's deployed.
export function DefaultToggle({ value, onChange, disabled }: { value: boolean; onChange: () => void; disabled?: boolean }) {
  return (
    <button
      onClick={onChange}
      disabled={disabled}
      title={value ? 'Default: on — click to flip to off' : 'Default: off — click to flip to on'}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        background: 'transparent',
        border: `1px solid ${C.border}`,
        borderRadius: 999,
        padding: '2px 4px 2px 10px',
        cursor: disabled ? 'default' : 'pointer',
        opacity: disabled ? 0.6 : 1,
      }}
    >
      <span style={{
        fontSize: 11, fontWeight: 700, letterSpacing: 0.5, textTransform: 'uppercase',
        color: value ? C.success : C.muted,
      }}>
        {value ? 'on' : 'off'}
      </span>
      <span style={{
        position: 'relative',
        width: 32, height: 18,
        background: value ? 'rgba(16,185,129,0.25)' : C.border,
        borderRadius: 999,
        transition: 'background 120ms ease',
      }}>
        <span style={{
          position: 'absolute',
          top: 2,
          left: value ? 16 : 2,
          width: 14, height: 14,
          background: value ? C.success : C.muted,
          borderRadius: '50%',
          transition: 'left 120ms ease',
        }} />
      </span>
    </button>
  )
}

export function useFlagActions(
  flag: FeatureFlag, projectId: string,
  onUpdated: (f: FeatureFlag) => void, onDeleted: () => void,
) {
  const [toggling, setToggling] = useState(false)
  const [togglingDefault, setTogglingDefault] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const toggleStatus = async () => {
    setToggling(true)
    try {
      const nextStatus = flag.status === 'active' ? 'paused' : 'active'
      const updated = await api.updateFlag(projectId, flag.id, { ...flag, status: nextStatus })
      trackEvent('flag_toggled', { flag_key: flag.flag_key, status: nextStatus })
      onUpdated(updated)
    } catch (e) {
      reportError(e instanceof Error ? e : new Error(String(e)), { source: 'Flags.toggle' })
    } finally {
      setToggling(false)
    }
  }

  const toggleDefault = async () => {
    if (flag.flag_type !== 'boolean') return
    const newDefault: 'on' | 'off' = flag.default_variant === 'on' ? 'off' : 'on'
    setTogglingDefault(true)
    try {
      // A config flag keeps its default variant and flips the value it holds:
      // renaming it to on/off points it at a variant the flag may not have.
      const updated = await api.updateFlag(projectId, flag.id, flag.flag_kind === 'config'
        ? { ...flag, ...toggleConfigBoolean(flag) }
        : { ...flag, default_variant: newDefault, split: alignBooleanSplit(flag.split, newDefault) })
      onUpdated(updated)
    } catch (e) {
      reportError(e instanceof Error ? e : new Error(String(e)), { source: 'Flags.toggleDefault' })
    } finally {
      setTogglingDefault(false)
    }
  }

  const handleDelete = async () => {
    if (!confirm('Delete this flag? This cannot be undone.')) return
    setDeleting(true)
    try {
      await api.deleteFlag(projectId, flag.id)
      trackEvent('flag_deleted', { flag_key: flag.flag_key, flag_id: flag.id })
      onDeleted()
    } catch (e) {
      reportError(e instanceof Error ? e : new Error(String(e)), { source: 'Flags.delete' })
    } finally {
      setDeleting(false)
    }
  }

  return { toggling, togglingDefault, deleting, toggleStatus, toggleDefault, handleDelete }
}

export function FlagHeader({ flag, isConfig, configValue, actions, onEdit }: {
  flag: FeatureFlag
  isConfig: boolean
  configValue: unknown
  actions: ReturnType<typeof useFlagActions>
  onEdit: () => void
}) {
  const { toggling, togglingDefault, deleting, toggleStatus, toggleDefault, handleDelete } = actions
  return (
    <div style={{
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'flex-start',
      gap: 12,
      flexWrap: 'wrap',
      marginBottom: 16,
    }}>
        <div style={{ flex: '1 1 200px', minWidth: 0 }}>
          <h2 style={{ fontSize: 20, fontWeight: 800, margin: '0 0 4px', overflowWrap: 'anywhere' }}>{flag.name}</h2>
          <code style={{
            fontSize: 13, color: C.amber, background: 'rgba(245,158,11,0.1)',
            padding: '2px 8px', borderRadius: 4,
            display: 'inline-block', maxWidth: '100%', overflowWrap: 'anywhere',
          }}>
            {flag.flag_key}
          </code>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap', flexShrink: 0 }}>
          {flag.flag_type === 'boolean' && (
            <DefaultToggle value={isConfig ? configValue === true : flag.default_variant === 'on'} onChange={toggleDefault} disabled={togglingDefault} />
          )}
          {originBadge(flag.origin)}
          {statusBadge(flag.status)}
          <button onClick={toggleStatus} disabled={toggling}
            title={flag.status === 'active'
              ? 'Pause — every evaluation returns the default value, targeting rules and split are bypassed (reason: DISABLED)'
              : 'Resume — re-enable targeting rules and split'}
            style={{ background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '4px 8px', display: 'flex', alignItems: 'center' }}>
            {flag.status === 'active' ? <Pause size={14} /> : <Play size={14} />}
          </button>
          <button onClick={() => onEdit()} title="Edit"
            style={{ background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '4px 8px', display: 'flex', alignItems: 'center' }}>
            <Pencil size={14} />
          </button>
          <button onClick={handleDelete} disabled={deleting} title="Delete"
            style={{ background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 6, color: C.error, cursor: 'pointer', padding: '4px 8px', display: 'flex', alignItems: 'center' }}>
            <Trash2 size={14} />
          </button>
        </div>
    </div>
  )
}

export function FlagMeta({ flag, isConfig }: { flag: FeatureFlag; isConfig: boolean }) {
  return (
      <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', marginBottom: 16, fontSize: 13, color: C.muted }}>
        <div>Kind: <span style={{ color: C.text }}>{isConfig ? 'config value' : 'experiment'}</span></div>
        <div>Type: <span style={{ color: C.text }}>{flag.flag_type}</span></div>
        {!isConfig && flag.flag_type !== 'boolean' && (
          <div>Default: <span style={{ color: C.text }}>{flag.default_variant}</span></div>
        )}
        {flag.conversion_event && <div>Conversion: <span style={{ color: C.text }}>{flag.conversion_event}</span></div>}
        <div>Created: <span style={{ color: C.text }}>{flagDate(flag.created_at) ?? 'unknown'}</span></div>
        <div>Last evaluated: <span style={{ color: C.text }}>{flagDate(flag.last_evaluated_at) ?? 'never'}</span></div>
      </div>
  )
}

// For boolean flags, only render the split tiles when there's a real
// rollout (neither side is 0%). A 100/0 split is just "use the default" and
// the header toggle already conveys that.
export function SplitTiles({ flag, variants, split }: {
  flag: FeatureFlag
  variants: Record<string, unknown>
  split: Record<string, number>
}) {
  const entries = Object.entries(split)
  const hasRollout = flag.flag_type !== 'boolean' || entries.every(([, pct]) => pct !== 0)
  if (!hasRollout) return null
  return (
    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 16 }}>
      {entries.map(([variant, pct]) => (
        <div key={variant} style={{
          background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8,
          padding: '8px 12px', fontSize: 13,
        }}>
          <span style={{ color: C.text, fontWeight: 600 }}>{variant}</span>
          <span style={{ color: C.muted }}> = {JSON.stringify(variants[variant])}</span>
          <span style={{ color: C.amber, marginLeft: 8 }}>{pct}%</span>
        </div>
      ))}
    </div>
  )
}

type FlagAnalysis = ReturnType<typeof useFlagAnalysis>['analysis']

export function AnalyticsPanel({ analysis, loading }: { analysis: FlagAnalysis; loading: boolean }) {
  if (analysis?.unavailable === 'config') {
    return (
      <div style={{ color: C.muted, fontSize: 14, textAlign: 'center', padding: '2rem', lineHeight: 1.6 }}>
        Config values record no evaluations, so there is no variant or conversion
        report for this flag.
        <br />
        That is the point — a server polling this value would otherwise write
        thousands of meaningless rows a day.
      </div>
    )
  }
  if (loading) {
    return (
      <div style={{ display: 'flex', gap: '1rem' }}>
        <Skeleton height={180} />
        <Skeleton height={180} />
      </div>
    )
  }
  const results = analysis?.results ?? []
  const bestRate = Math.max(...results.map(r => r.rate), 0)
  if (results.length === 0) {
    return (
      <div style={{ color: C.muted, fontSize: 14, textAlign: 'center', padding: '2rem' }}>
        No evaluations yet. Integrate the flag in your application to start collecting data.
      </div>
    )
  }
  return (
    <>
      <div style={{ display: 'flex', gap: '1rem', marginBottom: '1.5rem', flexWrap: 'wrap' }}>
        {results.map((r) => (
          <StatCard
            key={r.variant}
            label={r.variant}
            sample={r.sample}
            conversions={r.conversions}
            rate={r.rate}
            winner={analysis?.significant === true && r.rate === bestRate && results.length > 1}
          />
        ))}
      </div>
      {analysis && <SignificanceBanner analysis={analysis} />}
    </>
  )
}

function SignificanceBanner({ analysis }: { analysis: NonNullable<FlagAnalysis> }) {
  return (
    <div style={{
      background: analysis.significant ? 'rgba(16,185,129,0.08)' : 'rgba(245,158,11,0.08)',
      border: `1px solid ${analysis.significant ? 'rgba(16,185,129,0.3)' : 'rgba(245,158,11,0.3)'}`,
      borderRadius: 10, padding: '1rem 1.25rem',
      fontSize: 14, color: analysis.significant ? C.success : C.amber, fontWeight: 600,
    }}>
      {analysis.significant
        ? `Significant (95% CI) — z = ${(analysis.z_score ?? 0).toFixed(2)}`
        : `Not significant yet${analysis.z_score ? ` — z = ${analysis.z_score.toFixed(2)}` : ''}`}
    </div>
  )
}
