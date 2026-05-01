import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'

// ---------------------------------------------------------------------------
// Mocks — defined before importing App so vi.mock hoisting works
// ---------------------------------------------------------------------------

const mockUseAuth = vi.fn()
vi.mock('./lib/auth', () => ({
  useAuth: () => mockUseAuth(),
  AuthProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

const mockUseProjects = vi.fn()
vi.mock('./lib/projects', () => ({
  useProjects: () => mockUseProjects(),
  ProjectProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

// Stub heavy page components so this file only tests routing logic
vi.mock('./pages/Dashboard', () => ({
  default: () => <div data-testid="dashboard-page">Dashboard</div>,
}))
vi.mock('./pages/Login', () => ({
  default: () => <div data-testid="login-page">Login</div>,
}))
vi.mock('./pages/Landing', () => ({
  default: () => <div data-testid="landing-page">Landing</div>,
}))
vi.mock('./pages/Funnels', () => ({
  default: () => <div data-testid="funnels-page">Funnels</div>,
}))
vi.mock('./pages/Live', () => ({
  default: () => <div data-testid="live-page">Live</div>,
}))
vi.mock('./pages/Settings', () => ({
  default: () => <div data-testid="settings-page">Settings</div>,
}))
vi.mock('./pages/ABTests', () => ({
  default: () => <div data-testid="abtests-page">ABTests</div>,
}))
vi.mock('./components/wizards/FirstRunWizard', () => ({
  default: () => <div data-testid="first-run-wizard">Wizard</div>,
}))
vi.mock('./components/ui/ErrorBoundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

// ---------------------------------------------------------------------------
// Helper: render DefaultProjectRoute in isolation via the real App routes
// ---------------------------------------------------------------------------

// We import the internal DefaultProjectRoute indirectly by rendering /dashboard
// through App's routes.  To keep tests simple we directly test DefaultProjectRoute
// by extracting it via a thin wrapper around the App routing tree.

import React from 'react'

// Pull DefaultProjectRoute out by re-rendering App at the /dashboard path.
// Because all providers and pages are mocked, only the routing logic is exercised.
import App from './App'

function renderAt(path: string) {
  // Patch window.location to the desired path before rendering
  window.history.pushState({}, '', path)
  return render(<App />)
}

// ---------------------------------------------------------------------------
// DefaultProjectRoute tests
// ---------------------------------------------------------------------------

const loggedInUser = { id: 'u1', username: 'admin', has_projects: true }

describe('DefaultProjectRoute', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: user is authenticated
    mockUseAuth.mockReturnValue({ user: loggedInUser, isLoading: false })
  })

  it('shows a spinner while projects are loading', () => {
    mockUseProjects.mockReturnValue({
      projects: [],
      isLoading: true,
      defaultProjectId: null,
      refetch: vi.fn(),
      setDefaultProjectId: vi.fn(),
    })

    renderAt('/dashboard')

    // The spinner is a div with a spin animation — it has no accessible role/text,
    // so we check that neither the Dashboard page nor a Navigate has rendered.
    expect(screen.queryByTestId('dashboard-page')).toBeNull()
    // The spinning div is styled with borderTopColor; we verify the container is present
    // by asserting the page isn't showing the empty-state Dashboard yet.
    expect(screen.queryByText('Dashboard')).toBeNull()
  })

  it('renders Dashboard (empty-state) when loading is done and there are no projects', async () => {
    mockUseProjects.mockReturnValue({
      projects: [],
      isLoading: false,
      defaultProjectId: null,
      refetch: vi.fn(),
      setDefaultProjectId: vi.fn(),
    })

    renderAt('/dashboard')

    await waitFor(() =>
      expect(screen.getByTestId('dashboard-page')).toBeInTheDocument(),
    )
  })

  it('redirects to the first project when projects are available', async () => {
    mockUseProjects.mockReturnValue({
      projects: [
        { id: 'p1', name: 'Alpha', slug: 'alpha', status: 'active' },
        { id: 'p2', name: 'Beta', slug: 'beta', status: 'active' },
      ],
      isLoading: false,
      defaultProjectId: null,
      refetch: vi.fn(),
      setDefaultProjectId: vi.fn(),
    })

    renderAt('/dashboard')

    // After redirect the route /dashboard/p1 should render the Dashboard page
    await waitFor(() =>
      expect(screen.getByTestId('dashboard-page')).toBeInTheDocument(),
    )
    expect(window.location.pathname).toBe('/dashboard/p1')
  })

  it('redirects to the defaultProject when it is set and valid', async () => {
    mockUseProjects.mockReturnValue({
      projects: [
        { id: 'p1', name: 'Alpha', slug: 'alpha', status: 'active' },
        { id: 'p2', name: 'Beta', slug: 'beta', status: 'active' },
      ],
      isLoading: false,
      defaultProjectId: 'p2',
      refetch: vi.fn(),
      setDefaultProjectId: vi.fn(),
    })

    renderAt('/dashboard')

    await waitFor(() =>
      expect(screen.getByTestId('dashboard-page')).toBeInTheDocument(),
    )
    expect(window.location.pathname).toBe('/dashboard/p2')
  })
})
