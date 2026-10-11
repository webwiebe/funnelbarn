// Funnel shapes of the REST API, re-exported from api.ts.

export interface FunnelStep {
  step_order: number
  event_name: string
  filters?: { property: string; value: string }[]
}

export interface FunnelStepInput {
  event_name: string
  filters?: { property: string; value: string }[]
}

/**
 * What one funnel entry is. A user funnel follows one user (the user_id
 * events carry) through the steps in order, within the conversion window of
 * step 1; session and page_view funnels count each step on its own.
 */
export type FunnelScope = 'session' | 'page_view' | 'user'

export interface Funnel {
  id: string
  name: string
  description?: string
  scope: FunnelScope
  /** Scope user only; the server uses 30 days when it is unset. */
  conversion_window_seconds?: number
  steps: FunnelStep[]
  /**
   * Step event names this project has never emitted. A funnel that references
   * them can never report a conversion, and otherwise looks identical to a
   * legitimate funnel that nobody has converted through yet. Read-only.
   */
  unmatched_steps?: string[]
}

export interface StepResult {
  step_order: number
  event_name: string
  count: number
  conversion: number
  drop_off: number
}

export interface FunnelAnalysis {
  funnel: Funnel
  results: StepResult[]
  from: string
  to: string
}

export interface FunnelSegments {
  device_types: string[]
  browsers: string[]
  countries: string[]
}
