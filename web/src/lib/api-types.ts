// Response and entity shapes for the API client. Re-exported from ./api.
import type { IambarnConfig } from './iambarn-config'
import type { StepResult } from './funnel-types'

export interface Project {
  id: string
  name: string
  slug: string
  status: string
  domain?: string
}

export interface DashboardData {
  total_events: number
  unique_sessions: number
  bounce_rate: number
  top_pages: { url: string; views: number }[]
  top_referrers: { domain: string; visits: number }[]
  top_event_names: { name: string; count: number }[]
  events_time_series: { time: string; count: number }[]
}

export interface Event {
  id: string
  name: string
  url: string
  occurred_at: string
}

export interface SegmentRule {
  field: string
  operator: 'eq' | 'neq' | 'contains' | 'not_contains' | 'is_null' | 'is_not_null'
  value: string
}

export interface DistributionEntry {
  value: string
  count: number
  pct: number
}

export interface Segment {
  id: string
  project_id: string
  name: string
  rules: SegmentRule[]
  created_at: string
}

export interface ApiKey {
  id: string
  /** The one project this key can act on. Every key is bound to exactly one. */
  project_id: string
  name: string
  /** 'ingest' | 'analytics:read' | 'flags:read' | 'flags:write' | 'full' */
  scope: string
  created_at: string
  /** Absent when the key has never authenticated a request. */
  last_used_at?: string
}

export interface DashboardWidget {
  id: string
  project_id: string
  event_name: string
  property: string
  title: string
  position: number
  size: number
  created_at: string
}

export interface PropertyBreakdown {
  value: string
  count: number
}

export interface WidgetBreakdownResult {
  widget: DashboardWidget
  breakdown: PropertyBreakdown[]
}

export interface ContextKeySuggestion {
  context_key: string
  seen_count: number
  pct: number
}

export interface FlowNode {
  id: string
  label: string
  type: 'page' | 'referrer' | 'exit'
  depth: number
  sessions: number
}

export interface FlowLink {
  source: string
  target: string
  value: number
}

export interface FlowData {
  focused_page: string
  total_sessions: number
  nodes: FlowNode[]
  links: FlowLink[]
}

export interface Recording {
  id: string
  project_id: string
  session_id: string
  environment: string
  first_chunk_index: number
  last_chunk_index: number
  chunk_count: number
  has_snapshot: boolean
  duration_ms: number
  started_at: string
  ended_at?: string
  device_type: string
  is_bot: boolean
  page_url?: string
}

export interface RecordingRule {
  pattern: string
  action: 'capture' | 'ignore'
}

export interface ProjectRecordingSettings {
  enabled: boolean | null
  sample_rate: number | null
  rules: RecordingRule[]
  effective_enabled: boolean
  effective_rate: number
}

export interface ProjectHealth {
  project_id: string
  setup_called: boolean
  events_received: boolean
  flags_evaluated: boolean
  recordings_received: boolean
  updated_at: string
}

export interface ClientConfig {
  bugbarn_endpoint: string
  bugbarn_ingest_key: string
  bugbarn_project?: string
  funnelbarn_endpoint?: string
  funnelbarn_api_key?: string
  funnelbarn_project?: string
  funnelbarn_recording?: boolean
  funnelbarn_recording_rate?: number
  local_auth_available?: boolean
  iambarn?: IambarnConfig
  oidc?: {
    enabled?: boolean
    loginURL?: string
  }
}

// ---- Cross-project overview types ----

interface ProjectRollup {
  project_id: string
  events: number
  unique_sessions: number
}

export interface OverviewData {
  from: string
  to: string
  total_events: number
  unique_sessions: number
  projects: ProjectRollup[]
  visitors_by_project: { day: string; project_id: string; count: number }[]
  top_pages: { project_id: string; url: string; views: number }[]
  top_referrers: { project_id: string; domain: string; visits: number }[]
  top_countries: { project_id: string; country_code: string; count: number }[]
  dimension_breakdown: { value: string; count: number }[] | null
}

export interface OverviewEvent {
  id: string
  project_id: string
  session_id: string
  name: string
  url: string
  referrer_domain: string
  browser: string
  device_type: string
  country_code: string
  environment?: string
  occurred_at: string
}

export interface OverviewEventsResponse {
  events: OverviewEvent[]
  next_cursor: { cursor_time: string; cursor_id: string } | null
}

export interface CanonicalEvent {
  key: string
  label: string
  sort_order: number
}

export interface EventNameMapping {
  project_id: string
  raw_name: string
  canonical_key: string
}

export interface MappingSuggestion {
  raw_name: string
  suggested_key: string
}

interface CanonicalFunnelStep {
  step_order?: number
  canonical_key: string
  label?: string
}

export interface CanonicalFunnel {
  id: string
  name: string
  description?: string
  scope: 'session' | 'page_view'
  project_ids: string[] | null
  segment?: string
  steps: CanonicalFunnelStep[]
  created_at?: string
}

export interface CanonicalFunnelInput {
  name: string
  description?: string
  scope?: 'session' | 'page_view'
  project_ids?: string[]
  segment?: string
  steps: { canonical_key: string }[]
}

interface ProjectFunnelBreakdown {
  project_id: string
  project_name: string
  steps: StepResult[]
}

interface ExcludedProject {
  project_id: string
  project_name: string
  reason: string
}

interface CanonicalFunnelResult {
  steps: StepResult[]
  by_project: ProjectFunnelBreakdown[]
  excluded_projects: ExcludedProject[]
}

export interface CanonicalFunnelAnalysis {
  funnel: CanonicalFunnel
  result: CanonicalFunnelResult
  from: string
  to: string
}
