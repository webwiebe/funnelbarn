import { useEffect, useRef, useState } from 'react'
import { Plus, X } from 'lucide-react'
import { api } from '../../lib/api'
import type { FeatureFlag, FlagEvaluationResult, TargetingRule } from '../../lib/api'
import { C } from './shared'

// FlagPlayground is the detail pane's "Try it" tab: evaluate the flag against
// a hand-written context without recording an evaluation.
export function FlagPlayground({ projectId, flag }: { projectId: string; flag: FeatureFlag }) {
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

export function TargetingRulesDisplay({ rulesJSON }: { rulesJSON: string }) {
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
