import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Flag, Plus, X, Pause, Play, Pencil, Trash2 } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api } from '../lib/api'
import type { FeatureFlag, FlagAnalysis, FlagEvaluationResult, TargetingRule } from '../lib/api'
import { useProjects } from '../lib/projects'
import { reportError } from '../lib/bugbarn'
import { trackEvent } from '../lib/analytics'
import { Skeleton } from '../components/ui/Skeleton'
import { C, statusBadge, StatCard } from '../components/flags/shared'
import { FlagFormModal } from '../components/flags/FlagFormModal'

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

function FlagPlayground({ projectId, flag }: { projectId: string; flag: FeatureFlag }) {
  const [flagKey, setFlagKey] = useState(flag.flag_key)
  const [defaultValue, setDefaultValue] = useState('')
  // targeting_key is what the eval service buckets on; defaulting to user_id
  // would silently bucket every user to the same variant.
  const [context, setContext] = useState<Array<{ k: string; v: string }>>([{ k: 'targeting_key', v: '' }])
  const [result, setResult] = useState<FlagEvaluationResult | null>(null)
  const [evaluating, setEvaluating] = useState(false)
  const [requestError, setRequestError] = useState<string | null>(null)

  // Keep flag_key in sync if the user switches to a different flag in the list.
  useEffect(() => { setFlagKey(flag.flag_key) }, [flag.flag_key])

  // Build the context object the API expects. Skip rows with empty keys.
  // Cast numbers and booleans so targeting-rule comparisons line up with what
  // a real SDK would send.
  const contextObject = (() => {
    const out: Record<string, unknown> = {}
    for (const { k, v } of context) {
      const key = k.trim()
      if (!key) continue
      if (v === 'true') out[key] = true
      else if (v === 'false') out[key] = false
      else if (v !== '' && !isNaN(Number(v))) out[key] = Number(v)
      else out[key] = v
    }
    return out
  })()

  // Debounced live evaluation. Triggers on every change to flag_key, default,
  // or context (300ms quiet period).
  const debounceRef = useRef<number | undefined>(undefined)
  useEffect(() => {
    if (!flagKey.trim()) {
      setResult(null)
      setRequestError(null)
      return
    }
    window.clearTimeout(debounceRef.current)
    debounceRef.current = window.setTimeout(() => {
      setEvaluating(true)
      setRequestError(null)
      api.evaluateFlagPlayground(projectId, {
        flag_key: flagKey.trim(),
        default_value: defaultValue || undefined,
        context: contextObject,
      })
        .then(setResult)
        .catch((e) => {
          setResult(null)
          setRequestError(e instanceof Error ? e.message : String(e))
        })
        .finally(() => setEvaluating(false))
    }, 300)
    return () => window.clearTimeout(debounceRef.current)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, flagKey, defaultValue, JSON.stringify(contextObject)])

  const inputStyle: React.CSSProperties = {
    background: C.bg, border: `1px solid ${C.border}`, borderRadius: 6,
    color: C.text, padding: '6px 10px', fontSize: 13, width: '100%',
  }
  const labelStyle: React.CSSProperties = { fontSize: 12, color: C.muted, marginBottom: 4, display: 'block' }

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
      <div>
        <div style={{ marginBottom: 12 }}>
          <label style={labelStyle}>Flag key</label>
          <input value={flagKey} onChange={(e) => setFlagKey(e.target.value)} style={inputStyle} spellCheck={false} />
        </div>
        <div style={{ marginBottom: 12 }}>
          <label style={labelStyle}>Default value (returned if flag is missing)</label>
          <input value={defaultValue} onChange={(e) => setDefaultValue(e.target.value)} style={inputStyle} placeholder="optional" />
        </div>
        <div style={{ marginBottom: 8 }}>
          <label style={labelStyle}>Evaluation context</label>
          {context.map((row, i) => (
            <div key={i} style={{ display: 'flex', gap: 6, marginBottom: 6 }}>
              <input
                value={row.k}
                onChange={(e) => setContext((rows) => rows.map((r, idx) => idx === i ? { ...r, k: e.target.value } : r))}
                style={{ ...inputStyle, flex: '0 0 40%' }}
                placeholder="key"
                spellCheck={false}
              />
              <input
                value={row.v}
                onChange={(e) => setContext((rows) => rows.map((r, idx) => idx === i ? { ...r, v: e.target.value } : r))}
                style={{ ...inputStyle, flex: 1 }}
                placeholder="value"
                spellCheck={false}
              />
              <button
                onClick={() => setContext((rows) => rows.filter((_, idx) => idx !== i))}
                disabled={context.length === 1}
                title="Remove row"
                style={{ background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '0 8px' }}
              >
                <X size={12} />
              </button>
            </div>
          ))}
          <button
            onClick={() => setContext((rows) => [...rows, { k: '', v: '' }])}
            style={{ background: 'transparent', border: `1px dashed ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '4px 10px', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 4 }}
          >
            <Plus size={12} /> Add field
          </button>
        </div>
      </div>

      <div style={{ background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8, padding: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
          <span style={{ fontSize: 12, color: C.muted, letterSpacing: 0.5, textTransform: 'uppercase' }}>Result</span>
          {evaluating && <span style={{ fontSize: 11, color: C.muted }}>evaluating…</span>}
        </div>
        {requestError ? (
          <div style={{ color: C.error, fontSize: 13 }}>{requestError}</div>
        ) : result ? (
          <>
            <div style={{ marginBottom: 8 }}>
              <span style={{ fontSize: 12, color: C.muted }}>Variant: </span>
              <code style={{ fontSize: 13, color: C.amber, background: 'rgba(245,158,11,0.1)', padding: '2px 6px', borderRadius: 4 }}>
                {result.variant || '(none)'}
              </code>
            </div>
            <div style={{ marginBottom: 8 }}>
              <span style={{ fontSize: 12, color: C.muted }}>Value: </span>
              <code style={{ fontSize: 13, color: C.text }}>{JSON.stringify(result.value)}</code>
            </div>
            <div style={{ marginBottom: 8 }}>
              <span style={{ fontSize: 12, color: C.muted }}>Reason: </span>
              <code style={{ fontSize: 13, color: result.reason === 'ERROR' ? C.error : C.success }}>{result.reason}</code>
            </div>
            {result.error && (
              <div style={{ marginTop: 12, padding: 8, background: 'rgba(239,68,68,0.08)', border: `1px solid rgba(239,68,68,0.3)`, borderRadius: 6, color: C.error, fontSize: 12 }}>
                {result.error}{result.error_code ? ` (${result.error_code})` : ''}
              </div>
            )}
          </>
        ) : (
          <div style={{ color: C.muted, fontSize: 13 }}>Enter a flag key to evaluate.</div>
        )}
      </div>
    </div>
  )
}

function TargetingRulesDisplay({ rulesJSON }: { rulesJSON: string }) {
  const rules: TargetingRule[] = (() => { try { return JSON.parse(rulesJSON || '[]') } catch { return [] } })()
  if (rules.length === 0) return null
  return (
    <div style={{ marginBottom: 24 }}>
      <div style={{ fontSize: 12, color: C.muted, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 8 }}>
        Targeting Rules ({rules.length})
      </div>
      {rules.map((rule, i) => (
        <div key={i} style={{
          background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8,
          padding: '10px 12px', marginBottom: 6, fontSize: 13,
        }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
            <span style={{ color: C.text, fontWeight: 600 }}>{rule.name || `Rule ${i + 1}`}</span>
            <span style={{ color: C.amber }}>→ {rule.variant}</span>
          </div>
          <div style={{ color: C.muted, fontSize: 12 }}>
            {rule.match === 'all' ? 'ALL' : 'ANY'} of:{' '}
            {rule.conditions.map((c, j) => (
              <span key={j}>
                {j > 0 && <span style={{ color: C.amber }}> {rule.match === 'all' ? '&&' : '||'} </span>}
                <code style={{ color: C.text }}>{c.context_key}</code>
                {' '}<span style={{ color: C.amber }}>{c.operator}</span>{' '}
                {c.operator !== 'present' && c.operator !== 'not_present' && (
                  <code style={{ color: C.text }}>"{c.value}"</code>
                )}
              </span>
            ))}
          </div>
        </div>
      ))}
    </div>
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
        <div>Type: <span style={{ color: C.text }}>{flag.flag_type}</span></div>
        {flag.flag_type !== 'boolean' && (
          <div>Default: <span style={{ color: C.text }}>{flag.default_variant}</span></div>
        )}
        {flag.conversion_event && <div>Conversion: <span style={{ color: C.text }}>{flag.conversion_event}</span></div>}
      </div>

      {(() => {
        // For boolean flags, only render the split tiles when there's a real
        // rollout (neither side is 0%). A 100/0 split is just "use the
        // default" and the header toggle already conveys that.
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

      <TargetingRulesDisplay rulesJSON={flag.targeting_rules} />

      <div style={{ display: 'flex', gap: 0, borderBottom: `1px solid ${C.border}`, margin: '12px 0 16px' }}>
        <TabButton active={tab === 'analytics'} onClick={() => setTab('analytics')}>Analytics</TabButton>
        <TabButton active={tab === 'tryit'} onClick={() => setTab('tryit')}>Try it</TabButton>
      </div>

      {tab === 'tryit' ? (
        <FlagPlayground projectId={projectId} flag={flag} />
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

export default function Flags() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const [flags, setFlags] = useState<FeatureFlag[]>([])
  const [selected, setSelected] = useState<FeatureFlag | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [showDetail, setShowDetail] = useState(false)

  useEffect(() => {
    if (!projectId) return
    setSelected(null)
    setShowDetail(false)
    api.listFlags(projectId)
      .then((d) => setFlags(d.flags || []))
      .catch((e) => { console.error(e); reportError(e, { source: 'Flags' }) })
  }, [projectId])

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
      <style>{`
        @media (max-width: 767px) {
          .flags-layout { grid-template-columns: 1fr !important; }
          .flags-list.detail-open { display: none !important; }
          .flags-detail.no-selection { display: none !important; }
          .back-btn { display: block !important; }
        }
        @media (min-width: 768px) {
          .back-btn { display: none !important; }
        }
      `}</style>

      {showCreate && (
        <FlagFormModal projectId={projectId} onClose={() => setShowCreate(false)}
          onSaved={(f) => { setFlags((prev) => [f, ...prev]); setShowCreate(false); setSelected(f); setShowDetail(true) }} />
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Flags</h1>
        <button onClick={() => setShowCreate(true)} style={{
          display: 'flex', alignItems: 'center', gap: 6,
          background: C.amber, border: 'none', borderRadius: 8,
          color: '#0f1117', padding: '0.6rem 1.1rem', cursor: 'pointer', fontSize: 14, fontWeight: 700,
        }}>
          <Plus size={16} /> Create flag
        </button>
      </div>

      <div className="flags-layout" style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '1.5rem' }}>
        <div className={`flags-list${showDetail ? ' detail-open' : ''}`}>
          {flags.length === 0 ? (
            <div style={{
              background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12,
              padding: '2rem', textAlign: 'center', color: C.muted,
            }}>
              <Flag size={32} style={{ opacity: 0.3, marginBottom: 8 }} />
              <div style={{ fontSize: 14 }}>No flags yet</div>
              <button onClick={() => setShowCreate(true)} style={{
                marginTop: 12, background: 'transparent', border: `1px solid ${C.border}`,
                borderRadius: 7, color: C.amber, padding: '0.4rem 0.8rem', cursor: 'pointer', fontSize: 13, fontWeight: 600,
              }}>
                Create one
              </button>
            </div>
          ) : (
            flags.map((f) => (
              <div key={f.id} onClick={() => { setSelected(f); setShowDetail(true) }} style={{
                padding: '0.875rem 1rem', marginBottom: '0.5rem', cursor: 'pointer',
                background: selected?.id === f.id ? 'rgba(245,158,11,0.1)' : C.surface,
                border: `1px solid ${selected?.id === f.id ? 'rgba(245,158,11,0.4)' : C.border}`,
                borderRadius: 10, transition: 'all 0.15s',
              }}>
                <div style={{ fontWeight: 700, fontSize: 14, color: C.text, marginBottom: 4 }}>{f.name}</div>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <code style={{ fontSize: 12, color: C.muted }}>{f.flag_key}</code>
                  {statusBadge(f.status)}
                </div>
              </div>
            ))
          )}
        </div>

        <div className={`flags-detail${!selected ? ' no-selection' : ''}`} style={{
          background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12,
          padding: '1.5rem', minHeight: 300,
        }}>
          <button className="back-btn" onClick={() => setShowDetail(false)} style={{
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
              onUpdated={(f) => { setSelected(f); setFlags((prev) => prev.map((x) => x.id === f.id ? f : x)) }}
              onDeleted={() => { setFlags((prev) => prev.filter((x) => x.id !== selected.id)); setSelected(null); setShowDetail(false) }}
            />
          )}
        </div>
      </div>
    </Shell>
  )
}
