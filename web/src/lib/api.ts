// Typed API client with 401 interceptor

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
    throw new ApiError(res.status, msg)
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

  getFunnelAnalysis: (projectId: string, funnelId: string) =>
    request<FunnelAnalysis>(`/api/v1/projects/${projectId}/funnels/${funnelId}/analysis`),

  // API Keys
  listApiKeys: () =>
    request<{ api_keys: ApiKey[] }>('/api/v1/apikeys'),

  createApiKey: (name: string, scope: string) =>
    request<{ api_key: ApiKey; key: string }>('/api/v1/apikeys', {
      method: 'POST',
      body: JSON.stringify({ name, scope }),
    }),
}

// Types
export interface Project {
  id: string
  name: string
  domain?: string
}

export interface DashboardData {
  total_events: number
  unique_sessions: number
  bounce_rate: number
  top_pages: { URL: string; Views: number }[]
  top_referrers: { Domain: string; Visits: number }[]
  top_event_names: { Name: string; Count: number }[]
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
  StepOrder: number
  EventName: string
  Count: number
  Conversion: number
  DropOff: number
}

export interface FunnelAnalysis {
  funnel: Funnel
  results: StepResult[]
  from: string
  to: string
}

export interface ApiKey {
  id: string
  name: string
  scope: string
  created_at: string
}
