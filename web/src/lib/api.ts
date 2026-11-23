// Typed API client with 401 interceptor and BugBarn 5xx reporting
import { ApiError } from './api-error'
import { buildHeaders, failFor, fetchWithRetry } from './request-core'
import type { LogoutResponse } from './auth-types'
import type { Funnel, FunnelAnalysis, FunnelScope, FunnelSegments, FunnelStepInput } from './funnel-types'
export type { Funnel, FunnelAnalysis, FunnelScope, FunnelSegments, FunnelStepInput, StepResult } from './funnel-types'

export { ApiError }
import type { Project, DashboardData, Event, SegmentRule, DistributionEntry, Segment, ApiKey, DashboardWidget, PropertyBreakdown, WidgetBreakdownResult, ContextKeySuggestion, FlowData, Recording, RecordingRule, ProjectRecordingSettings, ProjectHealth, ClientConfig, OverviewData, OverviewEventsResponse, CanonicalEvent, EventNameMapping, MappingSuggestion, CanonicalFunnel, CanonicalFunnelInput, CanonicalFunnelAnalysis } from './api-types'
export type * from './api-types'

export async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const method = (options.method || 'GET').toUpperCase()
  const headers = buildHeaders(options, method)
  const res = await fetchWithRetry(() => fetch(path, { ...options, headers, credentials: 'include' }), method)
  if (!res.ok) return failFor(res, path)

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
    request<LogoutResponse | undefined>('/api/v1/logout', { method: 'POST' }),

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

  createFunnel: (projectId: string, name: string, steps: FunnelStepInput[], scope?: FunnelScope, conversionWindowSeconds?: number) =>
    request<Funnel>(`/api/v1/projects/${projectId}/funnels`, {
      method: 'POST',
      body: JSON.stringify({ name, steps, scope: scope ?? 'session', conversion_window_seconds: conversionWindowSeconds }),
    }),

  getFunnelAnalysis: (projectId: string, funnelId: string, segment?: string, segmentId?: string, environment?: string) => {
    const params = new URLSearchParams()
    if (segment && segment !== 'all') params.set('segment', segment)
    if (segmentId) params.set('segment_id', segmentId)
    if (environment) params.set('environment', environment)
    const qs = params.toString()
    return request<FunnelAnalysis>(`/api/v1/projects/${projectId}/funnels/${funnelId}/analysis${qs ? `?${qs}` : ''}`)
  },
  getFunnelSegments: (projectId: string, funnelId: string) =>
    request<FunnelSegments>(`/api/v1/projects/${projectId}/funnels/${funnelId}/segments`),

  updateFunnel: (projectId: string, funnelId: string, data: { name: string; description?: string; scope?: FunnelScope; conversion_window_seconds?: number; steps: FunnelStepInput[] }) =>
    request<Funnel>(`/api/v1/projects/${projectId}/funnels/${funnelId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteFunnel: (projectId: string, funnelId: string) =>
    request<void>(`/api/v1/projects/${projectId}/funnels/${funnelId}`, { method: 'DELETE' }),

  // ---- Cross-project overview ----
  getOverview: (range = '7d', env = '', dimension = '') => {
    const qs = new URLSearchParams({ range })
    if (env) qs.set('environment', env)
    if (dimension) qs.set('dimension', dimension)
    return request<OverviewData>(`/api/v1/overview?${qs}`)
  },

  getOverviewEvents: (params: { projectId?: string; name?: string; environment?: string; cursorTime?: string; cursorId?: string; limit?: number } = {}) => {
    const qs = new URLSearchParams()
    if (params.projectId) qs.set('project_id', params.projectId)
    if (params.name) qs.set('name', params.name)
    if (params.environment) qs.set('environment', params.environment)
    if (params.cursorTime) qs.set('cursor_time', params.cursorTime)
    if (params.cursorId) qs.set('cursor_id', params.cursorId)
    qs.set('limit', String(params.limit ?? 50))
    return request<OverviewEventsResponse>(`/api/v1/overview/events?${qs}`)
  },

  // ---- Canonical event catalog ----
  listCanonicalEvents: () =>
    request<{ canonical_events: CanonicalEvent[] }>('/api/v1/canonical-events'),

  createCanonicalEvent: (body: CanonicalEvent) =>
    request<CanonicalEvent>('/api/v1/canonical-events', { method: 'POST', body: JSON.stringify(body) }),

  updateCanonicalEvent: (key: string, body: { label: string; sort_order: number }) =>
    request<CanonicalEvent>(`/api/v1/canonical-events/${encodeURIComponent(key)}`, { method: 'PUT', body: JSON.stringify(body) }),

  deleteCanonicalEvent: (key: string) =>
    request<void>(`/api/v1/canonical-events/${encodeURIComponent(key)}`, { method: 'DELETE' }),

  // ---- Per-project event-name mappings ----
  listMappings: (projectId: string) =>
    request<{ mappings: EventNameMapping[] }>(`/api/v1/projects/${projectId}/event-mappings`),

  getMappingSuggestions: (projectId: string) =>
    request<{ suggestions: MappingSuggestion[] }>(`/api/v1/projects/${projectId}/event-mappings/suggestions`),

  setMappings: (projectId: string, mappings: { raw_name: string; canonical_key: string }[]) =>
    request<{ mappings: EventNameMapping[] }>(`/api/v1/projects/${projectId}/event-mappings`, {
      method: 'PUT',
      body: JSON.stringify({ mappings }),
    }),

  deleteMapping: (projectId: string, rawName: string) =>
    request<void>(`/api/v1/projects/${projectId}/event-mappings/${encodeURIComponent(rawName)}`, { method: 'DELETE' }),

  // ---- Cross-project canonical funnels ----
  listCanonicalFunnels: () =>
    request<{ funnels: CanonicalFunnel[] }>('/api/v1/overview/funnels'),

  createCanonicalFunnel: (body: CanonicalFunnelInput) =>
    request<CanonicalFunnel>('/api/v1/overview/funnels', { method: 'POST', body: JSON.stringify(body) }),

  updateCanonicalFunnel: (id: string, body: CanonicalFunnelInput) =>
    request<CanonicalFunnel>(`/api/v1/overview/funnels/${id}`, { method: 'PUT', body: JSON.stringify(body) }),

  deleteCanonicalFunnel: (id: string) =>
    request<void>(`/api/v1/overview/funnels/${id}`, { method: 'DELETE' }),

  analyzeCanonicalFunnel: (id: string, opts: { from?: string; to?: string; segment?: string; projectIds?: string[] } = {}) => {
    const qs = new URLSearchParams()
    if (opts.from) qs.set('from', opts.from)
    if (opts.to) qs.set('to', opts.to)
    if (opts.segment !== undefined && opts.segment !== 'all') qs.set('segment', opts.segment)
    if (opts.projectIds && opts.projectIds.length) qs.set('project_ids', opts.projectIds.join(','))
    const q = qs.toString()
    return request<CanonicalFunnelAnalysis>(`/api/v1/overview/funnels/${id}/analysis${q ? `?${q}` : ''}`)
  },

  // API Keys
  // Without a projectId this returns every project's keys, which is only ever
  // what an instance-wide view wants — a project page must pass its own id.
  listApiKeys: (projectId?: string) =>
    request<{ api_keys: ApiKey[] }>(
      projectId ? `/api/v1/apikeys?project_id=${encodeURIComponent(projectId)}` : '/api/v1/apikeys',
    ),

  createApiKey: (name: string, scope: string, projectId: string) =>
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
  getActiveSessions: (projectId: string, environment?: string) =>
    request<{ active_sessions: number; window_minutes: number }>(
      `/api/v1/projects/${projectId}/sessions/active${environment ? `?environment=${encodeURIComponent(environment)}` : ''}`,
    ),

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
  getSessionDistributions: (projectId: string, environment?: string) =>
    request<{ distributions: Record<string, DistributionEntry[]> }>(
      `/api/v1/projects/${projectId}/session-distributions${environment ? `?environment=${encodeURIComponent(environment)}` : ''}`,
    ),

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

  deleteRecording: (projectId: string, recordingId: string) =>
    request<void>(`/api/v1/projects/${projectId}/recordings/${recordingId}`, { method: 'DELETE' }),

  getRecordingFlags: (projectId: string, recordingId: string) =>
    request<{ evaluations: FlagEvaluationEntry[] }>(`/api/v1/projects/${projectId}/recordings/${recordingId}/flags`),

  getProjectRecordingSettings: (projectId: string) =>
    request<ProjectRecordingSettings>(`/api/v1/projects/${projectId}/recording-settings`),

  updateProjectRecordingSettings: (
    projectId: string,
    settings: { enabled: boolean | null; sample_rate: number | null; rules: RecordingRule[] }
  ) =>
    request<void>(`/api/v1/projects/${projectId}/recording-settings`, {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),

  getFunnelStepSessions: (projectId: string, funnelId: string, step: number, params: { from?: string; to?: string; environment?: string } = {}) => {
    const qs = new URLSearchParams()
    for (const [key, value] of Object.entries(params)) if (value) qs.set(key, value)
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

  // Project integration health
  getProjectHealth: (projectId: string) =>
    request<ProjectHealth>(`/api/v1/projects/${projectId}/health`),

  resetProjectHealth: (projectId: string) =>
    request<void>(`/api/v1/projects/${projectId}/health/reset`, { method: 'POST' }),
}

// Feature-flag shapes live in ./api.flags and are re-exported here so every
// existing `from '../lib/api'` import keeps working.
import type {
  FeatureFlag,
  FlagAnalysis,
  FlagEvaluationResult,
  FlagEvaluationEntry,
} from './api.flags'

export type {
  TargetingOperator,
  TargetingRule,
  FeatureFlag,
  FlagAnalysis,
  FlagEvaluationResult,
  FlagEvaluationEntry,
} from './api.flags'
