import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Live from './Live'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('../lib/auth', () => ({
  useAuth: () => ({ user: { id: 'u1', username: 'admin' }, isLoading: false, logout: vi.fn() }),
}))

vi.mock('../lib/projects', () => ({
  useProjects: () => ({
    projects: [{ id: 'p1', name: 'My Site', slug: 'my-site', status: 'active' }],
    isLoading: false,
    refetch: vi.fn(),
    defaultProjectId: 'p1',
    selectedEnvironment: '',
    setSelectedEnvironment: vi.fn(),
  }),
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => vi.fn() }
})

const mockApi = vi.hoisted(() => ({
  getActiveSessions: vi.fn(),
  getEnvironments: vi.fn().mockResolvedValue({ environments: [] }),
  getClientConfig: vi.fn().mockResolvedValue({
    bugbarn_endpoint: '',
    bugbarn_ingest_key: '',
  }),
}))

vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return { ...actual, api: mockApi }
})

// jsdom has neither EventSource nor a real fetch — stub both so the SSE/polling
// effect can run without crashing. The fake EventSource never fires onopen, so
// the component stays in the "Connecting…" state, which keeps the test stable.
class FakeEventSource {
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((e: MessageEvent) => void) | null = null
  close = vi.fn()
  constructor(public url: string) {}
}

beforeEach(() => {
  vi.clearAllMocks()
  mockApi.getActiveSessions.mockResolvedValue({ active_sessions: 7, window_minutes: 5 })
  vi.stubGlobal('EventSource', FakeEventSource as unknown as typeof EventSource)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: async () => ({ events: [] }) }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderLive(projectId: string | null = 'p1') {
  const path = projectId ? `/live/${projectId}` : '/live'
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/live/:projectId" element={<Live />} />
        <Route path="/live" element={<Live />} />
      </Routes>
    </MemoryRouter>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Live', () => {
  it('renders the Live heading', async () => {
    renderLive()
    expect(screen.getByRole('heading', { name: /^live$/i })).toBeInTheDocument()
  })

  it('shows the connecting state initially', async () => {
    renderLive()
    expect(screen.getByText(/connecting/i)).toBeInTheDocument()
  })

  it('fetches active sessions for the project on mount', async () => {
    renderLive('p1')
    await waitFor(() => expect(mockApi.getActiveSessions).toHaveBeenCalledWith('p1'))
  })

  it('renders the active sessions count once loaded', async () => {
    renderLive()
    await waitFor(() => expect(screen.getByText('7')).toBeInTheDocument())
    expect(screen.getByText(/active in last 5 min/i)).toBeInTheDocument()
  })

  it('shows the empty "Waiting for events" state when no events have arrived', async () => {
    renderLive()
    await waitFor(() => expect(screen.getByText(/waiting for events/i)).toBeInTheDocument())
    expect(screen.getByText(/0 events/i)).toBeInTheDocument()
  })

  it('renders the events-per-second sparkline label', async () => {
    renderLive()
    expect(screen.getByText(/events per second/i)).toBeInTheDocument()
  })

  it('opens an EventSource stream against the project events endpoint', async () => {
    const spy = vi.fn()
    vi.stubGlobal(
      'EventSource',
      class extends FakeEventSource {
        constructor(url: string) {
          super(url)
          spy(url)
        }
      } as unknown as typeof EventSource,
    )
    renderLive('p1')
    await waitFor(() => expect(spy).toHaveBeenCalled())
    expect(spy.mock.calls[0][0]).toContain('/api/v1/projects/p1/events?stream=true')
  })

  it('shows the select-a-project prompt when no project id is present', async () => {
    renderLive(null)
    expect(screen.getByText(/select a project to view live stats/i)).toBeInTheDocument()
    // No session fetch without a project id.
    expect(mockApi.getActiveSessions).not.toHaveBeenCalled()
  })

  it('does not crash when active sessions request fails (keeps last value)', async () => {
    mockApi.getActiveSessions.mockRejectedValueOnce(new Error('boom'))
    renderLive()
    // Loading skeleton clears once the (failed) first fetch settles; count falls
    // back to the initial 0.
    await waitFor(() => expect(screen.getByText('0')).toBeInTheDocument())
  })
})
