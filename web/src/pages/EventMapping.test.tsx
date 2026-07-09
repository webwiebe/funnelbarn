import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import EventMapping from './EventMapping'
import type { CanonicalEvent, EventNameMapping, MappingSuggestion } from '../lib/api'

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
  listCanonicalEvents: vi.fn(),
  listMappings: vi.fn(),
  getMappingSuggestions: vi.fn(),
  setMappings: vi.fn(),
  createCanonicalEvent: vi.fn(),
  deleteCanonicalEvent: vi.fn(),
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

const mockCatalog: CanonicalEvent[] = [
  { key: 'contact_form', label: 'Contact Form', sort_order: 10 },
  { key: 'signup', label: 'Sign Up', sort_order: 20 },
]
const mockMappings: EventNameMapping[] = [
  { project_id: 'p1', raw_name: 'contact_submit', canonical_key: 'contact_form' },
]
const mockSuggestions: MappingSuggestion[] = [
  { raw_name: 'register_click', suggested_key: 'signup' },
]

function renderPage() {
  return render(
    <MemoryRouter>
      <EventMapping />
    </MemoryRouter>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('EventMapping', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.listCanonicalEvents.mockResolvedValue({ canonical_events: mockCatalog })
    mockApi.listMappings.mockResolvedValue({ mappings: mockMappings })
    mockApi.getMappingSuggestions.mockResolvedValue({ suggestions: mockSuggestions })
    mockApi.setMappings.mockResolvedValue({ mappings: mockMappings })
    mockApi.createCanonicalEvent.mockResolvedValue(mockCatalog[0])
    mockApi.deleteCanonicalEvent.mockResolvedValue(undefined)
  })

  it('renders the Event Normalization heading', async () => {
    renderPage()
    expect(
      screen.getByRole('heading', { name: /event normalization/i }),
    ).toBeInTheDocument()
  })

  it('lists the canonical events from the catalog', async () => {
    renderPage()
    // Labels appear in the catalog list AND as <option>s in the per-row selects.
    await waitFor(() => expect(screen.getAllByText('Contact Form').length).toBeGreaterThan(0))
    expect(screen.getAllByText('Sign Up').length).toBeGreaterThan(0)
  })

  it('loads mappings and suggestions for the default project', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('contact_submit')).toBeInTheDocument())
    expect(screen.getByText('register_click')).toBeInTheDocument()
    expect(mockApi.listMappings).toHaveBeenCalledWith('p1')
    expect(mockApi.getMappingSuggestions).toHaveBeenCalledWith('p1')
  })

  it('shows the mapped count summary', async () => {
    renderPage()
    // 1 existing mapping + 1 suggestion (both have a canonical key) = 2 of 2 mapped
    await waitFor(() => expect(screen.getByText(/2 of 2 mapped/i)).toBeInTheDocument())
  })

  it('saves mappings when clicking Save', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('contact_submit')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /save mappings/i }))
    await waitFor(() => expect(mockApi.setMappings).toHaveBeenCalled())
    expect(mockApi.setMappings.mock.calls[0][0]).toBe('p1')
    await waitFor(() => expect(screen.getByText(/saved 2 mappings/i)).toBeInTheDocument())
  })

  it('adds a new canonical event', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('Contact Form')).toBeInTheDocument())

    fireEvent.change(screen.getByPlaceholderText(/key \(e\.g\. contact_form\)/i), {
      target: { value: 'purchase' },
    })
    fireEvent.change(screen.getByPlaceholderText(/label/i), {
      target: { value: 'Purchase' },
    })
    fireEvent.click(screen.getByRole('button', { name: /add canonical event/i }))

    await waitFor(() =>
      expect(mockApi.createCanonicalEvent).toHaveBeenCalledWith(
        expect.objectContaining({ key: 'purchase', label: 'Purchase' }),
      ),
    )
  })

  it('shows an error banner when the catalog fails to load', async () => {
    mockApi.listCanonicalEvents.mockRejectedValueOnce(new Error('boom'))
    renderPage()
    await waitFor(() => expect(screen.getByText('boom')).toBeInTheDocument())
  })
})
