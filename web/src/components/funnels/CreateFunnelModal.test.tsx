import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { CreateFunnelModal } from './CreateFunnelModal'
import type { Funnel } from '../../lib/api'
import type { FunnelTemplate } from './constants'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockApi = vi.hoisted(() => ({
  createFunnel: vi.fn(),
  getEventNames: vi.fn(),
  getEventProperties: vi.fn(),
  getEventPropertyValues: vi.fn(),
}))

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return { ...actual, api: mockApi }
})

const mockTrackEvent = vi.hoisted(() => vi.fn())
vi.mock('../../lib/analytics', () => ({ trackEvent: mockTrackEvent }))

const createdFunnel: Funnel = {
  id: 'f1',
  name: 'Signup flow',
  scope: 'session',
  steps: [{ step_order: 0, event_name: 'page_view' }],
}

function renderModal(overrides?: {
  onClose?: () => void
  onCreated?: (f: Funnel) => void
  initialTemplate?: FunnelTemplate
}) {
  const onClose = overrides?.onClose ?? vi.fn()
  const onCreated = overrides?.onCreated ?? vi.fn()
  render(
    <CreateFunnelModal
      projectId="p1"
      onClose={onClose}
      onCreated={onCreated}
      initialTemplate={overrides?.initialTemplate}
    />,
  )
  return { onClose, onCreated }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('CreateFunnelModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.getEventNames.mockResolvedValue({ event_names: [] })
    mockApi.getEventProperties.mockResolvedValue({ properties: [] })
    mockApi.getEventPropertyValues.mockResolvedValue({ values: [] })
    mockApi.createFunnel.mockResolvedValue(createdFunnel)
  })

  it('renders the "Create funnel" heading', () => {
    renderModal()
    expect(screen.getByRole('heading', { name: /create funnel/i })).toBeInTheDocument()
  })

  it('renders the funnel name input and Create button', () => {
    renderModal()
    expect(screen.getByPlaceholderText(/signup flow/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^create$/i })).toBeInTheDocument()
  })

  it('shows an error when the name is empty', async () => {
    renderModal()
    fireEvent.click(screen.getByRole('button', { name: /^create$/i }))
    await waitFor(() =>
      expect(screen.getByText(/name is required/i)).toBeInTheDocument(),
    )
    expect(mockApi.createFunnel).not.toHaveBeenCalled()
  })

  it('shows an error when a step is missing its event name', async () => {
    renderModal()
    fireEvent.change(screen.getByPlaceholderText(/signup flow/i), {
      target: { value: 'My funnel' },
    })
    fireEvent.click(screen.getByRole('button', { name: /^create$/i }))
    await waitFor(() =>
      expect(screen.getByText(/all steps need an event name/i)).toBeInTheDocument(),
    )
    expect(mockApi.createFunnel).not.toHaveBeenCalled()
  })

  it('creates a funnel and calls onCreated when the form is valid', async () => {
    const { onCreated } = renderModal()
    fireEvent.change(screen.getByPlaceholderText(/signup flow/i), {
      target: { value: 'Signup flow' },
    })
    const stepInputs = screen.getAllByPlaceholderText(/event name/i)
    fireEvent.change(stepInputs[0], { target: { value: 'page_view' } })
    fireEvent.change(stepInputs[1], { target: { value: 'signup' } })

    fireEvent.click(screen.getByRole('button', { name: /^create$/i }))

    await waitFor(() => expect(mockApi.createFunnel).toHaveBeenCalledTimes(1))
    expect(mockApi.createFunnel).toHaveBeenCalledWith(
      'p1',
      'Signup flow',
      [{ event_name: 'page_view' }, { event_name: 'signup' }],
      'session',
    )
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(createdFunnel))
    expect(mockTrackEvent).toHaveBeenCalledWith('funnel_created', {
      funnel_name: 'Signup flow',
      step_count: 2,
    })
  })

  it('shows an error banner when the API call fails', async () => {
    mockApi.createFunnel.mockRejectedValue(new Error('boom'))
    const { onCreated } = renderModal()
    fireEvent.change(screen.getByPlaceholderText(/signup flow/i), {
      target: { value: 'Signup flow' },
    })
    const stepInputs = screen.getAllByPlaceholderText(/event name/i)
    fireEvent.change(stepInputs[0], { target: { value: 'page_view' } })
    fireEvent.change(stepInputs[1], { target: { value: 'signup' } })

    fireEvent.click(screen.getByRole('button', { name: /^create$/i }))

    await waitFor(() => expect(screen.getByText(/boom/i)).toBeInTheDocument())
    expect(onCreated).not.toHaveBeenCalled()
  })

  it('calls onClose when Cancel is clicked', () => {
    const { onClose } = renderModal()
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('prefills name, scope and steps from an initial template', () => {
    const template: FunnelTemplate = {
      name: 'E-commerce',
      scope: 'page_view',
      steps: ['page_view', 'add_to_cart', 'purchase'],
    }
    renderModal({ initialTemplate: template })
    expect(screen.getByDisplayValue('E-commerce')).toBeInTheDocument()
    expect(screen.getByDisplayValue('page_view')).toBeInTheDocument()
    expect(screen.getByDisplayValue('add_to_cart')).toBeInTheDocument()
    expect(screen.getByDisplayValue('purchase')).toBeInTheDocument()
  })
})
