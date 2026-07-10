import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import Account from './Account'

const mockGetClientConfig = vi.fn()
let mockReady = false

vi.mock('../components/shell/Shell', () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('../lib/api', () => ({
  api: { getClientConfig: (...a: unknown[]) => mockGetClientConfig(...a) },
}))

vi.mock('../lib/iambarn-widget', () => ({
  useIambarnWidget: () => mockReady,
  iambarnThemeVars: {},
}))

const iambarn = {
  server_url: 'https://iam.test.wiebe.xyz',
  widget_url: 'https://iam.test.wiebe.xyz/widget/iambarn-widget.iife.js',
}

describe('Account page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockReady = false
    document.cookie = 'funnelbarn_auth_method=; path=/; max-age=0'
    mockGetClientConfig.mockResolvedValue({ iambarn })
  })

  it('prompts to sign in with IAMBarn for non-OIDC sessions', async () => {
    render(<Account />)
    expect(await screen.findByText(/Sign in with IAMBarn/i)).toBeInTheDocument()
    expect(document.querySelector('iambarn-profile')).toBeNull()
  })

  it('shows a loading state for OIDC sessions until the widget is ready', async () => {
    document.cookie = 'funnelbarn_auth_method=oidc; path=/'
    render(<Account />)
    expect(await screen.findByText('Loading account editor…')).toBeInTheDocument()
    expect(document.querySelector('iambarn-profile')).toBeNull()
  })

  it('renders the hosted iambarn-profile once ready', async () => {
    document.cookie = 'funnelbarn_auth_method=oidc; path=/'
    mockReady = true
    const { container } = render(<Account />)
    await waitFor(() => {
      const el = container.querySelector('iambarn-profile')
      expect(el).not.toBeNull()
      expect(el?.getAttribute('server-url')).toBe(iambarn.server_url)
    })
  })
})
