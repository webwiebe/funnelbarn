// Funnel breakdown: the funnel split by an event property of its first step.
// Mirrors repository.FunnelBreakdownGroup.
import { request } from './api'
import type { StepResult } from './funnel-types'

interface FunnelBreakdownGroup {
  /** The property value; empty for entries without the property. */
  value: string
  /** What to show: the value, "(none)" or "(other)". */
  label: string
  /** True for the group that collects every value beyond the cap. */
  other?: boolean
  entries: number
  results: StepResult[]
}

export interface FunnelBreakdown {
  property: string
  groups: FunnelBreakdownGroup[]
}

export const getFunnelBreakdown = (
  projectId: string,
  funnelId: string,
  property: string,
  opts: { segment?: string; segmentId?: string; environment?: string } = {},
) => {
  const params = new URLSearchParams({ breakdown: property })
  if (opts.segment && opts.segment !== 'all') params.set('segment', opts.segment)
  if (opts.segmentId) params.set('segment_id', opts.segmentId)
  if (opts.environment) params.set('environment', opts.environment)
  return request<{ breakdown: FunnelBreakdown }>(
    `/api/v1/projects/${projectId}/funnels/${funnelId}/analysis?${params.toString()}`,
  ).then((r) => r.breakdown)
}
