import { useEffect, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { Flag, Plus, Pause, Play, Pencil, Trash2 } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api } from '../lib/api'
import type { FeatureFlag, FlagAnalysis, TargetingRule } from '../lib/api'
import { useProjects } from '../lib/projects'
import { reportError } from '../lib/bugbarn'
import { trackEvent } from '../lib/analytics'
import { Skeleton } from '../components/ui/Skeleton'
import { C, statusBadge, originBadge, StatCard, FlagsTitle } from '../components/flags/shared'
import { FlagFormModal } from '../components/flags/FlagFormModal'
import { FlagList } from '../components/flags/FlagList'
import { flagDate } from '../components/flags/flagListQuery'
import { FlagPlayground, TargetingRulesDisplay } from '../components/flags/FlagPlayground'

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

interface StringVariantRow {
  // The variant name is what's returned (and what targeting rules pick).
  // returnValue is only different from name when the user opts in to the
  // "use a different return value" disclosure — rare in practice.
  name: string
  returnValue: string
  splitPct: number
}

// parseFlagFormState converts a stored FeatureFlag back into the form state.
// Returns the values the modal needs to pre-fill when editing.
export function parseFlagFormState(flag: FeatureFlag): {
  defaultBool: 'on' | 'off'
  rolloutEnabled: boolean
  rolloutPct: number
  variants: StringVariantRow[]
  defaultVariant: string
  advancedReturnValues: boolean
  targetingRules: TargetingRule[]
} {
  let split: Record<string, number> = {}
  try { split = JSON.parse(flag.split) as Record<string, number> } catch { /* leave empty */ }
  let variantsObj: Record<string, unknown> = {}
  try { variantsObj = JSON.parse(flag.variants) as Record<string, unknown> } catch { /* leave empty */ }
  let rules: TargetingRule[] = []
  try { rules = JSON.parse(flag.targeting_rules || '[]') as TargetingRule[] } catch { /* leave empty */ }

  if (flag.flag_type === 'boolean') {
    const defaultBool: 'on' | 'off' = flag.default_variant === 'on' ? 'on' : 'off'
    const opposite = defaultBool === 'on' ? 'off' : 'on'
    const oppositePct = split[opposite] ?? 0
    const rolloutEnabled = oppositePct > 0 && oppositePct < 100
    return {
      defaultBool,
      rolloutEnabled,
      rolloutPct: rolloutEnabled ? oppositePct : 0,
      variants: [
        { name: 'control', returnValue: 'control', splitPct: 50 },
        { name: 'treatment', returnValue: 'treatment', splitPct: 50 },
      ],
      defaultVariant: 'control',
      advancedReturnValues: false,
      targetingRules: rules,
    }
  }

  // String flag — rebuild variant rows from variants+split.
  const rows: StringVariantRow[] = Object.entries(variantsObj).map(([name, value]) => ({
    name,
    returnValue: typeof value === 'string' ? value : JSON.stringify(value),
    splitPct: split[name] ?? 0,
  }))
  const advanced = rows.some((r) => r.name !== r.returnValue)
  return {
    defaultBool: 'off',
    rolloutEnabled: false,
    rolloutPct: 0,
    variants: rows.length >= 2 ? rows : [
      { name: 'control', returnValue: 'control', splitPct: 50 },
      { name: 'treatment', returnValue: 'treatment', splitPct: 50 },
    ],
    defaultVariant: flag.default_variant || (rows[0]?.name ?? 'control'),
    advancedReturnValues: advanced,
    targetingRules: rules,
  }
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

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      style={{
        background: 'transparent',
        border: 'none',
        borderBottom: `2px solid ${active ? C.amber : 'transparent'}`,
        color: active ? C.text : C.muted,
        fontWeight: active ? 600 : 500,
        cursor: 'pointer',
        padding: '8px 16px',
        fontSize: 14,
        marginBottom: -1,
      }}
    >
      {children}
    </button>
  )
}

function FlagDetail({ flag, projectId, onUpdated, onDeleted }: {
  flag: FeatureFlag; projectId: string
  onUpdated: (f: FeatureFlag) => void
  onDeleted: () => void
}) {
  const [analysis, setAnalysis] = useState<FlagAnalysis | null>(null)
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState(false)
  const [togglingDefault, setTogglingDefault] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [tab, setTab] = useState<'analytics' | 'tryit'>('analytics')
  const [editing, setEditing] = useState(false)

  useEffect(() => {
    setLoading(true)
    api.getFlagAnalysis(projectId, flag.id)
      .then(setAnalysis)
      .catch((e) => { console.error(e); reportError(e, { source: 'Flags' }) })
      .finally(() => setLoading(false))
  }, [projectId, flag.id])

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
      const updated = await api.updateFlag(projectId, flag.id, {
        ...flag,
        default_variant: newDefault,
        split: alignBooleanSplit(flag.split, newDefault),
      })
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

  const variants = (() => { try { return JSON.parse(flag.variants) as Record<string, unknown> } catch { return {} } })()
  const split = (() => { try { return JSON.parse(flag.split) as Record<string, number> } catch { return {} } })()

  const results = analysis?.results ?? []
  const bestRate = Math.max(...results.map(r => r.rate), 0)
  const isConfig = flag.flag_kind === 'config'
  const configValue = isConfig ? variants[flag.default_variant] : undefined

  return (
    <div>
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
            <DefaultToggle value={flag.default_variant === 'on'} onChange={toggleDefault} disabled={togglingDefault} />
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
          <button onClick={() => setEditing(true)} title="Edit"
            style={{ background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '4px 8px', display: 'flex', alignItems: 'center' }}>
            <Pencil size={14} />
          </button>
          <button onClick={handleDelete} disabled={deleting} title="Delete"
            style={{ background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 6, color: C.error, cursor: 'pointer', padding: '4px 8px', display: 'flex', alignItems: 'center' }}>
            <Trash2 size={14} />
          </button>
        </div>
      </div>

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

      {(() => {
        // For boolean flags, only render the split tiles when there's a real
        // rollout (neither side is 0%). A 100/0 split is just "use the
        // default" and the header toggle already conveys that.
        if (isConfig) return null
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
      })()}

      {isConfig && (
        <div style={{
          background: C.bg, border: `1px solid ${C.border}`, borderRadius: 10,
          padding: '1rem 1.25rem', marginBottom: 16,
        }}>
          <div style={{ fontSize: 12, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Current value</div>
          <code style={{ fontSize: 20, color: C.amber, fontWeight: 700, overflowWrap: 'anywhere' }}>
            {JSON.stringify(configValue)}
          </code>
        </div>
      )}

      <TargetingRulesDisplay rulesJSON={flag.targeting_rules} />

      <div style={{ display: 'flex', gap: 0, borderBottom: `1px solid ${C.border}`, margin: '12px 0 16px' }}>
        <TabButton active={tab === 'analytics'} onClick={() => setTab('analytics')}>Analytics</TabButton>
        <TabButton active={tab === 'tryit'} onClick={() => setTab('tryit')}>Try it</TabButton>
      </div>

      {tab === 'tryit' ? (
        <FlagPlayground projectId={projectId} flag={flag} />
      ) : analysis?.unavailable === 'config' ? (
        <div style={{ color: C.muted, fontSize: 14, textAlign: 'center', padding: '2rem', lineHeight: 1.6 }}>
          Config values record no evaluations, so there is no variant or conversion
          report for this flag.
          <br />
          That is the point — a server polling this value would otherwise write
          thousands of meaningless rows a day.
        </div>
      ) : loading ? (
        <div style={{ display: 'flex', gap: '1rem' }}>
          <Skeleton height={180} />
          <Skeleton height={180} />
        </div>
      ) : results.length === 0 ? (
        <div style={{ color: C.muted, fontSize: 14, textAlign: 'center', padding: '2rem' }}>
          No evaluations yet. Integrate the flag in your application to start collecting data.
        </div>
      ) : (
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

          {analysis && (
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
          )}
        </>
      )}

      {editing && (
        <FlagFormModal
          projectId={projectId}
          flag={flag}
          onClose={() => setEditing(false)}
          onSaved={(f) => { onUpdated(f); setEditing(false) }}
        />
      )}
    </div>
  )
}

// Wide screens: both columns stick below the page header and fill the
// viewport, the rows scroll inside the list column and the detail pane stays
// in view whichever flag is picked. Phones: one column, the detail replaces
// the list and Back returns to the same scroll position.
const LAYOUT_CSS = `
  .flags-layout { display: grid; grid-template-columns: minmax(260px, 340px) minmax(0, 1fr); gap: 1.5rem; align-items: start; }
  @media (min-width: 768px) {
    .flags-list, .flags-detail { position: sticky; top: 1rem; max-height: calc(100vh - 2rem); box-sizing: border-box; }
    .flags-list { display: flex; flex-direction: column; min-width: 0; }
    .flags-rows { flex: 1 1 auto; min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding-right: 4px; }
    .flags-detail { overflow-y: auto; }
    .back-btn { display: none !important; }
  }
  @media (max-width: 767px) {
    .flags-layout { grid-template-columns: minmax(0, 1fr); }
    .flags-list.detail-open { display: none; }
    .flags-detail.no-selection { display: none; }
    .back-btn { display: block !important; }
  }
`

const isNarrow = () => typeof window !== 'undefined' && !!window.matchMedia?.('(max-width: 767px)').matches

// useProjectFlags loads a project's flags. flags is null until the first
// response; a failed load sets error instead of reading as an empty project.
function useProjectFlags(projectId: string | undefined) {
  const [flags, setFlags] = useState<FeatureFlag[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    setFlags(null)
    setError(null)
    api.listFlags(projectId)
      .then((d) => { if (!cancelled) setFlags(d.flags || []) })
      .catch((e) => {
        reportError(e instanceof Error ? e : new Error(String(e)), { source: 'Flags' })
        if (!cancelled) setError(e instanceof Error ? e.message : String(e))
      })
    return () => { cancelled = true }
  }, [projectId, attempt])

  return { flags, setFlags, error, retry: () => setAttempt((n) => n + 1) }
}

export default function Flags() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const [searchParams, setSearchParams] = useSearchParams()
  const { flags, setFlags, error, retry } = useProjectFlags(projectId)
  const [showCreate, setShowCreate] = useState(false)
  const listScroll = useRef(0)

  const selectedKey = searchParams.get('flag')
  const selected = flags?.find((f) => f.flag_key === selectedKey) ?? null

  const select = (f: FeatureFlag) => {
    listScroll.current = window.scrollY
    setSearchParams({ flag: f.flag_key }, { replace: true })
    if (isNarrow()) window.scrollTo(0, 0)
  }
  const clearSelection = () => {
    setSearchParams({}, { replace: true })
    if (isNarrow()) requestAnimationFrame(() => window.scrollTo(0, listScroll.current))
  }

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, padding: '2rem', textAlign: 'center' }}>Select a project to manage flags.</div>
      </Shell>
    )
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <style>{LAYOUT_CSS}</style>

      {showCreate && (
        <FlagFormModal projectId={projectId} onClose={() => setShowCreate(false)}
          onSaved={(f) => { setFlags((prev) => [f, ...(prev ?? [])]); setShowCreate(false); select(f) }} />
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: '1.25rem' }}>
        <FlagsTitle projectId={projectId} active="flags" />
        <button onClick={() => setShowCreate(true)} style={{
          display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0,
          background: C.amber, border: 'none', borderRadius: 8,
          color: '#0f1117', padding: '0.6rem 1.1rem', cursor: 'pointer', fontSize: 14, fontWeight: 700,
        }}>
          <Plus size={16} /> Create flag
        </button>
      </div>

      <div className="flags-layout">
        <div className={`flags-list${selected ? ' detail-open' : ''}`}>
          <FlagList flags={flags} error={error} onRetry={retry} selectedId={selected?.id ?? null}
            onSelect={select} onCreate={() => setShowCreate(true)} />
        </div>

        <div className={`flags-detail${!selected ? ' no-selection' : ''}`} style={{
          background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12,
          padding: '1.5rem', minHeight: 300,
        }}>
          <button className="back-btn" onClick={clearSelection} style={{
            display: 'none', background: 'none', border: 'none', color: C.amber,
            cursor: 'pointer', fontSize: 14, fontWeight: 600, padding: '0 0 1rem 0',
          }}>
            ← Back
          </button>
          {!selected ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: 300, color: C.muted, gap: 8 }}>
              <Flag size={40} opacity={0.3} />
              <div>Select a flag to view results</div>
            </div>
          ) : (
            <FlagDetail flag={selected} projectId={projectId}
              onUpdated={(f) => { setFlags((prev) => prev?.map((x) => x.id === f.id ? f : x) ?? prev); setSearchParams({ flag: f.flag_key }, { replace: true }) }}
              onDeleted={() => { setFlags((prev) => prev?.filter((x) => x.id !== selected.id) ?? prev); clearSelection() }}
            />
          )}
        </div>
      </div>
    </Shell>
  )
}
