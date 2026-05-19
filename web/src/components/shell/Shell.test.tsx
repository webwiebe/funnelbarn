import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Shell from './Shell'
import type { ReactNode } from 'react'

const mockLogout = vi.fn()
const mockNavigate = vi.fn()
const mockGetClientConfig = vi.fn()

vi.mock('../../lib/auth', () => ({
  useAuth: () => ({
    user: { id: 'u1', username: 'testuser' },
    isLoading: false,
    login: vi.fn(),
    logout: mockLogout,
  }),
}))

vi.mock('../../lib/projects', () => ({
  useProjects: () => ({
    projects: [{ id: 'proj-123', name: 'My Project', slug: 'my-project', status: 'active' }],
    isLoading: false,
    refetch: vi.fn(),
    defaultProjectId: 'proj-123',
    setDefaultProjectId: vi.fn(),
  }),
}))

vi.mock('../../lib/api', () => ({
  api: {
    getClientConfig: (...args: unknown[]) => mockGetClientConfig(...args),
  },
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => mockNavigate }
})

function renderShell(ui: ReactNode = <div>child content</div>, projectId?: string, projectName?: string) {
  return render(
    <MemoryRouter initialEntries={['/dashboard']}>
      <Shell projectId={projectId} projectName={projectName}>{ui}</Shell>
    </MemoryRouter>,
  )
}

describe('Shell', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Reset auth-method cookie between tests so we can opt in per case.
    document.cookie = 'funnelbarn_auth_method=; path=/; max-age=0'
    mockGetClientConfig.mockResolvedValue({
      bugbarn_endpoint: '',
      bugbarn_ingest_key: '',
      iambarn_enabled: false,
    })
  })

  it('renders its children', () => {
    renderShell(<p>Hello world</p>)
    expect(screen.getByText('Hello world')).toBeInTheDocument()
  })

  it('shows the FunnelBarn brand name', () => {
    renderShell()
    expect(screen.getByText('Barn')).toBeInTheDocument()
    expect(screen.getByText('Funnel')).toBeInTheDocument()
  })

  it('uses projectId in nav link hrefs when provided', () => {
    renderShell(undefined, 'proj-123')
    const hrefs = screen.getAllByRole('link').map((l) => l.getAttribute('href') ?? '')
    expect(hrefs.some((h) => h.includes('proj-123'))).toBe(true)
  })

  it('shows project name badge when projectName provided', () => {
    renderShell(undefined, 'proj-123', 'My Project')
    expect(screen.getByText('My Project')).toBeInTheDocument()
  })

  it('shows the logged-in username', () => {
    renderShell()
    expect(screen.getByText('testuser')).toBeInTheDocument()
  })

  it('shows all nav links', () => {
    renderShell()
    expect(screen.getAllByText('Funnels').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Overview').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Settings').length).toBeGreaterThan(0)
  })

  it('renders without errors when no projectId is provided', () => {
    expect(() => renderShell()).not.toThrow()
  })

  it('does not show "Edit IAMBarn profile" link when iambarn profile_url is not configured', async () => {
    document.cookie = 'funnelbarn_auth_method=oidc; path=/'
    renderShell()
    // Open the desktop user menu so the dropdown is visible.
    fireEvent.click(screen.getByText('testuser'))
    // Give the effect a tick to resolve.
    await waitFor(() => expect(mockGetClientConfig).toHaveBeenCalled())
    expect(screen.queryByText('Edit IAMBarn profile')).not.toBeInTheDocument()
  })

  it('shows "Edit IAMBarn profile" link in the user menu when iambarn profile_url is configured and session is OIDC', async () => {
    document.cookie = 'funnelbarn_auth_method=oidc; path=/'
    mockGetClientConfig.mockResolvedValue({
      bugbarn_endpoint: '',
      bugbarn_ingest_key: '',
      iambarn_enabled: false,
      iambarn: { profile_url: 'https://iam.test.wiebe.xyz/admin#profile' },
    })
    renderShell()
    fireEvent.click(screen.getByText('testuser'))
    const link = await screen.findByText('Edit IAMBarn profile')
    const anchor = link.closest('a')
    expect(anchor).not.toBeNull()
    expect(anchor!.getAttribute('href')).toBe('https://iam.test.wiebe.xyz/admin#profile')
    expect(anchor!.getAttribute('target')).toBe('_blank')
    expect(anchor!.getAttribute('rel')).toBe('noopener noreferrer')
  })

  it('does not show "Edit IAMBarn profile" link for local password sessions even when iambarn profile_url is configured', async () => {
    // No funnelbarn_auth_method cookie — local password session.
    mockGetClientConfig.mockResolvedValue({
      bugbarn_endpoint: '',
      bugbarn_ingest_key: '',
      iambarn_enabled: false,
      iambarn: { profile_url: 'https://iam.test.wiebe.xyz/admin#profile' },
    })
    renderShell()
    fireEvent.click(screen.getByText('testuser'))
    // Give the component effect time; getClientConfig should NOT be called
    // because the OIDC cookie is absent.
    await new Promise((r) => setTimeout(r, 0))
    expect(mockGetClientConfig).not.toHaveBeenCalled()
    expect(screen.queryByText('Edit IAMBarn profile')).not.toBeInTheDocument()
  })
})
