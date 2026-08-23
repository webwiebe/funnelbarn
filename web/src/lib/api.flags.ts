// Feature-flag API shapes. Split out of api.ts so the flag model can grow
// without pushing that file past its length ratchet.

export type TargetingOperator =
  | 'eq' | 'neq' | 'contains' | 'not_contains' | 'starts_with'
  | 'ends_with' | 'in' | 'not_in' | 'present' | 'not_present'

export interface TargetingCondition {
  context_key: string
  operator: TargetingOperator
  value: string
}

export interface TargetingRule {
  name: string
  variant: string
  match: 'all' | 'any'
  conditions: TargetingCondition[]
}

export interface FeatureFlag {
  id: string
  project_id: string
  flag_key: string
  name: string
  flag_type: string
  variants: string
  default_variant: string
  split: string
  conversion_event?: string
  targeting_rules: string
  status: string
  created_at: string
  /** 'manual' | 'auto' — 'auto' flags were created on first evaluation. */
  origin: string
  last_evaluated_at?: string
  /**
   * 'experiment' — bucketed per user, every read recorded and reported on.
   * 'config' — one value for everyone, no evaluation rows, no variant report.
   */
  flag_kind: string
}

export interface FlagAnalysisVariant {
  variant: string
  sample: number
  conversions: number
  rate: number
}

export interface FlagAnalysis {
  flag: FeatureFlag
  results: FlagAnalysisVariant[]
  significant?: boolean
  z_score?: number
  /** 'config' when the flag records no evaluations, so there is no report. */
  unavailable?: string
}

export interface FlagEvaluationResult {
  flag_key: string
  variant: string
  value: unknown
  reason: string
  error_code?: string
  error?: string
  /** How long this result may be reused without re-evaluating. 0 = don't cache. */
  cache_max_age_seconds?: number
}

export interface FlagEvaluationEntry {
  flag_name: string
  variant: string
  evaluated_at: string
}
