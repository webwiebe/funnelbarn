import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Settings from './Settings'
import type { ApiKey, Project } from '../lib/api'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// Two projects, with the selected one deliberately NOT first: the create form
// used to hand the server `projects[0]`, which on a multi-project instance
// minted the key against a project the user never chose.
const mockProjects: Project[] = [
  { id: 'p1', name: 'My Site', slug: 'my-site', status: 'active' },
  { id: 'p2', name: 'Other Site', slug: 'other-site', status: 'active' },
]
const SELECTED_PROJECT_ID = 'p2'
const mockRefetch = vi.fn()
const mockSetSelectedProjectId = vi.fn()

vi.mock('../lib/projects', () => ({
  useProjects: () => ({
    projects: mockProjects,
    isLoading: false,
    refetch: mockRefetch,
    defaultProjectId: null,
    setDefaultProjectId: vi.fn(),
    selectedProjectId: SELECTED_PROJECT_ID,
    setSelectedProjectId: mockSetSelectedProjectId,
    selectedEnvironment: '',
    setSelectedEnvironment: vi.fn(),
  }),
  useEffectiveProjectId: () => SELECTED_PROJECT_ID,
}))

vi.mock('../lib/auth', () => ({
  useAuth: () => ({ user: { id: 'u1', username: 'admin' }, isLoading: false }),
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => vi.fn() }
})

const mockApiKeys: ApiKey[] = [
  { id: 'k1', project_id: SELECTED_PROJECT_ID, name: 'prod-key', scope: 'ingest', created_at: '2024-01-01T00:00:00Z' },
]

const mockApi = vi.hoisted(() => ({
  listApiKeys: vi.fn(),
  createApiKey: vi.fn(),
  deleteApiKey: vi.fn(),
  updateProject: vi.fn(),
  deleteProject: vi.fn(),
  approveProject: vi.fn(),
  getInstanceSettings: vi.fn(),
  setInstanceSettings: vi.fn(),
  getProjectHealth: vi.fn().mockResolvedValue({
    project_id: 'p1',
    setup_called: false,
    events_received: false,
    flags_evaluated: false,
    recordings_received: false,
    updated_at: '2024-01-01T00:00:00Z',
  }),
  resetProjectHealth: vi.fn().mockResolvedValue(undefined),
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

function renderSettings() {
  return render(
    <MemoryRouter>
      <Settings />
    </MemoryRouter>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Settings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.listApiKeys.mockResolvedValue({ api_keys: mockApiKeys })
    mockApi.getInstanceSettings.mockResolvedValue({ settings: { geo_enabled: 'true' } })
  })

  it('renders the Settings heading', async () => {
    renderSettings()
    expect(screen.getByRole('heading', { name: /settings/i })).toBeInTheDocument()
  })

  it('shows existing API keys after loading', async () => {
    renderSettings()
    // desktop table + mobile cards both render, so multiple matches are expected
    await waitFor(() => expect(screen.getAllByText('prod-key').length).toBeGreaterThan(0))
  })

  it('shows the Projects section', async () => {
    renderSettings()
    await waitFor(() => expect(screen.getByText('Projects')).toBeInTheDocument())
  })

  it('shows the tracking snippet section', async () => {
    renderSettings()
    await waitFor(() => expect(screen.getByText(/tracking snippet/i)).toBeInTheDocument())
  })

  it('shows the API Keys section', async () => {
    renderSettings()
    await waitFor(() => expect(screen.getByText('API Keys')).toBeInTheDocument())
  })

  it('shows error when creating key with empty name', async () => {
    renderSettings()
    await waitFor(() => screen.getByText('API Keys'))

    fireEvent.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() =>
      expect(screen.getByText(/name is required/i)).toBeInTheDocument(),
    )
    expect(mockApi.createApiKey).not.toHaveBeenCalled()
  })

  it('creates an API key and shows it revealed', async () => {
    mockApi.createApiKey.mockResolvedValue({
      api_key: { id: 'k2', project_id: SELECTED_PROJECT_ID, name: 'new-key', scope: 'ingest', created_at: '2024-06-01T00:00:00Z' },
      key: 'plaintext-value-here',
    })
    renderSettings()
    await waitFor(() => screen.getByText('API Keys'))

    const nameInput = screen.getByPlaceholderText(/key name/i)
    fireEvent.change(nameInput, { target: { value: 'new-key' } })
    fireEvent.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() =>
      expect(screen.getByText('plaintext-value-here')).toBeInTheDocument(),
    )
    expect(mockApi.createApiKey).toHaveBeenCalledWith('new-key', 'ingest', SELECTED_PROJECT_ID)
  })

  it('lists only the selected project’s keys', async () => {
    renderSettings()
    await waitFor(() =>
      expect(mockApi.listApiKeys).toHaveBeenCalledWith(SELECTED_PROJECT_ID),
    )
  })

  it('names the selected project in the API Keys section', async () => {
    renderSettings()
    // The name appears in several places on this page; only the API Keys
    // subtitle emphasises it, which is the one that has to say "Other Site".
    await waitFor(() =>
      expect(
        screen.getAllByText('Other Site').some((el) => el.tagName === 'STRONG'),
      ).toBe(true),
    )
  })

  it('mints against the project chosen in the form, not the first one', async () => {
    mockApi.createApiKey.mockResolvedValue({
      api_key: { id: 'k3', project_id: 'p1', name: 'other-key', scope: 'ingest', created_at: '2024-06-01T00:00:00Z' },
      key: 'plaintext-for-p1',
    })
    renderSettings()
    await waitFor(() => screen.getByText('API Keys'))

    fireEvent.change(screen.getByPlaceholderText(/key name/i), { target: { value: 'other-key' } })
    fireEvent.change(screen.getByLabelText(/project for the new api key/i), { target: { value: 'p1' } })
    fireEvent.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() =>
      expect(mockApi.createApiKey).toHaveBeenCalledWith('other-key', 'ingest', 'p1'),
    )
  })

  it('first delete click sets confirm state; second click deletes', async () => {
    mockApi.deleteApiKey.mockResolvedValue(undefined)
    renderSettings()
    await waitFor(() => expect(screen.getAllByText('prod-key').length).toBeGreaterThan(0))

    // First click on the icon button (desktop table): asks for confirmation
    const deleteBtns = screen.getAllByTitle(/delete key/i)
    fireEvent.click(deleteBtns[0])
    // Both desktop and mobile render confirm text — at least one should appear
    await waitFor(() => expect(screen.getAllByText(/confirm\?/i).length).toBeGreaterThan(0))

    // Second click on the red Confirm/Delete button (first occurrence = desktop)
    const confirmDeleteBtns = screen.getAllByRole('button', { name: /^delete$/i })
    fireEvent.click(confirmDeleteBtns[0])
    await waitFor(() => expect(mockApi.deleteApiKey).toHaveBeenCalledWith('k1'))
  })

  it('shows "No API keys yet" when list is empty', async () => {
    mockApi.listApiKeys.mockResolvedValue({ api_keys: [] })
    renderSettings()
    await waitFor(() =>
      expect(screen.getByText(/no api keys yet/i)).toBeInTheDocument(),
    )
  })
})
