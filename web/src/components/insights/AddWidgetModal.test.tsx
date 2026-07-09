import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { AddWidgetModal } from './AddWidgetModal'

const mockApi = vi.hoisted(() => ({
  getEventNames: vi.fn(),
  getEventProperties: vi.fn(),
  createWidget: vi.fn(),
}))

vi.mock('../../lib/api', () => ({ api: mockApi }))

function renderModal(overrides: Partial<{ onClose: () => void; onAdded: () => void }> = {}) {
  const onClose = overrides.onClose ?? vi.fn()
  const onAdded = overrides.onAdded ?? vi.fn()
  render(<AddWidgetModal projectId="p1" onClose={onClose} onAdded={onAdded} />)
  return { onClose, onAdded }
}

describe('AddWidgetModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.getEventNames.mockResolvedValue({ event_names: ['signup', 'purchase'] })
    mockApi.getEventProperties.mockResolvedValue({ properties: ['plan', 'amount'] })
    mockApi.createWidget.mockResolvedValue({ id: 'w1' })
  })

  it('renders the modal heading and form fields', async () => {
    renderModal()
    expect(screen.getByRole('heading', { name: /add widget/i })).toBeInTheDocument()
    expect(screen.getByText('Event Name')).toBeInTheDocument()
    expect(screen.getByText('Property (optional)')).toBeInTheDocument()
    expect(screen.getByText('Title (optional)')).toBeInTheDocument()
    // Event names load asynchronously.
    await waitFor(() => expect(screen.getByRole('option', { name: 'signup' })).toBeInTheDocument())
  })

  it('loads event names on mount for the given project', async () => {
    renderModal()
    await waitFor(() => expect(mockApi.getEventNames).toHaveBeenCalledWith('p1'))
  })

  it('loads properties after an event is selected', async () => {
    renderModal()
    await waitFor(() => expect(screen.getByRole('option', { name: 'signup' })).toBeInTheDocument())
    const [eventSelect] = screen.getAllByRole('combobox')
    fireEvent.change(eventSelect, { target: { value: 'signup' } })
    await waitFor(() => expect(mockApi.getEventProperties).toHaveBeenCalledWith('p1', 'signup'))
    await waitFor(() => expect(screen.getByRole('option', { name: 'plan' })).toBeInTheDocument())
  })

  it('creates a widget with a default title and calls onAdded on save', async () => {
    const { onAdded } = renderModal()
    await waitFor(() => expect(screen.getByRole('option', { name: 'signup' })).toBeInTheDocument())
    const [eventSelect] = screen.getAllByRole('combobox')
    fireEvent.change(eventSelect, { target: { value: 'signup' } })

    fireEvent.click(screen.getByRole('button', { name: /add widget/i }))
    await waitFor(() =>
      expect(mockApi.createWidget).toHaveBeenCalledWith('p1', {
        event_name: 'signup',
        property: '',
        title: 'signup (count)',
      }),
    )
    await waitFor(() => expect(onAdded).toHaveBeenCalled())
  })

  it('uses the typed title when provided', async () => {
    renderModal()
    await waitFor(() => expect(screen.getByRole('option', { name: 'signup' })).toBeInTheDocument())
    const [eventSelect] = screen.getAllByRole('combobox')
    fireEvent.change(eventSelect, { target: { value: 'purchase' } })
    fireEvent.change(screen.getByPlaceholderText(/purchase/i), { target: { value: 'My widget' } })

    fireEvent.click(screen.getByRole('button', { name: /add widget/i }))
    await waitFor(() =>
      expect(mockApi.createWidget).toHaveBeenCalledWith('p1', {
        event_name: 'purchase',
        property: '',
        title: 'My widget',
      }),
    )
  })

  it('does not save while no event is selected', async () => {
    renderModal()
    await waitFor(() => expect(mockApi.getEventNames).toHaveBeenCalled())
    const saveButton = screen.getByRole('button', { name: /add widget/i })
    expect(saveButton).toBeDisabled()
    fireEvent.click(saveButton)
    expect(mockApi.createWidget).not.toHaveBeenCalled()
  })

  it('calls onClose when the Cancel button is clicked', async () => {
    const { onClose } = renderModal()
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onClose).toHaveBeenCalled()
  })

  it('calls onClose when the overlay is clicked', async () => {
    const { onClose } = renderModal()
    // The overlay is the fixed full-screen backdrop behind the dialog (z-index 1000).
    fireEvent.click(document.querySelector('div[style*="z-index: 1000"]')!)
    expect(onClose).toHaveBeenCalled()
  })
})
