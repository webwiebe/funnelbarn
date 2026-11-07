import type { FlagEvaluationResult } from '../../lib/api'

// ENVIRONMENT_SCOPE_NOTE is shown wherever a flag can be changed. A flag
// belongs to the project, so the top-bar environment switcher (an analytics
// filter) has no effect on it.
export const ENVIRONMENT_SCOPE_NOTE =
  'Saves apply to every environment of this project. Only a targeting rule on the environment key limits a value to one.'

// ruleEnvironments lists the environment values a flag's targeting rules name,
// so the panel also evaluates an environment no event has been recorded for.
export function ruleEnvironments(rulesJSON: string): string[] {
  try {
    const rules = JSON.parse(rulesJSON) as Array<{ conditions?: Array<{ context_key?: string; operator?: string; value?: string }> }>
    const out: string[] = []
    for (const rule of rules ?? []) {
      for (const c of rule.conditions ?? []) {
        if (c.context_key !== 'environment' || !c.value) continue
        if (c.operator !== 'eq' && c.operator !== 'in') continue
        for (const v of c.value.split(',')) {
          const name = v.trim()
          if (name && !out.includes(name)) out.push(name)
        }
      }
    }
    return out
  } catch {
    return []
  }
}

// servedText renders what an evaluation returned: the value, or the reason it
// returned none, so a flag that serves nothing is never shown as a blank.
export function servedText(r: FlagEvaluationResult | 'error' | undefined): string {
  if (r === undefined) return '…'
  if (r === 'error') return 'error'
  if (r.error_code) return `error (${r.error_code})`
  if (r.value === undefined || r.value === null || r.value === '') return 'nothing'
  return typeof r.value === 'string' ? r.value : JSON.stringify(r.value)
}
