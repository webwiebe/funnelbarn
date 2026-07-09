import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { MoreSheet } from './MoreSheet'
import type { Project } from '../../lib/api'

vi.mock('../../lib/auth', () => ({
  useAuth: () => ({ user: { id: 'u1', username: 'testuser' } }),
}))

const projects: Project[] = [
  { id: 'proj-123', name: 'My Project', slug: 'my-project', status: 'active' },
]

function renderSheet(overrides: Partial<{
  projectId?: string
  projects: Project[]
  iambarnProfileURL: string | null
  onClose: () => void
  onLogout: () => void
}> = {}) {
  const onClose = overrides.onClose ?? vi.fn()
  const onLogout = overrides.onLogout ?? vi.fn()
  render(
    <MemoryRouter initialEntries={['/dashboard/proj-123']}>
      <MoreSheet
        projectId={'projectId' in overrides ? overrides.projectId : 'proj-123'}
        projects={overrides.projects ?? projects}
        iambarnProfileURL={overrides.iambarnProfileURL ?? null}
        onClose={onClose}
        onLogout={onLogout}
      />
    </MemoryRouter>,
  )
  return { onClose, onLogout }
}

describe('MoreSheet', () => {
  it('shows the logged-in username', () => {
    renderSheet()
    expect(screen.getByText('testuser')).toBeInTheDocument()
  })

  it('renders the overflow nav links and Settings', () => {
    renderSheet()
    expect(screen.getByText('All Projects')).toBeInTheDocument()
    expect(screen.getByText('Cross-site Funnels')).toBeInTheDocument()
    expect(screen.getByText('Integrations')).toBeInTheDocument()
    expect(screen.getByText('Settings')).toBeInTheDocument()
  })

  it('scopes project-specific links to the current projectId', () => {
    renderSheet({ projectId: 'proj-123' })
    const insights = screen.getByText('Insights').closest('a')
    expect(insights?.getAttribute('href')).toBe('/insights/proj-123')
  })

  it('falls back to unscoped links when no projectId is given', () => {
    renderSheet({ projectId: undefined })
    const insights = screen.getByText('Insights').closest('a')
    expect(insights?.getAttribute('href')).toBe('/insights')
  })

  it('renders the project picker when projects exist', () => {
    renderSheet()
    expect(screen.getByText('Switch Project')).toBeInTheDocument()
    expect(screen.getByText('My Project')).toBeInTheDocument()
  })

  it('hides the project picker when there are no projects', () => {
    renderSheet({ projects: [] })
    expect(screen.queryByText('Switch Project')).not.toBeInTheDocument()
  })

  it('shows the IAMBarn profile link only when a profile URL is configured', () => {
    renderSheet({ iambarnProfileURL: 'https://iam.test.wiebe.xyz/admin#profile' })
    const link = screen.getByText('Edit IAMBarn profile').closest('a')
    expect(link?.getAttribute('href')).toBe('https://iam.test.wiebe.xyz/admin#profile')
    expect(link?.getAttribute('target')).toBe('_blank')
  })

  it('does not show the IAMBarn profile link when unconfigured', () => {
    renderSheet({ iambarnProfileURL: null })
    expect(screen.queryByText('Edit IAMBarn profile')).not.toBeInTheDocument()
  })

  it('calls onClose when a nav link is clicked', () => {
    const { onClose } = renderSheet()
    fireEvent.click(screen.getByText('All Projects'))
    expect(onClose).toHaveBeenCalled()
  })

  it('calls onClose when the overlay is clicked', () => {
    const { onClose } = renderSheet()
    fireEvent.click(document.querySelector('div[style*="z-index: 200"]')!)
    expect(onClose).toHaveBeenCalled()
  })

  it('closes and logs out when Log out is clicked', () => {
    const { onClose, onLogout } = renderSheet()
    fireEvent.click(screen.getByText('Log out'))
    expect(onClose).toHaveBeenCalled()
    expect(onLogout).toHaveBeenCalled()
  })
})
