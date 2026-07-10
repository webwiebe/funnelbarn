import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { MoreSheet } from './MoreSheet'
import type { Project } from '../../lib/api'
import type { IambarnConfig } from '../../lib/iambarn-widget'

vi.mock('../../lib/auth', () => ({
  useAuth: () => ({ user: { id: 'u1', username: 'testuser' } }),
}))

const projects: Project[] = [
  { id: 'proj-123', name: 'My Project', slug: 'my-project', status: 'active' },
]

const iambarnCfg: IambarnConfig = {
  server_url: 'https://iam.test.wiebe.xyz',
  client_id: 'ibc_test',
  widget_url: 'https://iam.test.wiebe.xyz/widget/iambarn-widget.iife.js',
  post_logout_redirect_uri: 'https://app.test/api/v1/auth/oidc/logged-out',
}

function renderSheet(overrides: Partial<{
  projectId?: string
  projects: Project[]
  iambarn: IambarnConfig | null
  iambarnReady: boolean
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
        iambarn={overrides.iambarn ?? null}
        iambarnReady={overrides.iambarnReady ?? false}
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

  it('shows a Manage account link to /account for IAMBarn sessions', () => {
    renderSheet({ iambarn: iambarnCfg })
    const link = screen.getByText('Manage account').closest('a')
    expect(link?.getAttribute('href')).toBe('/account')
  })

  it('does not show the Manage account link for local sessions', () => {
    renderSheet({ iambarn: null })
    expect(screen.queryByText('Manage account')).not.toBeInTheDocument()
  })

  it('renders the hosted logout button once the widget is ready', () => {
    const { container } = { container: document.body }
    renderSheet({ iambarn: iambarnCfg, iambarnReady: true })
    // The plain Log out button is replaced by the hosted component.
    expect(screen.queryByText('Log out')).not.toBeInTheDocument()
    const el = container.querySelector('iambarn-logout-button')
    expect(el).not.toBeNull()
    expect(el?.getAttribute('server-url')).toBe(iambarnCfg.server_url)
    expect(el?.getAttribute('client-id')).toBe(iambarnCfg.client_id)
    expect(el?.getAttribute('post-logout-redirect-uri')).toBe(iambarnCfg.post_logout_redirect_uri)
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

  it('closes and logs out via the plain button for local sessions', () => {
    const { onClose, onLogout } = renderSheet()
    fireEvent.click(screen.getByText('Log out'))
    expect(onClose).toHaveBeenCalled()
    expect(onLogout).toHaveBeenCalled()
  })
})
