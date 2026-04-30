// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import FirstRunWizard from './FirstRunWizard'
import { ApiError } from '../../lib/api'

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return {
    ...actual,
    api: {
      createProject: vi.fn(),
      createApiKey: vi.fn(),
    },
  }
})

// Import after mock so we get the mocked version
import { api } from '../../lib/api'

afterEach(() => {
  vi.clearAllMocks()
})

function renderWizard(onComplete = vi.fn()) {
  return render(<FirstRunWizard onComplete={onComplete} />)
}

describe('FirstRunWizard', () => {
  it('renders the first step initially', () => {
    renderWizard()
    expect(screen.getByText('Welcome to FunnelBarn')).toBeInTheDocument()
  })

  it('shows project name input after advancing past step 1', async () => {
    renderWizard()
    fireEvent.click(screen.getByText("Let's go →"))
    expect(screen.getByPlaceholderText('e.g. My SaaS App')).toBeInTheDocument()
  })

  it('calls onComplete when wizard finishes', async () => {
    const onComplete = vi.fn()
    vi.mocked(api.createProject).mockResolvedValue({
      id: 'p1',
      name: 'My App',
      slug: 'my-app',
      status: 'active',
    })
    vi.mocked(api.createApiKey).mockResolvedValue({
      api_key: { id: 'k1', name: 'My App ingest', scope: 'ingest', created_at: '' },
      key: 'fb_testkey',
    })

    renderWizard(onComplete)

    // Step 1 → step 2
    fireEvent.click(screen.getByText("Let's go →"))

    // Fill in project name
    const nameInput = screen.getByPlaceholderText('e.g. My SaaS App')
    fireEvent.change(nameInput, { target: { value: 'My App' } })

    // Submit step 2
    await act(async () => {
      fireEvent.click(screen.getByText('Create project →'))
    })

    // Now on step 3 — advance to step 4
    await waitFor(() => expect(screen.getByText('Done, continue →')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Done, continue →'))

    // Step 4 — click Go to dashboard
    await waitFor(() => expect(screen.getByText('Go to dashboard →')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Go to dashboard →'))

    expect(onComplete).toHaveBeenCalledTimes(1)
  })

  it('shows error message when API call fails with 422', async () => {
    vi.mocked(api.createProject).mockRejectedValue(
      new ApiError(422, 'name: required')
    )

    renderWizard()

    // Advance to step 2
    fireEvent.click(screen.getByText("Let's go →"))

    // Leave name blank and submit — the component checks for empty name client-side
    // Provide a non-empty name so the API call is actually made
    const nameInput = screen.getByPlaceholderText('e.g. My SaaS App')
    fireEvent.change(nameInput, { target: { value: 'Bad Project' } })

    await act(async () => {
      fireEvent.click(screen.getByText('Create project →'))
    })

    await waitFor(() =>
      expect(screen.getByText(/ApiError: name: required/i)).toBeInTheDocument()
    )
  })

  it('shows client-side validation error when project name is empty', async () => {
    renderWizard()

    // Advance to step 2
    fireEvent.click(screen.getByText("Let's go →"))

    // Click Create without filling in name
    fireEvent.click(screen.getByText('Create project →'))

    expect(screen.getByText('Project name is required')).toBeInTheDocument()
  })
})
