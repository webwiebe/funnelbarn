import { useEffect, useMemo, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Flag, Plus, X, Pause, Play, Pencil, Trash2, ChevronDown, ChevronRight } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api } from '../lib/api'
import type { FeatureFlag, FlagAnalysis, FlagEvaluationResult, TargetingRule, TargetingOperator } from '../lib/api'
import { useProjects } from '../lib/projects'
import { reportError } from '../lib/bugbarn'
import { Skeleton } from '../components/ui/Skeleton'

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

function statusBadge(status: string) {
  const styles: Record<string, { bg: string; color: string }> = {
    active: { bg: 'rgba(16,185,129,0.1)', color: '#10b981' },
    paused: { bg: 'rgba(245,158,11,0.1)', color: '#f59e0b' },
  }
  const s = styles[status] ?? styles.active
  return (
    <span style={{
      fontSize: 11, fontWeight: 700,
      background: s.bg, color: s.color,
      border: `1px solid ${s.color}33`,
      borderRadius: 99, padding: '0.15rem 0.6rem',
      textTransform: 'uppercase', letterSpacing: '0.05em',
    }}>
      {status}
    </span>
  )
}

function StatCard({ label, sample, conversions, rate, winner }: {
  label: string; sample: number; conversions: number; rate: number; winner: boolean
}) {
  return (
    <div style={{
      flex: 1, background: C.bg,
      border: `1px solid ${winner ? 'rgba(16,185,129,0.4)' : C.border}`,
      borderRadius: 12, padding: '1.25rem', position: 'relative',
    }}>
      {winner && (
        <div style={{
          position: 'absolute', top: 10, right: 10,
          fontSize: 11, fontWeight: 700,
          background: 'rgba(16,185,129,0.15)', color: C.success,
          border: '1px solid rgba(16,185,129,0.3)',
          borderRadius: 99, padding: '0.15rem 0.6rem',
        }}>
          Winner
        </div>
      )}
      <div style={{ fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: '1rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        {label}
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.75rem' }}>
        <div>
          <div style={{ fontSize: 22, fontWeight: 800, color: C.text }}>{sample.toLocaleString()}</div>
          <div style={{ fontSize: 12, color: C.muted }}>evaluations</div>
        </div>
        <div>
          <div style={{ fontSize: 22, fontWeight: 800, color: C.text }}>{conversions.toLocaleString()}</div>
          <div style={{ fontSize: 12, color: C.muted }}>conversions</div>
        </div>
        <div style={{ gridColumn: '1 / -1' }}>
          <div style={{ fontSize: 28, fontWeight: 800, color: winner ? C.success : C.amber }}>
            {(rate * 100).toFixed(2)}%
          </div>
          <div style={{ fontSize: 12, color: C.muted }}>conversion rate</div>
        </div>
      </div>
    </div>
  )
}

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
      const updated = await api.updateFlag(projectId, flag.id, {
        ...flag,
        status: flag.status === 'active' ? 'paused' : 'active',
      })
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
  const [context, setContext] = useState<Array<{ k: string; v: string }>>([{ k: 'user_id', v: '' }])
  const [result, setResult] = useState<FlagEvaluationResult | null>(null)
  const [evaluating, setEvaluating] = useState(false)
  const [requestError, setRequestError] = useState<string | null>(null)

  // Keep flag_key in sync if the user switches to a different flag in the list.
  useEffect(() => { setFlagKey(flag.flag_key) }, [flag.flag_key])

  // Build the context object the API expects. Skip rows with empty keys.
  // Cast numbers and booleans so targeting-rule comparisons line up with what
  // a real SDK would send.
  const contextObject = useMemo(() => {
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
  }, [context])

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
  }, [projectId, flagKey, defaultValue, contextObject])

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

function FlagFormModal({ projectId, flag, onClose, onSaved }: {
  projectId: string
  flag?: FeatureFlag // when present, modal is in edit mode
  onClose: () => void
  onSaved: (f: FeatureFlag) => void
}) {
  const isEdit = flag !== undefined
  const initial = isEdit ? parseFlagFormState(flag) : null

  const [flagKey, setFlagKey] = useState(flag?.flag_key ?? '')
  const [name, setName] = useState(flag?.name ?? '')
  const [flagType, setFlagType] = useState<'boolean' | 'string'>(
    (flag?.flag_type as 'boolean' | 'string') ?? 'boolean'
  )

  // Boolean (release) state
  const [defaultBool, setDefaultBool] = useState<'on' | 'off'>(initial?.defaultBool ?? 'off')
  const [rolloutEnabled, setRolloutEnabled] = useState(initial?.rolloutEnabled ?? false)
  const [rolloutPct, setRolloutPct] = useState(initial?.rolloutPct ?? 0)

  // String (experiment) state
  const [variants, setVariants] = useState<StringVariantRow[]>(initial?.variants ?? [
    { name: 'control', returnValue: 'control', splitPct: 50 },
    { name: 'treatment', returnValue: 'treatment', splitPct: 50 },
  ])
  const [defaultVariant, setDefaultVariant] = useState(initial?.defaultVariant ?? 'control')
  const [advancedReturnValues, setAdvancedReturnValues] = useState(initial?.advancedReturnValues ?? false)

  const [conversionEvent, setConversionEvent] = useState(flag?.conversion_event ?? '')
  const [eventNames, setEventNames] = useState<string[]>([])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [targetingRules, setTargetingRules] = useState<TargetingRule[]>(initial?.targetingRules ?? [])
  const [showRules, setShowRules] = useState((initial?.targetingRules?.length ?? 0) > 0)

  useEffect(() => {
    api.getEventNames(projectId).then((d) => setEventNames(d.event_names || [])).catch(() => {})
  }, [projectId])

  const autoKey = (n: string) => n.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')

  // Variant names targeting-rule dropdowns and default-variant selectors can pick from.
  const variantNames = flagType === 'boolean'
    ? ['on', 'off']
    : variants.map((v) => v.name).filter((n) => n.trim() !== '')

  // Keep defaultVariant in sync when the user removes/renames the row it pointed at.
  useEffect(() => {
    if (flagType !== 'string') return
    if (!variantNames.includes(defaultVariant) && variantNames.length > 0) {
      setDefaultVariant(variantNames[0])
    }
  }, [flagType, variantNames, defaultVariant])

  const splitSum = variants.reduce((sum, v) => sum + (v.splitPct || 0), 0)

  const validateString = (): string | null => {
    if (variants.length < 2) return 'Need at least two variants'
    const trimmedNames = variants.map((v) => v.name.trim())
    if (trimmedNames.some((n) => !n)) return 'All variants need a name'
    if (new Set(trimmedNames).size !== trimmedNames.length) return 'Variant names must be unique'
    if (splitSum !== 100) return `Splits must add up to 100% (currently ${splitSum}%)`
    return null
  }

  const handleSave = async () => {
    if (!flagKey.trim() || !name.trim()) { setError('Key and name are required'); return }

    let variantsObj: Record<string, unknown>
    let splitObj: Record<string, number>
    let resolvedDefault: string

    if (flagType === 'boolean') {
      variantsObj = { on: true, off: false }
      resolvedDefault = defaultBool
      // No rollout → 100% to default. With rollout → that % goes to the
      // *opposite* of the default ("flip X% of users"), the rest stays default.
      if (!rolloutEnabled || rolloutPct === 0) {
        splitObj = { [defaultBool]: 100, [defaultBool === 'on' ? 'off' : 'on']: 0 }
      } else {
        const opposite = defaultBool === 'on' ? 'off' : 'on'
        splitObj = { [defaultBool]: 100 - rolloutPct, [opposite]: rolloutPct }
      }
    } else {
      const err = validateString()
      if (err) { setError(err); return }
      variantsObj = Object.fromEntries(variants.map((v) => [v.name.trim(), v.returnValue]))
      splitObj = Object.fromEntries(variants.map((v) => [v.name.trim(), v.splitPct]))
      resolvedDefault = defaultVariant
    }

    setSaving(true)
    setError(null)
    try {
      const body = {
        flag_key: flagKey,
        name,
        flag_type: flagType,
        variants: JSON.stringify(variantsObj),
        default_variant: resolvedDefault,
        split: JSON.stringify(splitObj),
        conversion_event: conversionEvent,
        targeting_rules: targetingRules.length > 0 ? JSON.stringify(targetingRules) : '[]',
      }
      const f = isEdit && flag
        ? await api.updateFlag(projectId, flag.id, body)
        : await api.createFlag(projectId, body)
      onSaved(f)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  const inputStyle = {
    background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8,
    padding: '0.6rem 0.875rem', color: C.text, fontSize: 14,
    width: '100%', boxSizing: 'border-box' as const, outline: 'none',
  }

  return (
    <>
      <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', zIndex: 1000 }} />
      <div style={{
        position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%, -50%)',
        background: C.surface, border: `1px solid ${C.border}`, borderRadius: 16,
        width: '90%', maxWidth: 520, zIndex: 1001, maxHeight: '90vh', overflowY: 'auto',
      }}>
        <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}`, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>{isEdit ? 'Edit Flag' : 'Create Flag'}</h2>
          <button onClick={onClose} style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer' }}><X size={18} /></button>
        </div>

        <div style={{ padding: '1.5rem' }}>
          {error && (
            <div style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: 8, padding: '0.6rem', color: C.error, fontSize: 13, marginBottom: '1rem' }}>
              {error}
            </div>
          )}

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Name</label>
          <input value={name} onChange={(e) => {
            setName(e.target.value)
            // Auto-sync the key to the name only when creating — once a flag is
            // created its key is the stable identifier consumers reference, so
            // changing it via this auto-derivation would silently break them.
            if (!isEdit && (!flagKey || flagKey === autoKey(name))) setFlagKey(autoKey(e.target.value))
          }}
            placeholder="e.g. Checkout Redesign" style={{ ...inputStyle, marginBottom: 16 }} />

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>
            Key {isEdit && <span style={{ color: C.muted, fontWeight: 400 }}>(immutable)</span>}
          </label>
          <input value={flagKey}
            onChange={(e) => setFlagKey(e.target.value)}
            disabled={isEdit}
            placeholder="e.g. checkout-redesign"
            style={{ ...inputStyle, marginBottom: 16, fontFamily: 'monospace', opacity: isEdit ? 0.6 : 1, cursor: isEdit ? 'not-allowed' : 'text' }} />

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>
            Type {isEdit && <span style={{ color: C.muted, fontWeight: 400 }}>(immutable)</span>}
          </label>
          <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
            {(['boolean', 'string'] as const).map((t) => (
              <button key={t} type="button" onClick={() => { if (!isEdit) setFlagType(t) }} disabled={isEdit} style={{
                flex: 1, padding: '0.5rem', border: `1px solid ${flagType === t ? C.amber : C.border}`,
                borderRadius: 8, background: flagType === t ? 'rgba(245,158,11,0.1)' : 'transparent',
                color: flagType === t ? C.amber : C.muted, fontSize: 13, fontWeight: 600,
                cursor: isEdit ? 'not-allowed' : 'pointer',
                opacity: isEdit && flagType !== t ? 0.5 : 1,
              }}>
                {t}
              </button>
            ))}
          </div>

          {flagType === 'boolean' && (
            <>
              <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Default value</label>
              <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
                {(['on', 'off'] as const).map((v) => (
                  <button key={v} onClick={() => setDefaultBool(v)} type="button" style={{
                    flex: 1, padding: '0.5rem', border: `1px solid ${defaultBool === v ? C.amber : C.border}`,
                    borderRadius: 8, background: defaultBool === v ? 'rgba(245,158,11,0.1)' : 'transparent',
                    color: defaultBool === v ? C.amber : C.muted, fontSize: 13, fontWeight: 600, cursor: 'pointer',
                  }}>
                    {v}
                  </button>
                ))}
              </div>
              <div style={{ marginBottom: 16 }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: C.muted, fontWeight: 600, cursor: 'pointer' }}>
                  <input type="checkbox" checked={rolloutEnabled} onChange={(e) => setRolloutEnabled(e.target.checked)} />
                  Gradual rollout
                </label>
                {rolloutEnabled && (
                  <div style={{ marginTop: 8, padding: '0.75rem', background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8 }}>
                    <div style={{ fontSize: 12, color: C.muted, marginBottom: 6 }}>
                      Flip <span style={{ color: C.amber, fontWeight: 700 }}>{rolloutPct}%</span> of users to{' '}
                      <code style={{ color: C.text }}>{defaultBool === 'on' ? 'off' : 'on'}</code>; the rest stay on the default.
                    </div>
                    <input type="range" min={0} max={100} value={rolloutPct}
                      onChange={(e) => setRolloutPct(Number(e.target.value))}
                      style={{ width: '100%', accentColor: C.amber }} />
                  </div>
                )}
              </div>
            </>
          )}

          {flagType === 'string' && (
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Variants</label>
              {variants.map((v, i) => (
                <div key={i} style={{ display: 'flex', gap: 6, marginBottom: 6, alignItems: 'center' }}>
                  <input
                    type="radio"
                    name="defaultVariant"
                    checked={defaultVariant === v.name}
                    onChange={() => setDefaultVariant(v.name)}
                    title="Default variant"
                    style={{ accentColor: C.amber }}
                  />
                  <input
                    value={v.name}
                    onChange={(e) => {
                      const next = [...variants]
                      const oldName = next[i].name
                      next[i] = { ...v, name: e.target.value, returnValue: advancedReturnValues ? v.returnValue : e.target.value }
                      setVariants(next)
                      if (defaultVariant === oldName) setDefaultVariant(e.target.value)
                    }}
                    placeholder="variant name"
                    style={{ ...inputStyle, flex: 2, fontFamily: 'monospace' }}
                  />
                  {advancedReturnValues && (
                    <input
                      value={v.returnValue}
                      onChange={(e) => {
                        const next = [...variants]; next[i] = { ...v, returnValue: e.target.value }; setVariants(next)
                      }}
                      placeholder="return value"
                      style={{ ...inputStyle, flex: 2 }}
                    />
                  )}
                  <input
                    type="number" min={0} max={100}
                    value={v.splitPct}
                    onChange={(e) => {
                      const next = [...variants]; next[i] = { ...v, splitPct: Number(e.target.value) }; setVariants(next)
                    }}
                    style={{ ...inputStyle, width: 70, textAlign: 'right' }}
                  />
                  <span style={{ color: C.muted, fontSize: 12, width: 14 }}>%</span>
                  <button type="button" onClick={() => {
                    if (variants.length <= 2) return
                    const next = variants.filter((_, idx) => idx !== i); setVariants(next)
                  }} disabled={variants.length <= 2}
                    style={{ background: 'transparent', border: 'none', color: variants.length <= 2 ? '#4a4d5a' : C.muted, cursor: variants.length <= 2 ? 'default' : 'pointer', padding: 2 }}>
                    <X size={14} />
                  </button>
                </div>
              ))}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 6 }}>
                <button type="button"
                  onClick={() => setVariants([...variants, { name: '', returnValue: '', splitPct: 0 }])}
                  style={{ background: 'transparent', border: `1px dashed ${C.border}`, borderRadius: 6, color: C.muted, cursor: 'pointer', padding: '4px 10px', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                  <Plus size={12} /> Add variant
                </button>
                <span style={{ fontSize: 12, color: splitSum === 100 ? C.success : C.error }}>
                  Splits sum: {splitSum}%
                </span>
              </div>
              <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: C.muted, marginTop: 10, cursor: 'pointer' }}>
                <input type="checkbox" checked={advancedReturnValues} onChange={(e) => {
                  setAdvancedReturnValues(e.target.checked)
                  if (!e.target.checked) {
                    setVariants((rows) => rows.map((r) => ({ ...r, returnValue: r.name })))
                  }
                }} />
                Use a different return value than the variant name
              </label>
            </div>
          )}

          <label style={{ display: 'block', fontSize: 13, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Conversion event</label>
          <select value={conversionEvent} onChange={(e) => setConversionEvent(e.target.value)}
            style={{ ...inputStyle, marginBottom: 16 }}>
            <option value="">Select an event (optional)...</option>
            {eventNames.map((n) => <option key={n} value={n}>{n}</option>)}
          </select>

          <div style={{ border: `1px solid ${C.border}`, borderRadius: 10, overflow: 'hidden' }}>
            <button onClick={() => setShowRules(!showRules)} type="button" style={{
              width: '100%', display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '0.75rem 1rem', background: 'transparent', border: 'none',
              color: C.muted, fontSize: 13, fontWeight: 600, cursor: 'pointer',
            }}>
              <span>Targeting Rules ({targetingRules.length})</span>
              {showRules ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            </button>

            {showRules && (
              <div style={{ padding: '0 1rem 1rem', borderTop: `1px solid ${C.border}` }}>
                {targetingRules.map((rule, ri) => {
                  const variantKeys = variantNames
                  return (
                    <div key={ri} style={{ background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8, padding: '0.75rem', marginTop: '0.75rem' }}>
                      <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                        <input value={rule.name} placeholder="Rule name" onChange={(e) => {
                          const updated = [...targetingRules]; updated[ri] = { ...rule, name: e.target.value }; setTargetingRules(updated)
                        }} style={{ ...inputStyle, flex: 2 }} />
                        <select value={rule.variant} onChange={(e) => {
                          const updated = [...targetingRules]; updated[ri] = { ...rule, variant: e.target.value }; setTargetingRules(updated)
                        }} style={{ ...inputStyle, flex: 1 }}>
                          {variantKeys.map((v) => <option key={v} value={v}>{v}</option>)}
                        </select>
                        <button onClick={() => setTargetingRules(targetingRules.filter((_, i) => i !== ri))} type="button"
                          style={{ background: 'transparent', border: 'none', color: C.error, cursor: 'pointer', padding: 4 }}>
                          <X size={14} />
                        </button>
                      </div>

                      <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                        {(['all', 'any'] as const).map((m) => (
                          <button key={m} type="button" onClick={() => {
                            const updated = [...targetingRules]; updated[ri] = { ...rule, match: m }; setTargetingRules(updated)
                          }} style={{
                            padding: '0.25rem 0.6rem', fontSize: 12, fontWeight: 600, cursor: 'pointer',
                            border: `1px solid ${rule.match === m ? C.amber : C.border}`, borderRadius: 6,
                            background: rule.match === m ? 'rgba(245,158,11,0.1)' : 'transparent',
                            color: rule.match === m ? C.amber : C.muted,
                          }}>
                            {m === 'all' ? 'ALL (AND)' : 'ANY (OR)'}
                          </button>
                        ))}
                      </div>

                      {rule.conditions.map((cond, ci) => (
                        <div key={ci} style={{ display: 'flex', gap: 4, marginBottom: 4 }}>
                          <input value={cond.context_key} placeholder="context key" onChange={(e) => {
                            const updated = [...targetingRules]
                            const conds = [...rule.conditions]; conds[ci] = { ...cond, context_key: e.target.value }
                            updated[ri] = { ...rule, conditions: conds }; setTargetingRules(updated)
                          }} style={{ ...inputStyle, flex: 2, fontSize: 12, padding: '0.4rem 0.6rem', fontFamily: 'monospace' }} />
                          <select value={cond.operator} onChange={(e) => {
                            const updated = [...targetingRules]
                            const conds = [...rule.conditions]; conds[ci] = { ...cond, operator: e.target.value as TargetingOperator }
                            updated[ri] = { ...rule, conditions: conds }; setTargetingRules(updated)
                          }} style={{ ...inputStyle, flex: 1.5, fontSize: 12, padding: '0.4rem 0.4rem' }}>
                            {(['eq', 'neq', 'contains', 'not_contains', 'starts_with', 'ends_with', 'in', 'not_in', 'present', 'not_present'] as const).map((op) => (
                              <option key={op} value={op}>{op}</option>
                            ))}
                          </select>
                          {cond.operator !== 'present' && cond.operator !== 'not_present' && (
                            <input value={cond.value} placeholder="value" onChange={(e) => {
                              const updated = [...targetingRules]
                              const conds = [...rule.conditions]; conds[ci] = { ...cond, value: e.target.value }
                              updated[ri] = { ...rule, conditions: conds }; setTargetingRules(updated)
                            }} style={{ ...inputStyle, flex: 2, fontSize: 12, padding: '0.4rem 0.6rem' }} />
                          )}
                          <button onClick={() => {
                            const updated = [...targetingRules]
                            updated[ri] = { ...rule, conditions: rule.conditions.filter((_, i) => i !== ci) }
                            setTargetingRules(updated)
                          }} type="button" style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer', padding: 2 }}>
                            <X size={12} />
                          </button>
                        </div>
                      ))}

                      <button onClick={() => {
                        const updated = [...targetingRules]
                        updated[ri] = { ...rule, conditions: [...rule.conditions, { context_key: '', operator: 'eq', value: '' }] }
                        setTargetingRules(updated)
                      }} type="button" style={{
                        background: 'transparent', border: 'none', color: C.amber,
                        fontSize: 12, cursor: 'pointer', padding: '4px 0', fontWeight: 600,
                      }}>
                        + Add condition
                      </button>
                    </div>
                  )
                })}

                <button onClick={() => setTargetingRules([...targetingRules, {
                  name: '', variant: flagType === 'boolean' ? (defaultBool === 'on' ? 'off' : 'on') : (variantNames[0] || ''),
                  match: 'all', conditions: [{ context_key: '', operator: 'eq', value: '' }],
                }])} type="button" style={{
                  marginTop: '0.75rem', width: '100%', padding: '0.5rem',
                  background: 'transparent', border: `1px dashed ${C.border}`, borderRadius: 8,
                  color: C.amber, fontSize: 13, fontWeight: 600, cursor: 'pointer',
                }}>
                  + Add rule
                </button>
              </div>
            )}
          </div>
        </div>

        <div style={{ padding: '1rem 1.5rem', borderTop: `1px solid ${C.border}`, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button onClick={onClose} style={{ padding: '0.5rem 1rem', background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 8, color: C.text, fontSize: 13, fontWeight: 600, cursor: 'pointer' }}>
            Cancel
          </button>
          <button onClick={handleSave} disabled={saving || !flagKey || !name}
            style={{
              padding: '0.5rem 1rem', border: 'none', borderRadius: 8, fontSize: 13, fontWeight: 700, cursor: saving ? 'default' : 'pointer',
              background: saving || !flagKey || !name ? '#4a4d5a' : C.amber,
              color: saving || !flagKey || !name ? C.muted : '#000',
            }}>
            {saving ? 'Saving...' : (isEdit ? 'Save changes' : 'Create Flag')}
          </button>
        </div>
      </div>
    </>
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
