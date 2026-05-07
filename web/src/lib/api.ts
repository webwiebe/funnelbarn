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

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json', ...options.headers as Record<string, string> }

  const method = (options.method || 'GET').toUpperCase()
  if (method === 'POST' || method === 'PUT' || method === 'DELETE' || method === 'PATCH') {
    const csrfToken = getCookie('funnelbarn_csrf')
    if (csrfToken) {
      headers['X-FunnelBarn-CSRF'] = csrfToken
    }
  }

  const res = await fetch(path, {
    ...options,
    headers,
    credentials: 'include',
  })

  if (res.status === 401) {
    // Redirect to login unless we're already there or on the landing page
    if (!window.location.pathname.includes('/login')) {
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
    // Report unexpected server errors (5xx) to BugBarn.
    if (res.status >= 500) {
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
  getDashboard: (projectId: string, range = '7d') =>
    request<DashboardData>(`/api/v1/projects/${projectId}/dashboard?range=${range}`),

  // Events
  getEvents: (projectId: string, limit = 20) =>
    request<{ events: Event[] }>(`/api/v1/projects/${projectId}/events?limit=${limit}`),

  // Funnels
  listFunnels: (projectId: string) =>
    request<{ funnels: Funnel[] }>(`/api/v1/projects/${projectId}/funnels`),

  createFunnel: (projectId: string, name: string, steps: FunnelStepInput[]) =>
    request<Funnel>(`/api/v1/projects/${projectId}/funnels`, {
      method: 'POST',
      body: JSON.stringify({ name, steps }),
    }),

  getFunnelAnalysis: (projectId: string, funnelId: string, segment?: string) =>
    request<FunnelAnalysis>(
      `/api/v1/projects/${projectId}/funnels/${funnelId}/analysis${segment && segment !== 'all' ? `?segment=${encodeURIComponent(segment)}` : ''}`
    ),

  getFunnelSegments: (projectId: string, funnelId: string) =>
    request<FunnelSegments>(`/api/v1/projects/${projectId}/funnels/${funnelId}/segments`),

  updateFunnel: (projectId: string, funnelId: string, data: { name: string; description?: string; steps: FunnelStepInput[] }) =>
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

  createWidget: (projectId: string, data: { event_name: string; property: string; title: string; position?: number }) =>
    request<DashboardWidget>(`/api/v1/projects/${projectId}/widgets`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateWidget: (projectId: string, widgetId: string, data: { event_name: string; property: string; title: string; position?: number }) =>
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

export interface ApiKey {
  id: string
  name: string
  scope: string
  created_at: string
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

export interface DashboardWidget {
  id: string
  project_id: string
  event_name: string
  property: string
  title: string
  position: number
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
