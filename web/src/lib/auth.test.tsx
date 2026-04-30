// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import { AuthProvider, useAuth } from './auth'
import { api } from './api'

vi.mock('./api', () => ({
  api: {
    me: vi.fn(),
    login: vi.fn(),
    logout: vi.fn(),
  },
  ApiError: class ApiError extends Error {
    status: number
    constructor(status: number, message: string) {
      super(message)
      this.name = 'ApiError'
      this.status = status
    }
  },
}))

afterEach(() => {
  vi.clearAllMocks()
})

function TestConsumer() {
  const { user, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="username">{user?.username ?? 'none'}</span>
      <button onClick={() => login('alice', 'secret')}>Login</button>
      <button onClick={() => logout()}>Logout</button>
    </div>
  )
}

describe('useAuth', () => {
  it('throws when used outside AuthProvider', () => {
    // Silence expected console.error from React
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<TestConsumer />)).toThrow('useAuth must be used inside AuthProvider')
    spy.mockRestore()
  })

  it('exposes user after successful me() fetch on mount', async () => {
    vi.mocked(api.me).mockResolvedValue({ id: 'u1', username: 'alice' })
    await act(async () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )
    })
    expect(screen.getByTestId('username').textContent).toBe('alice')
  })

  it('login() calls api.login with correct credentials and sets user', async () => {
    vi.mocked(api.me).mockRejectedValue({ status: 401, name: 'ApiError' })
    vi.mocked(api.login).mockResolvedValue({ id: 'u2', username: 'bob' })

    await act(async () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )
    })

    await act(async () => {
      fireEvent.click(screen.getByText('Login'))
    })

    expect(api.login).toHaveBeenCalledWith({ username: 'alice', password: 'secret' })
    expect(screen.getByTestId('username').textContent).toBe('bob')
  })

  it('logout() calls api.logout and clears user state', async () => {
    vi.mocked(api.me).mockResolvedValue({ id: 'u1', username: 'alice' })
    vi.mocked(api.logout).mockResolvedValue(undefined)

    await act(async () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )
    })

    expect(screen.getByTestId('username').textContent).toBe('alice')

    await act(async () => {
      fireEvent.click(screen.getByText('Logout'))
    })

    expect(api.logout).toHaveBeenCalled()
    expect(screen.getByTestId('username').textContent).toBe('none')
  })
})
