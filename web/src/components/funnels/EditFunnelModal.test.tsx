import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { EditFunnelModal } from './EditFunnelModal'
import type { Funnel } from '../../lib/api'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockApi = vi.hoisted(() => ({
  updateFunnel: vi.fn(),
  getEventNames: vi.fn(),
  getEventProperties: vi.fn(),
  getEventPropertyValues: vi.fn(),
}))

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return { ...actual, api: mockApi }
})

const existingFunnel: Funnel = {
  id: 'f1',
  name: 'Checkout',
  scope: 'session',
  steps: [
    { step_order: 0, event_name: 'page_view' },
    { step_order: 1, event_name: 'purchase' },
  ],
}

function renderModal(overrides?: {
  funnel?: Funnel
  onClose?: () => void
  onUpdated?: (f: Funnel) => void
}) {
  const onClose = overrides?.onClose ?? vi.fn()
  const onUpdated = overrides?.onUpdated ?? vi.fn()
  render(
    <EditFunnelModal
      projectId="p1"
      funnel={overrides?.funnel ?? existingFunnel}
      onClose={onClose}
      onUpdated={onUpdated}
    />,
  )
  return { onClose, onUpdated }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('EditFunnelModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.getEventNames.mockResolvedValue({ event_names: [] })
    mockApi.getEventProperties.mockResolvedValue({ properties: [] })
    mockApi.getEventPropertyValues.mockResolvedValue({ values: [] })
    mockApi.updateFunnel.mockResolvedValue({ ...existingFunnel, name: 'Checkout v2' })
  })

  it('renders the "Edit funnel" heading', () => {
    renderModal()
    expect(screen.getByRole('heading', { name: /edit funnel/i })).toBeInTheDocument()
  })

  it('prefills the name and existing steps from the funnel prop', () => {
    renderModal()
    expect(screen.getByDisplayValue('Checkout')).toBeInTheDocument()
    expect(screen.getByDisplayValue('page_view')).toBeInTheDocument()
    expect(screen.getByDisplayValue('purchase')).toBeInTheDocument()
  })

  it('shows an error when the name is cleared', async () => {
    renderModal()
    fireEvent.change(screen.getByDisplayValue('Checkout'), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: /^save$/i }))
    await waitFor(() =>
      expect(screen.getByText(/name is required/i)).toBeInTheDocument(),
    )
    expect(mockApi.updateFunnel).not.toHaveBeenCalled()
  })

  it('shows an error when a step is cleared', async () => {
    renderModal()
    fireEvent.change(screen.getByDisplayValue('purchase'), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: /^save$/i }))
    await waitFor(() =>
      expect(screen.getByText(/all steps need an event name/i)).toBeInTheDocument(),
    )
    expect(mockApi.updateFunnel).not.toHaveBeenCalled()
  })

  it('saves the funnel and calls onUpdated', async () => {
    const { onUpdated } = renderModal()
    fireEvent.change(screen.getByDisplayValue('Checkout'), {
      target: { value: 'Checkout v2' },
    })
    fireEvent.click(screen.getByRole('button', { name: /^save$/i }))

    await waitFor(() => expect(mockApi.updateFunnel).toHaveBeenCalledTimes(1))
    expect(mockApi.updateFunnel).toHaveBeenCalledWith('p1', 'f1', {
      name: 'Checkout v2',
      steps: [{ event_name: 'page_view' }, { event_name: 'purchase' }],
      scope: 'session',
    })
    await waitFor(() => expect(onUpdated).toHaveBeenCalled())
  })

  it('shows an error banner when the API call fails', async () => {
    mockApi.updateFunnel.mockRejectedValue(new Error('update failed'))
    const { onUpdated } = renderModal()
    fireEvent.click(screen.getByRole('button', { name: /^save$/i }))
    await waitFor(() => expect(screen.getByText(/update failed/i)).toBeInTheDocument())
    expect(onUpdated).not.toHaveBeenCalled()
  })

  it('calls onClose when Cancel is clicked', () => {
    const { onClose } = renderModal()
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('falls back to two empty steps when the funnel has no steps', () => {
    renderModal({ funnel: { id: 'f2', name: 'Empty', scope: 'session', steps: [] } })
    expect(screen.getByDisplayValue('Empty')).toBeInTheDocument()
    const stepInputs = screen.getAllByPlaceholderText(/event name/i)
    expect(stepInputs.length).toBeGreaterThanOrEqual(2)
  })
})
