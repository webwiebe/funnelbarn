import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Dashboard from './Dashboard'
import type { DashboardData } from '../lib/api'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('../lib/auth', () => ({
  useAuth: () => ({ user: { id: 'u1', username: 'admin' }, isLoading: false }),
}))

vi.mock('../lib/projects', () => ({
  useProjects: () => ({
    projects: [{ id: 'p1', name: 'My Site', slug: 'my-site', status: 'active' }],
    isLoading: false,
    refetch: vi.fn(),
    defaultProjectId: 'p1',
  }),
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => vi.fn() }
})

const mockApi = vi.hoisted(() => ({
  getDashboard: vi.fn(),
  getActiveSessions: vi.fn(),
  createProject: vi.fn(),
  listFunnels: vi.fn(),
  getFunnelAnalysis: vi.fn(),
}))

vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return { ...actual, api: mockApi }
})

const mockDashboard: DashboardData = {
  total_events: 42,
  unique_sessions: 7,
  bounce_rate: 0.42,
  top_pages: [{ URL: '/home', Views: 30 }],
  top_referrers: [{ Domain: 'google.com', Visits: 10 }],
  top_event_names: [{ name: 'page_view', count: 30 }],
  events_time_series: [{ Time: '2024-01-01T00:00:00Z', Count: 42 }],
}

function renderDashboard(projectId = 'p1') {
  return render(
    <MemoryRouter initialEntries={[`/dashboard/${projectId}`]}>
      <Routes>
        <Route path="/dashboard/:projectId" element={<Dashboard />} />
      </Routes>
    </MemoryRouter>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Dashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.getDashboard.mockResolvedValue(mockDashboard)
    mockApi.getActiveSessions.mockResolvedValue({ active_sessions: 7, window_minutes: 5 })
    mockApi.listFunnels.mockResolvedValue({ funnels: [] })
  })

  it('renders the overview heading', async () => {
    renderDashboard()
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /overview/i })).toBeInTheDocument(),
    )
  })

  it('shows total events stat card label', async () => {
    renderDashboard()
    await waitFor(() => expect(screen.getByText('Total Events')).toBeInTheDocument())
  })

  it('shows unique sessions label', async () => {
    renderDashboard()
    await waitFor(() => expect(screen.getByText('Unique Sessions')).toBeInTheDocument())
  })

  it('shows top page URL after data loads', async () => {
    renderDashboard()
    await waitFor(() => expect(screen.getByText('/home')).toBeInTheDocument())
  })

  it('does not show loading skeleton after data loads', async () => {
    renderDashboard()
    await waitFor(() => expect(screen.getByText('Total Events')).toBeInTheDocument())
    // getDashboard resolved — no error banner should be visible
    expect(screen.queryByText(/error/i)).toBeNull()
  })

  it('calls getDashboard with the correct project id', async () => {
    renderDashboard('p1')
    await waitFor(() =>
      expect(mockApi.getDashboard).toHaveBeenCalledWith('p1', expect.any(String)),
    )
  })

  it('calls listFunnels with the correct project id', async () => {
    renderDashboard('p1')
    await waitFor(() => expect(mockApi.listFunnels).toHaveBeenCalledWith('p1'))
  })
})
