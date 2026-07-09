import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { StepFilterEditor, type StepWithFilters } from './StepFilterEditor'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockApi = vi.hoisted(() => ({
  getEventNames: vi.fn(),
  getEventProperties: vi.fn(),
  getEventPropertyValues: vi.fn(),
}))

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return { ...actual, api: mockApi }
})

function renderEditor(
  props: Partial<React.ComponentProps<typeof StepFilterEditor>> = {},
) {
  const onChange = props.onChange ?? vi.fn()
  const steps: StepWithFilters[] = props.steps ?? [{ event_name: '', filters: [] }]
  const utils = render(
    <StepFilterEditor
      projectId={props.projectId ?? 'p1'}
      steps={steps}
      onChange={onChange}
      minSteps={props.minSteps}
      prefetchPropertiesFor={props.prefetchPropertiesFor}
    />,
  )
  return { onChange, ...utils }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('StepFilterEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.getEventNames.mockResolvedValue({ event_names: [] })
    mockApi.getEventProperties.mockResolvedValue({ properties: [] })
    mockApi.getEventPropertyValues.mockResolvedValue({ values: [] })
  })

  it('renders the Steps label and a numbered first step', () => {
    renderEditor()
    expect(screen.getByText('Steps')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument()
    expect(
      screen.getByPlaceholderText(/Event name, e\.g\. page_view/i),
    ).toBeInTheDocument()
  })

  it('fetches event names on mount for the project', async () => {
    renderEditor({ projectId: 'proj-9' })
    await waitFor(() =>
      expect(mockApi.getEventNames).toHaveBeenCalledWith('proj-9'),
    )
  })

  it('appends an empty step when "Add step" is clicked', () => {
    const { onChange } = renderEditor({ steps: [{ event_name: 'page_view', filters: [] }] })
    fireEvent.click(screen.getByText(/Add step/i))
    expect(onChange).toHaveBeenCalledWith([
      { event_name: 'page_view', filters: [] },
      { event_name: '', filters: [] },
    ])
  })

  it('does not show a remove button for a single step at default minSteps', () => {
    renderEditor({ steps: [{ event_name: '', filters: [] }] })
    // Only svg button present is the "Add step" button (it has text).
    const removeButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent === '' && b.querySelector('svg'))
    expect(removeButtons).toHaveLength(0)
  })

  it('removes a step when its remove button is clicked', () => {
    const steps: StepWithFilters[] = [
      { event_name: 'page_view', filters: [] },
      { event_name: 'purchase', filters: [] },
    ]
    const { onChange } = renderEditor({ steps })
    const removeButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent === '' && b.querySelector('svg'))
    expect(removeButtons.length).toBeGreaterThan(0)
    fireEvent.click(removeButtons[0])
    expect(onChange).toHaveBeenCalledWith([{ event_name: 'purchase', filters: [] }])
  })

  it('updates a step event name and fetches its properties', () => {
    const { onChange } = renderEditor({ steps: [{ event_name: '', filters: [] }] })
    const input = screen.getByPlaceholderText(/Event name, e\.g\. page_view/i)
    fireEvent.change(input, { target: { value: 'signup' } })
    expect(onChange).toHaveBeenCalledWith([{ event_name: 'signup', filters: [] }])
    expect(mockApi.getEventProperties).toHaveBeenCalledWith('p1', 'signup')
  })

  it('adds an empty filter row to a step', () => {
    const { onChange } = renderEditor({ steps: [{ event_name: 'page_view', filters: [] }] })
    fireEvent.click(screen.getByText('+ Add filter'))
    expect(onChange).toHaveBeenCalledWith([
      { event_name: 'page_view', filters: [{ property: '', value: '' }] },
    ])
  })

  it('renders existing filter rows with property/value inputs', () => {
    renderEditor({
      steps: [
        { event_name: 'page_view', filters: [{ property: 'url', value: '/home' }] },
      ],
    })
    const propInput = screen.getByDisplayValue('url')
    const valInput = screen.getByDisplayValue('/home')
    expect(propInput).toBeInTheDocument()
    expect(valInput).toBeInTheDocument()
  })

  it('updates a filter value via onChange', () => {
    const { onChange } = renderEditor({
      steps: [{ event_name: 'page_view', filters: [{ property: 'url', value: '' }] }],
    })
    fireEvent.change(screen.getByPlaceholderText('Value'), {
      target: { value: '/pricing' },
    })
    expect(onChange).toHaveBeenCalledWith([
      { event_name: 'page_view', filters: [{ property: 'url', value: '/pricing' }] },
    ])
  })

  it('shows the available events panel once event names load', async () => {
    mockApi.getEventNames.mockResolvedValue({
      event_names: ['page_view', 'purchase'],
    })
    renderEditor({ steps: [{ event_name: '', filters: [] }] })
    await waitFor(() =>
      expect(screen.getByText(/Available events \(2\)/i)).toBeInTheDocument(),
    )
  })

  it('selecting an available event fills the empty step', async () => {
    mockApi.getEventNames.mockResolvedValue({ event_names: ['checkout'] })
    const { onChange } = renderEditor({ steps: [{ event_name: '', filters: [] }] })
    await waitFor(() => screen.getByText(/Available events \(1\)/i))

    // expand the panel, then click the event chip
    fireEvent.click(screen.getByText(/Available events \(1\)/i))
    fireEvent.click(screen.getByRole('button', { name: 'checkout' }))
    expect(onChange).toHaveBeenCalledWith([{ event_name: 'checkout', filters: [] }])
  })

  it('renders property suggestion chips from prefetched properties and adds a filter on click', async () => {
    mockApi.getEventProperties.mockResolvedValue({ properties: ['url', 'referrer'] })
    const { onChange } = renderEditor({
      steps: [{ event_name: 'page_view', filters: [] }],
      prefetchPropertiesFor: ['page_view'],
    })

    await waitFor(() => expect(screen.getByText('+ url')).toBeInTheDocument())
    expect(screen.getByText('+ referrer')).toBeInTheDocument()

    fireEvent.click(screen.getByText('+ url'))
    expect(onChange).toHaveBeenCalledWith([
      { event_name: 'page_view', filters: [{ property: 'url', value: '' }] },
    ])
    // clicking a property chip pre-fetches candidate values for autocomplete
    expect(mockApi.getEventPropertyValues).toHaveBeenCalledWith('p1', 'page_view', 'url')
  })

  it('respects a higher minSteps by hiding the remove button', () => {
    renderEditor({
      steps: [
        { event_name: 'a', filters: [] },
        { event_name: 'b', filters: [] },
      ],
      minSteps: 2,
    })
    const removeButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent === '' && b.querySelector('svg'))
    expect(removeButtons).toHaveLength(0)
  })
})
