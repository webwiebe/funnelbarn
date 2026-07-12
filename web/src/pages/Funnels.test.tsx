import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Funnels from './Funnels'
import type { Funnel, FunnelAnalysis } from '../lib/api'

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

const mockNavigate = vi.fn()
vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => mockNavigate }
})

// Keep analytics tracking a no-op.
vi.mock('../lib/analytics', () => ({
  trackEvent: vi.fn(),
  trackPageView: vi.fn(),
}))

const mockApi = vi.hoisted(() => ({
  listApiKeys: vi.fn(),
  listFunnels: vi.fn(),
  listSegments: vi.fn(),
  getFunnelAnalysis: vi.fn(),
  deleteFunnel: vi.fn(),
  createFunnel: vi.fn(),
  getSessionDistributions: vi.fn(),
  getEventNames: vi.fn().mockResolvedValue({ event_names: [] }),
  getEventProperties: vi.fn().mockResolvedValue({ properties: [] }),
  getEventPropertyValues: vi.fn().mockResolvedValue({ values: [] }),
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

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const funnelA: Funnel = {
  id: 'f1',
  name: 'Signup flow',
  scope: 'session',
  steps: [
    { step_order: 0, event_name: 'page_view' },
    { step_order: 1, event_name: 'signup_completed' },
  ],
}

const funnelB: Funnel = {
  id: 'f2',
  name: 'Checkout',
  scope: 'page_view',
  steps: [{ step_order: 0, event_name: 'page_view' }],
}

const analysisWithData: FunnelAnalysis = {
  funnel: funnelA,
  from: '2024-01-01T00:00:00Z',
  to: '2024-01-31T00:00:00Z',
  results: [
    { step_order: 0, event_name: 'page_view', count: 100, conversion: 1, drop_off: 0 },
    { step_order: 1, event_name: 'signup_completed', count: 40, conversion: 0.4, drop_off: 0.6 },
  ],
}

const analysisEmpty: FunnelAnalysis = {
  funnel: funnelA,
  from: '2024-01-01T00:00:00Z',
  to: '2024-01-31T00:00:00Z',
  results: [
    { step_order: 0, event_name: 'page_view', count: 0, conversion: 0, drop_off: 0 },
  ],
}

function renderFunnels(projectId: string | null = 'p1') {
  const path = projectId ? `/funnels/${projectId}` : '/funnels'
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/funnels/:projectId" element={<Funnels />} />
        <Route path="/funnels" element={<Funnels />} />
      </Routes>
    </MemoryRouter>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Funnels', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.listApiKeys.mockResolvedValue({ api_keys: [{ id: 'k1', name: 'ingest', scope: 'ingest', created_at: '2024-01-01T00:00:00Z' }] })
    mockApi.listFunnels.mockResolvedValue({ funnels: [funnelA, funnelB] })
    mockApi.listSegments.mockResolvedValue({ segments: [] })
    mockApi.getSessionDistributions.mockResolvedValue({ distributions: {} })
    mockApi.getFunnelAnalysis.mockResolvedValue(analysisWithData)
    mockApi.deleteFunnel.mockResolvedValue(undefined)
  })

  it('renders the Funnels heading', async () => {
    renderFunnels()
    expect(screen.getByRole('heading', { name: /^funnels$/i })).toBeInTheDocument()
  })

  it('fetches funnels for the project on mount', async () => {
    renderFunnels('p1')
    await waitFor(() => expect(mockApi.listFunnels).toHaveBeenCalledWith('p1'))
    expect(mockApi.listSegments).toHaveBeenCalledWith('p1')
    expect(mockApi.listApiKeys).toHaveBeenCalled()
  })

  it('renders the list of funnels once loaded', async () => {
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())
    expect(screen.getByText('Checkout')).toBeInTheDocument()
    // funnelA has 2 steps.
    expect(screen.getByText(/2 steps/i)).toBeInTheDocument()
  })

  it('shows the empty state when there are no funnels', async () => {
    mockApi.listFunnels.mockResolvedValue({ funnels: [] })
    renderFunnels()
    await waitFor(() => expect(screen.getByText(/no funnels yet/i)).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /create one/i })).toBeInTheDocument()
  })

  it('shows the "select a funnel" placeholder before any funnel is chosen', async () => {
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())
    expect(screen.getByText(/select a funnel to view analysis/i)).toBeInTheDocument()
  })

  it('loads and renders analysis when a funnel is selected', async () => {
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())

    fireEvent.click(screen.getByText('Signup flow'))

    await waitFor(() =>
      expect(mockApi.getFunnelAnalysis).toHaveBeenCalledWith('p1', 'f1', 'all', undefined),
    )
    // The step event names from the analysis results render in the detail panel.
    await waitFor(() => expect(screen.getByText('signup_completed')).toBeInTheDocument())
    // Drop-off between step 1 and 2 is rendered.
    expect(screen.getByText(/60\.0% dropped off/i)).toBeInTheDocument()
  })

  it('shows the "no data yet" state for a funnel with an empty first step', async () => {
    mockApi.getFunnelAnalysis.mockResolvedValue(analysisEmpty)
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())

    fireEvent.click(screen.getByText('Signup flow'))
    await waitFor(() => expect(screen.getByText(/no data yet for/i)).toBeInTheDocument())
  })

  it('renders preset segment pills after selecting a funnel and refetches on segment change', async () => {
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Signup flow'))

    await waitFor(() => expect(mockApi.getFunnelAnalysis).toHaveBeenCalledTimes(1))

    // "Mobile" is a preset segment pill.
    const mobilePill = await screen.findByRole('button', { name: /^mobile$/i })
    fireEvent.click(mobilePill)

    await waitFor(() =>
      expect(mockApi.getFunnelAnalysis).toHaveBeenCalledWith('p1', 'f1', 'mobile', undefined),
    )
  })

  it('opens the create-funnel modal from the header button', async () => {
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /create funnel/i }))
    // The modal has a unique name input placeholder.
    await waitFor(() =>
      expect(screen.getByPlaceholderText(/e\.g\. signup flow/i)).toBeInTheDocument(),
    )
  })

  it('opens the create modal from the empty-state "Create one" button', async () => {
    mockApi.listFunnels.mockResolvedValue({ funnels: [] })
    renderFunnels()
    await waitFor(() => expect(screen.getByText(/no funnels yet/i)).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /create one/i }))
    await waitFor(() =>
      expect(screen.getByPlaceholderText(/e\.g\. signup flow/i)).toBeInTheDocument(),
    )
  })

  it('shows funnel templates in the Templates dropdown', async () => {
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /templates/i }))
    expect(await screen.findByText('E-commerce')).toBeInTheDocument()
    expect(screen.getByText('SaaS Signup')).toBeInTheDocument()
  })

  it('opens the create modal pre-seeded when a template is selected', async () => {
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /templates/i }))
    fireEvent.click(await screen.findByText('Lead Capture'))

    await waitFor(() =>
      expect(screen.getByPlaceholderText(/e\.g\. signup flow/i)).toBeInTheDocument(),
    )
  })

  it('deletes the selected funnel after confirmation', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Signup flow'))

    const deleteBtn = await screen.findByRole('button', { name: /^delete$/i })
    fireEvent.click(deleteBtn)

    await waitFor(() => expect(mockApi.deleteFunnel).toHaveBeenCalledWith('p1', 'f1'))
    confirmSpy.mockRestore()
  })

  it('does not delete when confirmation is dismissed', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Signup flow'))

    const deleteBtn = await screen.findByRole('button', { name: /^delete$/i })
    fireEvent.click(deleteBtn)

    expect(mockApi.deleteFunnel).not.toHaveBeenCalled()
    confirmSpy.mockRestore()
  })

  it('opens the edit modal for the selected funnel', async () => {
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Signup flow'))

    const editBtn = await screen.findByRole('button', { name: /edit/i })
    fireEvent.click(editBtn)
    // Edit modal surfaces the existing steps' event names in inputs.
    await waitFor(() => {
      const dialogs = screen.getAllByText(/save|update/i)
      expect(dialogs.length).toBeGreaterThan(0)
    })
  })

  it('shows an error banner when analysis loading fails', async () => {
    mockApi.getFunnelAnalysis.mockRejectedValue(new Error('backend down'))
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Signup flow'))

    await waitFor(() =>
      expect(screen.getByText(/failed to load analysis/i)).toBeInTheDocument(),
    )
  })

  it('renders the select-a-project prompt without a project id', async () => {
    renderFunnels(null)
    expect(screen.getByText(/select a project to view funnels/i)).toBeInTheDocument()
    expect(mockApi.listFunnels).not.toHaveBeenCalled()
  })

  it('renders stored segments as pills and filters by a stored segment', async () => {
    mockApi.listSegments.mockResolvedValue({
      segments: [{ id: 's1', project_id: 'p1', name: 'Power users', rules: [], created_at: '2024-01-01T00:00:00Z' }],
    })
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Signup flow'))

    // Wait for detail panel; the stored segment pill should appear.
    const detailPill = await screen.findByRole('button', { name: /power users/i })
    fireEvent.click(detailPill)

    await waitFor(() =>
      expect(mockApi.getFunnelAnalysis).toHaveBeenCalledWith('p1', 'f1', undefined, 's1'),
    )
  })

  it('marks the selected funnel as active in the list', async () => {
    renderFunnels()
    await waitFor(() => expect(screen.getByText('Signup flow')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Checkout'))
    await waitFor(() =>
      expect(mockApi.getFunnelAnalysis).toHaveBeenCalledWith('p1', 'f2', 'all', undefined),
    )
  })
})
