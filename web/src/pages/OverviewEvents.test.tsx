import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import OverviewEvents from './OverviewEvents'
import type { OverviewEvent } from '../lib/api'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('../lib/auth', () => ({
  useAuth: () => ({ user: { id: 'u1', username: 'admin' }, isLoading: false, logout: vi.fn() }),
}))

vi.mock('../lib/projects', () => ({
  useProjects: () => ({
    projects: [
      { id: 'p1', name: 'My Site', slug: 'my-site', status: 'active' },
      { id: 'p2', name: 'Other Site', slug: 'other-site', status: 'active' },
    ],
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
  getOverviewEvents: vi.fn(),
  getClientConfig: vi.fn().mockResolvedValue({
    bugbarn_endpoint: '',
    bugbarn_ingest_key: '',
    iambarn_enabled: false,
  }),
}))

vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return { ...actual, api: mockApi }
})

const mockEvents: OverviewEvent[] = [
  {
    id: 'e1',
    project_id: 'p1',
    session_id: 's1',
    name: 'page_view',
    url: '/home',
    referrer_domain: 'google.com',
    browser: 'Chrome',
    device_type: 'desktop',
    country_code: 'NL',
    occurred_at: '2024-01-01T12:00:00Z',
  },
]

function renderPage() {
  return render(
    <MemoryRouter>
      <OverviewEvents />
    </MemoryRouter>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('OverviewEvents', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.getOverviewEvents.mockResolvedValue({
      events: mockEvents,
      next_cursor: { cursor_time: '2024-01-01T11:00:00Z', cursor_id: 'e0' },
    })
  })

  it('renders the All Events heading', async () => {
    renderPage()
    expect(screen.getByRole('heading', { name: /all events/i })).toBeInTheDocument()
  })

  it('loads and displays events on mount', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('page_view')).toBeInTheDocument())
    expect(screen.getByText('/home')).toBeInTheDocument()
    // project id is resolved to the project name (appears in the table cell and
    // in the filter <select> option, so multiple matches are expected).
    expect(screen.getAllByText('My Site').length).toBeGreaterThan(0)
    expect(mockApi.getOverviewEvents).toHaveBeenCalled()
  })

  it('filters by event name', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('page_view')).toBeInTheDocument())

    fireEvent.change(screen.getByPlaceholderText(/filter by event name/i), {
      target: { value: 'signup' },
    })
    await waitFor(() =>
      expect(mockApi.getOverviewEvents).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'signup' }),
      ),
    )
  })

  it('filters by project', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('page_view')).toBeInTheDocument())

    const projectSelect = screen.getByRole('combobox')
    fireEvent.change(projectSelect, { target: { value: 'p2' } })
    await waitFor(() =>
      expect(mockApi.getOverviewEvents).toHaveBeenCalledWith(
        expect.objectContaining({ projectId: 'p2' }),
      ),
    )
  })

  it('pages forward with the next cursor', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('page_view')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /older/i }))
    await waitFor(() =>
      expect(mockApi.getOverviewEvents).toHaveBeenCalledWith(
        expect.objectContaining({ cursorTime: '2024-01-01T11:00:00Z', cursorId: 'e0' }),
      ),
    )
  })

  it('shows the empty state when there are no events', async () => {
    mockApi.getOverviewEvents.mockResolvedValue({ events: [], next_cursor: null })
    renderPage()
    await waitFor(() => expect(screen.getByText(/no events found/i)).toBeInTheDocument())
  })

  it('shows an error banner when loading fails', async () => {
    mockApi.getOverviewEvents.mockRejectedValueOnce(new Error('load blew up'))
    renderPage()
    await waitFor(() => expect(screen.getByText('load blew up')).toBeInTheDocument())
  })
})
