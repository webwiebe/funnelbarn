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
    selectedEnvironment: '',
    setSelectedEnvironment: vi.fn(),
  }),
}))

vi.mock('../../lib/api', () => ({
  api: {
    getClientConfig: (...args: unknown[]) => mockGetClientConfig(...args),
    getEnvironments: vi.fn().mockResolvedValue({ environments: [] }),
  },
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => mockNavigate }
})

// Control widget readiness deterministically; the real bundle never loads in
// jsdom. Default: not ready (falls back to the built-in dropdown).
let mockIambarnReady = false
vi.mock('../../lib/iambarn-widget', () => ({
  useIambarnWidget: () => mockIambarnReady,
  iambarnThemeVars: {},
}))

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
    mockIambarnReady = false
    // Reset auth-method cookie between tests so we can opt in per case.
    document.cookie = 'funnelbarn_auth_method=; path=/; max-age=0'
    mockGetClientConfig.mockResolvedValue({
      bugbarn_endpoint: '',
      bugbarn_ingest_key: '',
    })
  })

  const oidcConfig = {
    bugbarn_endpoint: '',
    bugbarn_ingest_key: '',
    iambarn: {
      server_url: 'https://iam.test.wiebe.xyz',
      client_id: 'ibc_test',
      widget_url: 'https://iam.test.wiebe.xyz/widget/iambarn-widget.iife.js',
      post_logout_redirect_uri: 'https://app.test/api/v1/auth/oidc/logged-out',
    },
  }

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

  it('does not show a Manage account link when iambarn is not configured', async () => {
    document.cookie = 'funnelbarn_auth_method=oidc; path=/'
    renderShell()
    // Open the desktop user menu so the dropdown is visible.
    fireEvent.click(screen.getByText('testuser'))
    // Give the effect a tick to resolve.
    await waitFor(() => expect(mockGetClientConfig).toHaveBeenCalled())
    expect(screen.queryByText('Manage account')).not.toBeInTheDocument()
  })

  it('shows a Manage account link to /account in the fallback menu for OIDC sessions before the widget loads', async () => {
    document.cookie = 'funnelbarn_auth_method=oidc; path=/'
    mockGetClientConfig.mockResolvedValue(oidcConfig)
    renderShell()
    fireEvent.click(screen.getByText('testuser'))
    const link = await screen.findByText('Manage account')
    const anchor = link.closest('a')
    expect(anchor).not.toBeNull()
    expect(anchor!.getAttribute('href')).toBe('/account')
  })

  it('renders the hosted iambarn-user-menu once the widget is ready', async () => {
    document.cookie = 'funnelbarn_auth_method=oidc; path=/'
    mockIambarnReady = true
    mockGetClientConfig.mockResolvedValue(oidcConfig)
    const { container } = renderShell()
    await waitFor(() => {
      expect(container.querySelector('iambarn-user-menu')).not.toBeNull()
    })
    const el = container.querySelector('iambarn-user-menu')!
    expect(el.getAttribute('server-url')).toBe(oidcConfig.iambarn.server_url)
    expect(el.getAttribute('client-id')).toBe(oidcConfig.iambarn.client_id)
    expect(el.getAttribute('account-href')).toBe('/account')
    // The built-in username button is not rendered in this mode.
    expect(screen.queryByText('testuser')).not.toBeInTheDocument()
  })

  it('does not fetch iambarn config or show Manage account for local password sessions', async () => {
    // No funnelbarn_auth_method cookie — local password session.
    mockGetClientConfig.mockResolvedValue(oidcConfig)
    renderShell()
    fireEvent.click(screen.getByText('testuser'))
    // getClientConfig should NOT be called because the OIDC cookie is absent.
    await new Promise((r) => setTimeout(r, 0))
    expect(mockGetClientConfig).not.toHaveBeenCalled()
    expect(screen.queryByText('Manage account')).not.toBeInTheDocument()
  })
})
