import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Shell from './Shell'
import type { ReactNode } from 'react'

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

vi.mock('../../lib/projects', () => ({
  useProjects: () => ({
    projects: [{ id: 'proj-123', name: 'My Project', slug: 'my-project', status: 'active' }],
    isLoading: false,
    refetch: vi.fn(),
    defaultProjectId: 'proj-123',
    setDefaultProjectId: vi.fn(),
  }),
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
  beforeEach(() => { vi.clearAllMocks() })

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
})
