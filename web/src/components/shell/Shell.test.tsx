import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Shell from './Shell'
import type { ReactNode } from 'react'
import type { Project } from '../../lib/api'

// ---------------------------------------------------------------------------
// Mock the two contexts Shell depends on
// ---------------------------------------------------------------------------

const mockLogout = vi.fn()
const mockNavigate = vi.fn()

vi.mock('../../lib/auth', () => ({
  useAuth: () => ({
    user: { id: 'u1', username: 'testuser' },
    isLoading: false,
    login: vi.fn(),
    logout: mockLogout,
  }),
}))

// Default: two projects so we can also test the single-project case by
// overriding per test.
let mockProjects: Project[] = [
  { id: 'p1', name: 'Project Alpha', slug: 'alpha', status: 'active' },
  { id: 'p2', name: 'Project Beta', slug: 'beta', status: 'active' },
]

vi.mock('../../lib/projects', () => ({
  useProjects: () => ({ projects: mockProjects, isLoading: false, refetch: vi.fn() }),
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderShell(ui: ReactNode = <div>child content</div>, initialEntry = '/dashboard') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Shell>{ui}</Shell>
    </MemoryRouter>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Shell', () => {
  beforeEach(() => {
    mockProjects = [
      { id: 'p1', name: 'Project Alpha', slug: 'alpha', status: 'active' },
      { id: 'p2', name: 'Project Beta', slug: 'beta', status: 'active' },
    ]
    vi.clearAllMocks()
  })

  it('renders its children', () => {
    renderShell(<p>Hello world</p>)
    expect(screen.getByText('Hello world')).toBeInTheDocument()
  })

  it('shows the FunnelBarn brand name', () => {
    renderShell()
    // "Funnel" and "Barn" are split across sibling spans — query for the
    // parent link text or just the "Barn" amber span.
    expect(screen.getByText('Barn')).toBeInTheDocument()
    expect(screen.getByText('Funnel')).toBeInTheDocument()
  })

  it('shows the current project name when projects are present', () => {
    renderShell()
    // project-name appears in desktop project switcher
    const projectNameEls = screen.getAllByText('Project Alpha')
    expect(projectNameEls.length).toBeGreaterThan(0)
  })

  it('shows the logged-in username', () => {
    renderShell()
    expect(screen.getByText('testuser')).toBeInTheDocument()
  })

  it('shows all nav links', () => {
    renderShell()
    // Nav labels appear in both the desktop nav and the mobile bottom tab bar,
    // so use getAllByText and assert at least one instance is present.
    expect(screen.getAllByText('Funnels').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Overview').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Settings').length).toBeGreaterThan(0)
  })

  it('does not show the project dropdown when there is only one project', () => {
    // With a single project there is nothing to switch to, so the dropdown
    // list should not render any secondary project.
    mockProjects = [{ id: 'p1', name: 'Only Project', slug: 'only', status: 'active' }]
    renderShell()
    // "Only Project" appears in the switcher button but NOT as a second option
    const matches = screen.getAllByText('Only Project')
    // Each switcher (desktop + mobile) shows the name once — total 2 buttons
    expect(matches.length).toBeLessThanOrEqual(2)
  })
})
