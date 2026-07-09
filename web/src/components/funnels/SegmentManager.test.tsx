import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { SegmentManager } from './SegmentManager'
import type { Segment } from '../../lib/api'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockApi = vi.hoisted(() => ({
  getSessionDistributions: vi.fn(),
  createSegment: vi.fn(),
  deleteSegment: vi.fn(),
}))

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return { ...actual, api: mockApi }
})

const mockTrackEvent = vi.hoisted(() => vi.fn())
vi.mock('../../lib/analytics', () => ({ trackEvent: mockTrackEvent }))

const mockSegments: Segment[] = [
  {
    id: 'seg1',
    project_id: 'p1',
    name: 'EU mobile',
    rules: [{ field: 'device_type', operator: 'eq', value: 'mobile' }],
    created_at: '2024-01-01T00:00:00Z',
  },
]

function renderManager(
  props: Partial<React.ComponentProps<typeof SegmentManager>> = {},
) {
  const onSegmentsChange = props.onSegmentsChange ?? vi.fn()
  render(
    <SegmentManager
      projectId={props.projectId ?? 'p1'}
      segments={props.segments ?? []}
      onSegmentsChange={onSegmentsChange}
    />,
  )
  return { onSegmentsChange }
}

// Expand the collapsed panel by clicking the "Segments" header button.
function expand() {
  fireEvent.click(screen.getByText('Segments'))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('SegmentManager', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.getSessionDistributions.mockResolvedValue({ distributions: {} })
  })

  it('renders the Segments header collapsed by default', () => {
    renderManager()
    expect(screen.getByText('Segments')).toBeInTheDocument()
    // Body content only appears once expanded.
    expect(screen.queryByText('Quick-start templates')).toBeNull()
  })

  it('shows a count badge when segments exist', () => {
    renderManager({ segments: mockSegments })
    expect(screen.getByText('1')).toBeInTheDocument()
  })

  it('fetches session distributions on mount for the project', async () => {
    renderManager({ projectId: 'proj-xyz' })
    await waitFor(() =>
      expect(mockApi.getSessionDistributions).toHaveBeenCalledWith('proj-xyz'),
    )
  })

  it('shows the empty state when expanded with no segments', () => {
    renderManager()
    expand()
    expect(screen.getByText('No segments yet.')).toBeInTheDocument()
  })

  it('renders existing segments with name and rule details when expanded', () => {
    renderManager({ segments: mockSegments })
    expand()
    expect(screen.getByText('EU mobile')).toBeInTheDocument()
    expect(screen.getByText('ID: seg1')).toBeInTheDocument()
    // rule renders field label, operator label and value
    expect(screen.getByText('Device type')).toBeInTheDocument()
    expect(screen.getByText('= equals')).toBeInTheDocument()
    expect(screen.getByText('mobile')).toBeInTheDocument()
  })

  it('renders quick-start templates when expanded and not creating', () => {
    renderManager()
    expand()
    expect(screen.getByText('Quick-start templates')).toBeInTheDocument()
    expect(screen.getByText('Mobile visitors')).toBeInTheDocument()
    expect(screen.getByText('Chrome users')).toBeInTheDocument()
  })

  it('clicking a template opens the create form pre-filled with its name', () => {
    renderManager()
    expand()
    fireEvent.click(screen.getByText('Dark mode users'))
    const nameInput = screen.getByPlaceholderText(/EU mobile users/i) as HTMLInputElement
    expect(nameInput.value).toBe('Dark mode users')
  })

  it('opens an empty create form via the "New segment" button', () => {
    renderManager()
    expand()
    fireEvent.click(screen.getByText(/New segment/i))
    expect(screen.getByText('Save segment')).toBeInTheDocument()
    const nameInput = screen.getByPlaceholderText(/EU mobile users/i) as HTMLInputElement
    expect(nameInput.value).toBe('')
  })

  it('shows a validation error and does not call the API when name is empty', async () => {
    renderManager()
    expand()
    fireEvent.click(screen.getByText(/New segment/i))
    fireEvent.click(screen.getByText('Save segment'))
    await waitFor(() =>
      expect(screen.getByText('Name is required')).toBeInTheDocument(),
    )
    expect(mockApi.createSegment).not.toHaveBeenCalled()
  })

  it('creates a segment with the entered name and default rule', async () => {
    const created: Segment = {
      id: 'seg2',
      project_id: 'p1',
      name: 'My segment',
      rules: [{ field: 'country_code', operator: 'eq', value: 'NL' }],
      created_at: '2024-06-01T00:00:00Z',
    }
    mockApi.createSegment.mockResolvedValue(created)
    const { onSegmentsChange } = renderManager({ segments: [] })
    expand()
    fireEvent.click(screen.getByText(/New segment/i))

    fireEvent.change(screen.getByPlaceholderText(/EU mobile users/i), {
      target: { value: 'My segment' },
    })
    // fill the default rule's value input
    fireEvent.change(screen.getByPlaceholderText(/NL, US, DE, FR/i), {
      target: { value: 'NL' },
    })
    fireEvent.click(screen.getByText('Save segment'))

    await waitFor(() =>
      expect(mockApi.createSegment).toHaveBeenCalledWith('p1', 'My segment', [
        { field: 'country_code', operator: 'eq', value: 'NL' },
      ]),
    )
    expect(onSegmentsChange).toHaveBeenCalledWith([created])
    expect(mockTrackEvent).toHaveBeenCalledWith(
      'segment_created',
      expect.objectContaining({ segment_name: 'My segment' }),
    )
  })

  it('adds and removes rule rows', () => {
    renderManager()
    expand()
    fireEvent.click(screen.getByText(/New segment/i))

    // one rule initially -> field + operator select
    expect(screen.getAllByRole('combobox')).toHaveLength(2)
    fireEvent.click(screen.getByText('+ Add rule'))
    expect(screen.getAllByRole('combobox')).toHaveLength(4)

    // inline X remove buttons only render with >1 rule: empty-text buttons
    // containing an svg icon (excludes the header/chevron which has text).
    const removeButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent === '' && b.querySelector('svg'))
    expect(removeButtons.length).toBeGreaterThan(0)
    fireEvent.click(removeButtons[0])
    expect(screen.getAllByRole('combobox')).toHaveLength(2)
  })

  it('hides the value input when the operator is "is empty"', () => {
    renderManager()
    expand()
    fireEvent.click(screen.getByText(/New segment/i))

    // value input is visible for default 'eq' operator
    expect(screen.getByPlaceholderText(/NL, US, DE, FR/i)).toBeInTheDocument()

    const operatorSelect = screen.getAllByRole('combobox')[1]
    fireEvent.change(operatorSelect, { target: { value: 'is_null' } })

    expect(screen.queryByPlaceholderText(/NL, US, DE, FR/i)).toBeNull()
  })

  it('changing the field select updates the value placeholder', () => {
    renderManager()
    expand()
    fireEvent.click(screen.getByText(/New segment/i))

    const fieldSelect = screen.getAllByRole('combobox')[0]
    fireEvent.change(fieldSelect, { target: { value: 'city' } })
    expect(screen.getByPlaceholderText(/Amsterdam, London/i)).toBeInTheDocument()
  })

  it('deletes a segment after confirmation', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    mockApi.deleteSegment.mockResolvedValue(undefined)
    const { onSegmentsChange } = renderManager({ segments: mockSegments })
    expand()

    fireEvent.click(screen.getByTitle('Delete segment'))

    await waitFor(() =>
      expect(mockApi.deleteSegment).toHaveBeenCalledWith('p1', 'seg1'),
    )
    expect(onSegmentsChange).toHaveBeenCalledWith([])
    confirmSpy.mockRestore()
  })

  it('does not delete when confirmation is cancelled', () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    renderManager({ segments: mockSegments })
    expand()

    fireEvent.click(screen.getByTitle('Delete segment'))
    expect(mockApi.deleteSegment).not.toHaveBeenCalled()
    confirmSpy.mockRestore()
  })

  it('cancels out of the create form', () => {
    renderManager()
    expand()
    fireEvent.click(screen.getByText(/New segment/i))
    expect(screen.getByText('Save segment')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Cancel'))
    expect(screen.queryByText('Save segment')).toBeNull()
  })
})
