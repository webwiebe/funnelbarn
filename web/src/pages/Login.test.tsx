import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Login from './Login'

const mockLogin = vi.fn()
const mockNavigate = vi.fn()
const mockGetClientConfig = vi.fn()

vi.mock('../lib/auth', () => ({
  useAuth: () => ({
    user: null,
    isLoading: false,
    login: mockLogin,
    logout: vi.fn(),
  }),
}))

const mockRefetch = vi.fn()
vi.mock('../lib/projects', () => ({
  useProjects: () => ({
    projects: [],
    isLoading: false,
    refetch: mockRefetch,
    defaultProjectId: null,
    setDefaultProjectId: vi.fn(),
  }),
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => mockNavigate }
})

// Mock the api module so getClientConfig resolves immediately and doesn't
// trigger SSO redirects. Individual tests can override via mockGetClientConfig.
vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getClientConfig: (...args: unknown[]) => mockGetClientConfig(...args),
    },
  }
})

function renderLogin() {
  return render(
    <MemoryRouter>
      <Login />
    </MemoryRouter>,
  )
}

describe('Login', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: no SSO configured — show the local form.
    mockGetClientConfig.mockResolvedValue({})
  })

  it('renders the sign-in form', async () => {
    renderLogin()
    expect(await screen.findByLabelText(/username/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument()
  })

  it('shows the FunnelBarn brand', async () => {
    renderLogin()
    expect(await screen.findByText('Funnel')).toBeInTheDocument()
    expect(screen.getByText('Barn')).toBeInTheDocument()
  })

  it('calls login with username and password on submit', async () => {
    mockLogin.mockResolvedValue(undefined)
    renderLogin()

    fireEvent.change(await screen.findByLabelText(/username/i), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => expect(mockLogin).toHaveBeenCalledWith('admin', 'secret'))
  })

  it('navigates to /dashboard on successful login', async () => {
    mockLogin.mockResolvedValue(undefined)
    renderLogin()

    fireEvent.change(await screen.findByLabelText(/username/i), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/dashboard', { replace: true }))
  })

  it('shows invalid credentials error on 401', async () => {
    const { ApiError } = await import('../lib/api')
    mockLogin.mockRejectedValue(new ApiError(401, 'Unauthorized'))
    renderLogin()

    fireEvent.change(await screen.findByLabelText(/username/i), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'wrong' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() =>
      expect(screen.getByText(/invalid username or password/i)).toBeInTheDocument(),
    )
  })

  it('shows generic error message for non-401 failures', async () => {
    const { ApiError } = await import('../lib/api')
    mockLogin.mockRejectedValue(new ApiError(500, 'Server error'))
    renderLogin()

    fireEvent.change(await screen.findByLabelText(/username/i), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() =>
      expect(screen.getByText(/something went wrong/i)).toBeInTheDocument(),
    )
  })

  it('disables the submit button while submitting', async () => {
    let resolve: () => void
    mockLogin.mockReturnValue(new Promise<void>((r) => { resolve = r }))
    renderLogin()

    fireEvent.change(await screen.findByLabelText(/username/i), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /signing in/i })).toBeDisabled(),
    )
    resolve!()
  })

  it('does not render the form when SSO is configured', async () => {
    mockGetClientConfig.mockResolvedValue({ oidc: { enabled: true, loginURL: '/api/v1/oidc/login' } })
    renderLogin()
    // Spinner or redirect view — form inputs must not be present.
    await waitFor(() =>
      expect(screen.queryByLabelText(/username/i)).not.toBeInTheDocument(),
    )
  })
})
