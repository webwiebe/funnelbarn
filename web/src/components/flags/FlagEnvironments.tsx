import { useEffect, useMemo, useState } from 'react'
import { api } from '../../lib/api'
import type { FeatureFlag, FlagEvaluationResult } from '../../lib/api'
import { C } from './shared'
import { ENVIRONMENT_SCOPE_NOTE, ruleEnvironments, servedText } from './environmentScope'

// FlagEnvironments shows what the flag serves right now in each environment of
// the project, by evaluating it without recording the evaluation.
export function FlagEnvironments({ projectId, flag }: { projectId: string; flag: FeatureFlag }) {
  const [recorded, setRecorded] = useState<string[]>([])
  const [served, setServed] = useState<Record<string, FlagEvaluationResult | 'error'>>({})

  useEffect(() => {
    let live = true
    api.getEnvironments(projectId)
      .then((d) => { if (live) setRecorded(d.environments ?? []) })
      .catch(() => { if (live) setRecorded([]) })
    return () => { live = false }
  }, [projectId])

  const environments = useMemo(() => {
    const names = [...recorded]
    for (const n of ruleEnvironments(flag.targeting_rules)) if (!names.includes(n)) names.push(n)
    return names
  }, [recorded, flag.targeting_rules])

  // The flag's own fields are part of the key so a toggle or an edit refreshes it.
  const stamp = [flag.id, flag.status, flag.default_variant, flag.variants, flag.split, flag.targeting_rules].join('|')
  useEffect(() => {
    let live = true
    setServed({})
    for (const environment of environments) {
      api.evaluateFlagPlayground(projectId, { flag_key: flag.flag_key, context: { environment } })
        .then((r) => { if (live) setServed((s) => ({ ...s, [environment]: r })) })
        .catch(() => { if (live) setServed((s) => ({ ...s, [environment]: 'error' })) })
    }
    return () => { live = false }
  }, [projectId, flag.flag_key, environments, stamp])

  return (
    <div data-testid="flag-environments" style={{
      background: C.bg, border: `1px solid ${C.border}`, borderRadius: 10,
      padding: '0.75rem 1.25rem', marginBottom: 16, fontSize: 13, color: C.muted,
    }}>
      <div style={{ fontWeight: 600, marginBottom: 6 }}>Serves now, per environment</div>
      {environments.length === 0 ? (
        <div>No environment has sent events yet, so there is nothing to evaluate.</div>
      ) : (
        <div style={{ display: 'flex', gap: 14, flexWrap: 'wrap' }}>
          {environments.map((environment) => (
            <span key={environment}>
              {environment}: <code style={{ color: C.text }}>{servedText(served[environment])}</code>
            </span>
          ))}
        </div>
      )}
      <div style={{ marginTop: 8, fontSize: 12 }}>{ENVIRONMENT_SCOPE_NOTE}</div>
    </div>
  )
}
