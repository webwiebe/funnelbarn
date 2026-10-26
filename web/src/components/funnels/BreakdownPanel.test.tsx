import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { BreakdownPanel } from './BreakdownPanel'
import type { Funnel } from '../../lib/api'

const mocks = vi.hoisted(() => ({ getEventProperties: vi.fn(), getFunnelBreakdown: vi.fn() }))
vi.mock('../../lib/api', () => ({ api: { getEventProperties: mocks.getEventProperties } }))
vi.mock('../../lib/api.funnelbreakdown', () => ({ getFunnelBreakdown: mocks.getFunnelBreakdown }))

const funnel: Funnel = {
  id: 'f1',
  name: 'Signup',
  scope: 'session',
  steps: [
    { step_order: 1, event_name: 'visit' },
    { step_order: 2, event_name: 'signup' },
  ],
}

const breakdown = {
  property: 'surface',
  groups: [
    {
      value: 'wordpress', label: 'wordpress', entries: 4,
      results: [
        { step_order: 1, event_name: 'visit', count: 4, conversion: 1, drop_off: 0 },
        { step_order: 2, event_name: 'signup', count: 3, conversion: 0.75, drop_off: 0.25 },
      ],
    },
    {
      value: '', label: '(none)', entries: 1,
      results: [
        { step_order: 1, event_name: 'visit', count: 1, conversion: 1, drop_off: 0 },
        { step_order: 2, event_name: 'signup', count: 0, conversion: 0, drop_off: 1 },
      ],
    },
  ],
}

// The panel keeps its breakdown in the URL; the probe shows what it wrote.
function LocationProbe() {
  return <output data-testid="search">{useLocation().search}</output>
}

function renderPanel(ui: ReactElement, search = '') {
  return render(<MemoryRouter initialEntries={[`/funnels/p1${search}`]}>{ui}<LocationProbe /></MemoryRouter>)
}

describe('BreakdownPanel', () => {
  beforeEach(() => {
    mocks.getEventProperties.mockReset()
    mocks.getFunnelBreakdown.mockReset()
    mocks.getEventProperties.mockResolvedValue({ properties: ['surface', 'plan'] })
    mocks.getFunnelBreakdown.mockResolvedValue(breakdown)
  })

  it('offers the properties of the first step event', async () => {
    renderPanel(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />)
    await screen.findByRole('option', { name: 'surface' })
    expect(mocks.getEventProperties).toHaveBeenCalledWith('p1', 'visit')
    expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual(['No breakdown', 'surface', 'plan'])
    expect(mocks.getFunnelBreakdown).not.toHaveBeenCalled()
  })

  it('shows one row per group with step counts and conversion', async () => {
    renderPanel(<BreakdownPanel projectId="p1" funnel={funnel} environment="production" segment="mobile" segmentId="seg1" />)
    await screen.findByRole('option', { name: 'surface' })
    fireEvent.change(screen.getByLabelText('Break down by'), { target: { value: 'surface' } })
    await screen.findByText('wordpress')
    expect(mocks.getFunnelBreakdown).toHaveBeenCalledWith('p1', 'f1', 'surface', {
      segment: 'mobile', segmentId: 'seg1', environment: 'production',
    })
    expect(screen.getByText('(none)')).toBeInTheDocument()
    expect(screen.getByText('(75.0%)')).toBeInTheDocument()
  })

  it('shows the failure when the breakdown cannot load', async () => {
    mocks.getFunnelBreakdown.mockRejectedValue(new Error('boom'))
    renderPanel(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />)
    await screen.findByRole('option', { name: 'surface' })
    fireEvent.change(screen.getByLabelText('Break down by'), { target: { value: 'surface' } })
    await screen.findByText(/Failed to load breakdown/)
  })

  it('renders nothing when the entry event has no properties', async () => {
    mocks.getEventProperties.mockResolvedValue({ properties: [] })
    renderPanel(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />)
    await waitFor(() => expect(mocks.getEventProperties).toHaveBeenCalled())
    expect(screen.queryByLabelText('Break down by')).not.toBeInTheDocument()
  })

  it('opens the breakdown a link names and marks its group', async () => {
    renderPanel(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />, '?funnel=f1&breakdown=surface&group=wordpress')
    const row = await screen.findByRole('row', { current: true })
    expect(row).toHaveTextContent('wordpress')
    expect(screen.getByLabelText('Break down by')).toHaveValue('surface')
    expect(mocks.getFunnelBreakdown).toHaveBeenCalledWith('p1', 'f1', 'surface', { segment: undefined, segmentId: undefined, environment: '' })
  })

  it('marks no row when the linked group is not in the breakdown', async () => {
    renderPanel(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />, '?breakdown=surface&group=lightroom')
    await screen.findByText('wordpress')
    expect(screen.queryByRole('row', { current: true })).not.toBeInTheDocument()
  })

  it('ignores a linked property the entry event does not carry', async () => {
    renderPanel(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />, '?breakdown=missing&group=wordpress')
    await screen.findByRole('option', { name: 'surface' })
    expect(screen.getByLabelText('Break down by')).toHaveValue('')
    expect(mocks.getFunnelBreakdown).not.toHaveBeenCalled()
  })

  it('ignores a linked property name the API would reject', async () => {
    mocks.getEventProperties.mockResolvedValue({ properties: ['surface', 'a-b'] })
    renderPanel(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />, '?breakdown=a-b')
    await screen.findByRole('option', { name: 'surface' })
    expect(screen.getByLabelText('Break down by')).toHaveValue('')
    expect(mocks.getFunnelBreakdown).not.toHaveBeenCalled()
  })

  it('writes the chosen property to the URL and drops the linked group', async () => {
    renderPanel(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />, '?funnel=f1&breakdown=surface&group=wordpress')
    await screen.findByRole('row', { current: true })
    fireEvent.change(screen.getByLabelText('Break down by'), { target: { value: 'plan' } })
    await waitFor(() => expect(screen.getByTestId('search')).toHaveTextContent('?funnel=f1&breakdown=plan'))
    fireEvent.change(screen.getByLabelText('Break down by'), { target: { value: '' } })
    await waitFor(() => expect(screen.getByTestId('search')).toHaveTextContent(/^\?funnel=f1$/))
  })
})
