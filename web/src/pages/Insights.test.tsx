import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Insights from './Insights'
import type { WidgetBreakdownResult, DistributionEntry } from '../lib/api'

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
  getBatchBreakdowns: vi.fn(),
  getSessionDistributions: vi.fn(),
  deleteWidget: vi.fn(),
  updateWidget: vi.fn(),
  getEnvironments: vi.fn().mockResolvedValue({ environments: [] }),
  getEventNames: vi.fn().mockResolvedValue({ event_names: ['page_view'] }),
  getEventProperties: vi.fn().mockResolvedValue({ properties: ['url'] }),
  createWidget: vi.fn(),
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

const mockResults: WidgetBreakdownResult[] = [
  {
    widget: {
      id: 'w1',
      project_id: 'p1',
      event_name: 'page_view',
      property: 'url',
      title: 'Top Pages',
      position: 0,
      size: 1,
      created_at: '2024-01-01T00:00:00Z',
    },
    breakdown: [
      { value: '/home', count: 30 },
      { value: '/about', count: 12 },
    ],
  },
]

const mockDistributions: Record<string, DistributionEntry[]> = {
  device_type: [
    { value: 'desktop', count: 40, pct: 80 },
    { value: 'mobile', count: 10, pct: 20 },
  ],
}

function renderPage(projectId = 'p1') {
  return render(
    <MemoryRouter initialEntries={[`/insights/${projectId}`]}>
      <Routes>
        <Route path="/insights/:projectId" element={<Insights />} />
      </Routes>
    </MemoryRouter>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Insights', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.getBatchBreakdowns.mockResolvedValue({ results: mockResults })
    mockApi.getSessionDistributions.mockResolvedValue({ distributions: mockDistributions })
    mockApi.deleteWidget.mockResolvedValue(undefined)
  })

  it('renders the Insights heading', async () => {
    renderPage()
    expect(screen.getByRole('heading', { name: /insights/i })).toBeInTheDocument()
  })

  it('renders widgets after data loads', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('Top Pages')).toBeInTheDocument())
    expect(screen.getByText('/home')).toBeInTheDocument()
    expect(mockApi.getBatchBreakdowns).toHaveBeenCalledWith('p1')
  })

  it('renders the visitor breakdown from session distributions', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText(/visitor breakdown/i)).toBeInTheDocument())
    expect(screen.getByText('desktop')).toBeInTheDocument()
    expect(mockApi.getSessionDistributions).toHaveBeenCalledWith('p1')
  })

  it('shows the empty state when there are no widgets', async () => {
    mockApi.getBatchBreakdowns.mockResolvedValue({ results: [] })
    renderPage()
    await waitFor(() => expect(screen.getByText(/no widgets yet/i)).toBeInTheDocument())
  })

  it('opens the add-widget modal when clicking Add widget', async () => {
    // No widgets -> the empty-state "Add your first widget" button also exists,
    // but the header button is always present.
    renderPage()
    await waitFor(() => expect(screen.getByText('Top Pages')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /add widget/i }))
    // Modal renders an "Add Widget" heading.
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /add widget/i })).toBeInTheDocument(),
    )
  })

  it('deletes a widget when clicking its delete button', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('Top Pages')).toBeInTheDocument())
    fireEvent.click(screen.getByTitle(/delete widget/i))
    await waitFor(() => expect(mockApi.deleteWidget).toHaveBeenCalledWith('p1', 'w1'))
    await waitFor(() => expect(screen.queryByText('Top Pages')).toBeNull())
  })
})
