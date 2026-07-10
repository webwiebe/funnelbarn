import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { TopNav } from './TopNav'
import type { IambarnConfig } from '../../lib/iambarn-widget'

vi.mock('../../lib/auth', () => ({
  useAuth: () => ({ user: { id: 'u1', username: 'testuser' } }),
}))

const iambarnCfg: IambarnConfig = {
  server_url: 'https://iam.test.wiebe.xyz',
  client_id: 'ibc_test',
  widget_url: 'https://iam.test.wiebe.xyz/widget/iambarn-widget.iife.js',
  post_logout_redirect_uri: 'https://app.test/api/v1/auth/oidc/logged-out',
}

function renderNav(overrides: Partial<{
  iambarn: IambarnConfig | null
  iambarnReady: boolean
  userMenuOpen: boolean
  onUserMenuToggle: () => void
  onLogout: () => void
}> = {}) {
  const onUserMenuToggle = overrides.onUserMenuToggle ?? vi.fn()
  const onLogout = overrides.onLogout ?? vi.fn()
  const { container } = render(
    <MemoryRouter>
      <TopNav
        projects={[]}
        navLinks={[]}
        userMenuOpen={overrides.userMenuOpen ?? false}
        onUserMenuToggle={onUserMenuToggle}
        iambarn={overrides.iambarn ?? null}
        iambarnReady={overrides.iambarnReady ?? false}
        onLogout={onLogout}
        isActive={() => false}
      />
    </MemoryRouter>,
  )
  return { container, onUserMenuToggle, onLogout }
}

describe('TopNav user menu', () => {
  it('shows the built-in username dropdown for local sessions', () => {
    renderNav({ iambarn: null, userMenuOpen: true })
    expect(screen.getByText('testuser')).toBeInTheDocument()
    expect(screen.getByText('Logout')).toBeInTheDocument()
    expect(screen.queryByText('Manage account')).not.toBeInTheDocument()
  })

  it('toggles the dropdown when the username button is clicked', () => {
    const { onUserMenuToggle } = renderNav({ iambarn: null })
    fireEvent.click(screen.getByText('testuser'))
    expect(onUserMenuToggle).toHaveBeenCalled()
  })

  it('shows a Manage account link to /account for OIDC sessions before the widget is ready', () => {
    renderNav({ iambarn: iambarnCfg, iambarnReady: false, userMenuOpen: true })
    const link = screen.getByText('Manage account').closest('a')
    expect(link?.getAttribute('href')).toBe('/account')
    // Still the built-in dropdown, not the hosted element.
    expect(screen.getByText('testuser')).toBeInTheDocument()
  })

  it('renders the hosted iambarn-user-menu when the widget is ready', () => {
    const { container } = renderNav({ iambarn: iambarnCfg, iambarnReady: true })
    const el = container.querySelector('iambarn-user-menu')
    expect(el).not.toBeNull()
    expect(el?.getAttribute('server-url')).toBe(iambarnCfg.server_url)
    expect(el?.getAttribute('client-id')).toBe(iambarnCfg.client_id)
    expect(el?.getAttribute('account-href')).toBe('/account')
    expect(el?.getAttribute('post-logout-redirect-uri')).toBe(iambarnCfg.post_logout_redirect_uri)
    // The built-in username button is not rendered in hosted mode.
    expect(screen.queryByText('testuser')).not.toBeInTheDocument()
  })
})
