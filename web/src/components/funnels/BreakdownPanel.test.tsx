import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
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

describe('BreakdownPanel', () => {
  beforeEach(() => {
    mocks.getEventProperties.mockReset()
    mocks.getFunnelBreakdown.mockReset()
    mocks.getEventProperties.mockResolvedValue({ properties: ['surface', 'plan'] })
    mocks.getFunnelBreakdown.mockResolvedValue(breakdown)
  })

  it('offers the properties of the first step event', async () => {
    render(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />)
    await screen.findByRole('option', { name: 'surface' })
    expect(mocks.getEventProperties).toHaveBeenCalledWith('p1', 'visit')
    expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual(['No breakdown', 'surface', 'plan'])
    expect(mocks.getFunnelBreakdown).not.toHaveBeenCalled()
  })

  it('shows one row per group with step counts and conversion', async () => {
    render(<BreakdownPanel projectId="p1" funnel={funnel} environment="production" segment="mobile" segmentId="seg1" />)
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
    render(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />)
    await screen.findByRole('option', { name: 'surface' })
    fireEvent.change(screen.getByLabelText('Break down by'), { target: { value: 'surface' } })
    await screen.findByText(/Failed to load breakdown/)
  })

  it('renders nothing when the entry event has no properties', async () => {
    mocks.getEventProperties.mockResolvedValue({ properties: [] })
    const { container } = render(<BreakdownPanel projectId="p1" funnel={funnel} environment="" />)
    await waitFor(() => expect(mocks.getEventProperties).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })
})
