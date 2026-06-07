// Typed API client with 401 interceptor and BugBarn 5xx reporting
import { reportError } from './bugbarn'

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

function getCookie(name: string): string | undefined {
  const match = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : undefined
}

// Transient statuses we'll retry once. 502/503/504 are almost always a
// k8s rollout in flight (pod cycling) — they recover in a couple of seconds.
const TRANSIENT_STATUSES = new Set([502, 503, 504])

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json', ...options.headers as Record<string, string> }

  const method = (options.method || 'GET').toUpperCase()
  if (method === 'POST' || method === 'PUT' || method === 'DELETE' || method === 'PATCH') {
    const csrfToken = getCookie('funnelbarn_csrf')
    if (csrfToken) {
      headers['X-FunnelBarn-CSRF'] = csrfToken
    }
  }

  const doFetch = () => fetch(path, { ...options, headers, credentials: 'include' })

  let res: Response
  try {
    res = await doFetch()
  } catch (e) {
    // TypeError: Failed to fetch — browser-level network failure. Don't
    // report (it's the user's network, not our code). Surface as an
    // ApiError so callers can render a "lost connection" state if they want.
    throw new ApiError(0, e instanceof Error ? e.message : 'network error')
  }

  // One-shot retry on transient 5xx after a short delay. Don't retry mutating
  // methods — the request may have already partially applied server-side.
  if (TRANSIENT_STATUSES.has(res.status) && method === 'GET') {
    await new Promise((r) => setTimeout(r, 800))
    try {
      res = await doFetch()
    } catch {
      // Same network-failure rationale as above.
      throw new ApiError(0, 'network error after retry')
    }
  }

  if (res.status === 401) {
    const { pathname } = window.location
    if (pathname !== '/' && !pathname.startsWith('/login')) {
      window.location.href = '/login'
    }
    throw new ApiError(401, 'Unauthorized')
  }

  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const body = await res.json()
      if (body.error) msg = body.error
    } catch {
      // ignore parse errors
    }
    const apiErr = new ApiError(res.status, msg)
    // Report unexpected server errors (5xx) to BugBarn — but skip the transient
    // statuses we just retried. If we retried and still got 503, it's likely a
    // longer rollout or real outage; we still don't want a BugBarn spam storm
    // for either case, so suppress those too.
    if (res.status >= 500 && !TRANSIENT_STATUSES.has(res.status)) {
      reportError(apiErr, {
        source: 'api.request',
        path,
        status: res.status,
      })
    }
    throw apiErr
  }

  const text = await res.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}

// Auth
export interface User {
  id: string
  username: string
}

export interface LoginRequest {
  username: string
  password: string
}

export const api = {
  getClientConfig: () =>
    request<ClientConfig>('/api/v1/client-config'),

  getContextKeySuggestions: (projectId: string) =>
    request<{ suggestions: ContextKeySuggestion[] }>(`/api/v1/projects/${projectId}/flags/context-keys`),

  login: (body: LoginRequest) =>
    request<User>('/api/v1/login', { method: 'POST', body: JSON.stringify(body) }),

  logout: () =>
    request<void>('/api/v1/logout', { method: 'POST' }),

  me: () =>
    request<User>('/api/v1/me'),

  // Projects
  listProjects: () =>
    request<{ projects: Project[] }>('/api/v1/projects'),

  createProject: (name: string, domain: string) =>
    request<Project>('/api/v1/projects', {
      method: 'POST',
      body: JSON.stringify({ name, domain }),
    }),

  // Dashboard
  getDashboard: (projectId: string, range = '7d', env = '') => {
    const qs = new URLSearchParams({ range })
    if (env) qs.set('environment', env)
    return request<DashboardData>(`/api/v1/projects/${projectId}/dashboard?${qs}`)
  },

  // Environments
  getEnvironments: (projectId: string) =>
    request<{ environments: string[] }>(`/api/v1/projects/${projectId}/environments`),

  // Events
  getEvents: (projectId: string, limit = 20) =>
    request<{ events: Event[] }>(`/api/v1/projects/${projectId}/events?limit=${limit}`),

  // Funnels
  listFunnels: (projectId: string) =>
    request<{ funnels: Funnel[] }>(`/api/v1/projects/${projectId}/funnels`),

  createFunnel: (projectId: string, name: string, steps: FunnelStepInput[], scope?: 'session' | 'page_view') =>
    request<Funnel>(`/api/v1/projects/${projectId}/funnels`, {
      method: 'POST',
      body: JSON.stringify({ name, steps, scope: scope ?? 'session' }),
    }),

  getFunnelAnalysis: (projectId: string, funnelId: string, segment?: string, segmentId?: string) => {
    const params = new URLSearchParams()
    if (segment && segment !== 'all') params.set('segment', segment)
    if (segmentId) params.set('segment_id', segmentId)
    const qs = params.toString()
    return request<FunnelAnalysis>(`/api/v1/projects/${projectId}/funnels/${funnelId}/analysis${qs ? `?${qs}` : ''}`)
  },

  getFunnelSegments: (projectId: string, funnelId: string) =>
    request<FunnelSegments>(`/api/v1/projects/${projectId}/funnels/${funnelId}/segments`),

  updateFunnel: (projectId: string, funnelId: string, data: { name: string; description?: string; scope?: string; steps: FunnelStepInput[] }) =>
    request<Funnel>(`/api/v1/projects/${projectId}/funnels/${funnelId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteFunnel: (projectId: string, funnelId: string) =>
    request<void>(`/api/v1/projects/${projectId}/funnels/${funnelId}`, { method: 'DELETE' }),

  // API Keys
  listApiKeys: () =>
    request<{ api_keys: ApiKey[] }>('/api/v1/apikeys'),

  createApiKey: (name: string, scope: string, projectId?: string) =>
    request<{ api_key: ApiKey; key: string }>('/api/v1/apikeys', {
      method: 'POST',
      body: JSON.stringify({ name, scope, project_id: projectId }),
    }),

  deleteApiKey: (keyId: string) =>
    request<void>(`/api/v1/apikeys/${keyId}`, { method: 'DELETE' }),

  // Project update
  updateProject: (projectId: string, data: { name: string; domain?: string }) =>
    request<Project>(`/api/v1/projects/${projectId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteProject: (projectId: string) =>
    request<void>(`/api/v1/projects/${projectId}`, { method: 'DELETE' }),

  approveProject: (projectId: string) =>
    request<Project>(`/api/v1/projects/${projectId}/approve`, { method: 'POST' }),

  // Feature Flags
  listFlags: (projectId: string) =>
    request<{ flags: FeatureFlag[] }>(`/api/v1/projects/${projectId}/flags`),

  createFlag: (projectId: string, data: {
    name: string
    flag_key: string
    flag_type: string
    variants: string
    default_variant: string
    split: string
    conversion_event?: string
    targeting_rules?: string
  }) =>
    request<FeatureFlag>(`/api/v1/projects/${projectId}/flags`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateFlag: (projectId: string, flagId: string, data: Partial<{
    name: string
    variants: string
    default_variant: string
    split: string
    conversion_event: string
    targeting_rules: string
    status: string
  }>) =>
    request<FeatureFlag>(`/api/v1/projects/${projectId}/flags/${flagId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteFlag: (projectId: string, flagId: string) =>
    request<void>(`/api/v1/projects/${projectId}/flags/${flagId}`, { method: 'DELETE' }),

  getFlagAnalysis: (projectId: string, flagId: string) =>
    request<FlagAnalysis>(`/api/v1/projects/${projectId}/flags/${flagId}/analysis`),

  evaluateFlagPlayground: (
    projectId: string,
    body: { flag_key: string; default_value?: unknown; context: Record<string, unknown> },
  ) =>
    request<FlagEvaluationResult>(`/api/v1/projects/${projectId}/flags/evaluate`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  // Event names (autocomplete)
  getEventNames: (projectId: string) =>
    request<{ event_names: string[] }>(`/api/v1/projects/${projectId}/event-names`),

  // Event properties (autocomplete for filters)
  getEventProperties: (projectId: string, eventName: string) =>
    request<{ properties: string[] }>(`/api/v1/projects/${projectId}/event-properties?event_name=${encodeURIComponent(eventName)}`),

  getEventPropertyValues: (projectId: string, eventName: string, property: string) =>
    request<{ values: string[] }>(`/api/v1/projects/${projectId}/event-property-values?event_name=${encodeURIComponent(eventName)}&property=${encodeURIComponent(property)}`),

  // Active sessions (last 5 minutes)
  getActiveSessions: (projectId: string) =>
    request<{ active_sessions: number; window_minutes: number }>(`/api/v1/projects/${projectId}/sessions/active`),

  // Widgets (Insights)
  listWidgets: (projectId: string) =>
    request<{ widgets: DashboardWidget[] }>(`/api/v1/projects/${projectId}/widgets`),

  createWidget: (projectId: string, data: { event_name: string; property: string; title: string; position?: number; size?: number }) =>
    request<DashboardWidget>(`/api/v1/projects/${projectId}/widgets`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateWidget: (projectId: string, widgetId: string, data: { event_name: string; property: string; title: string; position?: number; size?: number }) =>
    request<DashboardWidget>(`/api/v1/projects/${projectId}/widgets/${widgetId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteWidget: (projectId: string, widgetId: string) =>
    request<void>(`/api/v1/projects/${projectId}/widgets/${widgetId}`, { method: 'DELETE' }),

  getWidgetBreakdown: (projectId: string, widgetId: string) =>
    request<{ widget: DashboardWidget; breakdown: PropertyBreakdown[]; window: number }>(
      `/api/v1/projects/${projectId}/widgets/${widgetId}/breakdown`
    ),

  getBatchBreakdowns: (projectId: string) =>
    request<{ results: WidgetBreakdownResult[] }>(`/api/v1/projects/${projectId}/widgets/breakdowns`),

  // Session distributions (for segment charts)
  getSessionDistributions: (projectId: string) =>
    request<{ distributions: Record<string, DistributionEntry[]> }>(`/api/v1/projects/${projectId}/session-distributions`),

  // Segments
  listSegments: (projectId: string) =>
    request<{ segments: Segment[] }>(`/api/v1/projects/${projectId}/segments`),

  createSegment: (projectId: string, name: string, rules: SegmentRule[]) =>
    request<Segment>(`/api/v1/projects/${projectId}/segments`, {
      method: 'POST',
      body: JSON.stringify({ name, rules }),
    }),

  updateSegment: (projectId: string, segmentId: string, name: string, rules: SegmentRule[]) =>
    request<Segment>(`/api/v1/projects/${projectId}/segments/${segmentId}`, {
      method: 'PUT',
      body: JSON.stringify({ name, rules }),
    }),

  deleteSegment: (projectId: string, segmentId: string) =>
    request<void>(`/api/v1/projects/${projectId}/segments/${segmentId}`, { method: 'DELETE' }),

  // Instance settings
  getInstanceSettings: () =>
    request<{ settings: Record<string, string> }>('/api/v1/instance-settings'),

  setInstanceSettings: (settings: Record<string, string>) =>
    request<{ settings: Record<string, string> }>('/api/v1/instance-settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),

  // Geo anonymization
  anonymizeGeo: (params: { session_id?: string; ip?: string }) =>
    request<{ anonymized: number }>('/api/v1/admin/anonymize-geo', {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  // Page flows
  getPageFlows: (projectId: string, params: { page?: string; range?: string; from?: string; to?: string; depth?: number; environment?: string }) => {
    const qs = new URLSearchParams()
    if (params.page) qs.set('page', params.page)
    if (params.range) qs.set('range', params.range)
    if (params.from) qs.set('from', params.from)
    if (params.to) qs.set('to', params.to)
    if (params.depth) qs.set('depth', String(params.depth))
    if (params.environment) qs.set('environment', params.environment)
    const q = qs.toString()
    return request<FlowData>(`/api/v1/projects/${projectId}/flows${q ? `?${q}` : ''}`)
  },

  // Recordings
  listRecordings: (projectId: string, params: {
    limit?: number; offset?: number; environment?: string; session_ids?: string[]
    device_type?: string; human_only?: boolean; page_url?: string
  } = {}) => {
    const qs = new URLSearchParams()
    if (params.limit) qs.set('limit', String(params.limit))
    if (params.offset) qs.set('offset', String(params.offset))
    if (params.environment) qs.set('environment', params.environment)
    if (params.session_ids?.length) qs.set('session_ids', params.session_ids.join(','))
    if (params.device_type) qs.set('device_type', params.device_type)
    if (params.human_only) qs.set('human_only', 'true')
    if (params.page_url) qs.set('page_url', params.page_url)
    const q = qs.toString()
    return request<{ recordings: Recording[]; limit: number; offset: number }>(
      `/api/v1/projects/${projectId}/recordings${q ? `?${q}` : ''}`
    )
  },

  getRecordingChunk: (projectId: string, recordingId: string, index: number) =>
    request<unknown[]>(`/api/v1/projects/${projectId}/recordings/${recordingId}/chunks/${index}`),

  getRecordingFlags: (projectId: string, recordingId: string) =>
    request<{ evaluations: FlagEvaluationEntry[] }>(`/api/v1/projects/${projectId}/recordings/${recordingId}/flags`),

  getFunnelStepSessions: (projectId: string, funnelId: string, step: number, params: { from?: string; to?: string } = {}) => {
    const qs = new URLSearchParams()
    if (params.from) qs.set('from', params.from)
    if (params.to) qs.set('to', params.to)
    const q = qs.toString()
    return request<{ session_ids: string[] }>(
      `/api/v1/projects/${projectId}/funnels/${funnelId}/steps/${step}/sessions${q ? `?${q}` : ''}`
    )
  },

  getFlowPageSessions: (projectId: string, page: string, params: { from?: string; to?: string } = {}) => {
    const qs = new URLSearchParams({ page })
    if (params.from) qs.set('from', params.from)
    if (params.to) qs.set('to', params.to)
    return request<{ session_ids: string[] }>(`/api/v1/projects/${projectId}/flows/sessions?${qs}`)
  },
}

// Types
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

export interface FunnelStep {
  step_order: number
  event_name: string
  filters?: { property: string; value: string }[]
}

export interface FunnelStepInput {
  event_name: string
  filters?: { property: string; value: string }[]
}

export interface Funnel {
  id: string
  name: string
  description?: string
  scope: 'session' | 'page_view'
  steps: FunnelStep[]
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
  name: string
  scope: string
  created_at: string
}

export type TargetingOperator = 'eq' | 'neq' | 'contains' | 'not_contains' | 'starts_with' | 'ends_with' | 'in' | 'not_in' | 'present' | 'not_present'

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
}

export interface FlagEvaluationResult {
  flag_key: string
  variant: string
  value: unknown
  reason: string
  error_code?: string
  error?: string
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
  chunk_count: number
  duration_ms: number
  started_at: string
  ended_at?: string
  device_type: string
  is_bot: boolean
  page_url?: string
}

export interface FlagEvaluationEntry {
  flag_name: string
  variant: string
  evaluated_at: string
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
  iambarn_enabled: boolean
  local_auth_available?: boolean
  iambarn?: {
    profile_url?: string
  }
  oidc?: {
    enabled?: boolean
    loginURL?: string
  }
}
