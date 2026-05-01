// Typed API client with 401 interceptor and BugBarn 5xx reporting
import { reportError } from './bugbarn'

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...options.headers },
    credentials: 'include',
    ...options,
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

  // A/B Tests
  getABTests: (projectId: string) =>
    request<{ tests: ABTest[] }>(`/api/v1/projects/${projectId}/abtests`),

  createABTest: (projectId: string, data: {
    name: string
    conversion_event: string
    control_filter?: { property: string; value: string }
    variant_filter?: { property: string; value: string }
  }) =>
    request<ABTest>(`/api/v1/projects/${projectId}/abtests`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  getABTestAnalysis: (projectId: string, testId: string) =>
    request<ABTestAnalysis>(`/api/v1/projects/${projectId}/abtests/${testId}/analysis`),

  // Active sessions (last 5 minutes)
  getActiveSessions: (projectId: string) =>
    request<{ active_sessions: number; window_minutes: number }>(`/api/v1/projects/${projectId}/sessions/active`),
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
  top_pages: { URL: string; Views: number }[]
  top_referrers: { Domain: string; Visits: number }[]
  top_event_names: { name: string; count: number }[]
  events_time_series: { Time: string; Count: number }[]
}

export interface Event {
  id: string
  name: string
  url: string
  timestamp: string
}

export interface FunnelStep {
  step_order: number
  event_name: string
}

export interface FunnelStepInput {
  event_name: string
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

export interface ABTest {
  id: string
  name: string
  conversion_event: string
  status?: string
  created_at: string
  control_filter?: { property: string; value: string }
  variant_filter?: { property: string; value: string }
}

export interface ABTestAnalysis {
  test: ABTest
  control_sample: number
  control_conversions: number
  variant_sample: number
  variant_conversions: number
  significant: boolean
  z_score?: number
}
