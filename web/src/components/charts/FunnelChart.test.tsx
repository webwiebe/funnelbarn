import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import FunnelChart from './FunnelChart'
import type { StepResult } from '../../lib/api'

function makeStep(overrides: Partial<StepResult> & { step_order: number; event_name: string }): StepResult {
  return {
    count: 100,
    conversion: 1,
    drop_off: 0,
    ...overrides,
  }
}

describe('FunnelChart', () => {
  it('shows placeholder text when results array is empty', () => {
    render(<FunnelChart results={[]} />)
    expect(screen.getByText('No step data.')).toBeInTheDocument()
  })

  it('renders a single step without crashing', () => {
    const steps = [makeStep({ step_order: 1, event_name: 'page_view', count: 500 })]
    render(<FunnelChart results={steps} />)
    expect(screen.getByText('page_view')).toBeInTheDocument()
  })

  it('renders all step names for multiple steps', () => {
    const steps = [
      makeStep({ step_order: 1, event_name: 'page_view', count: 1000 }),
      makeStep({ step_order: 2, event_name: 'sign_up', count: 400, conversion: 0.4, drop_off: 0.6 }),
      makeStep({ step_order: 3, event_name: 'checkout', count: 100, conversion: 0.25, drop_off: 0.75 }),
    ]
    render(<FunnelChart results={steps} />)
    expect(screen.getByText('page_view')).toBeInTheDocument()
    expect(screen.getByText('sign_up')).toBeInTheDocument()
    expect(screen.getByText('checkout')).toBeInTheDocument()
  })

  it('shows user counts for each step', () => {
    const steps = [
      makeStep({ step_order: 1, event_name: 'start', count: 200 }),
      makeStep({ step_order: 2, event_name: 'finish', count: 50, conversion: 0.25, drop_off: 0.75 }),
    ]
    render(<FunnelChart results={steps} />)
    expect(screen.getByText(/200 users/)).toBeInTheDocument()
    expect(screen.getByText(/50 users/)).toBeInTheDocument()
  })

  it('shows "dropped off" annotation between steps', () => {
    const steps = [
      makeStep({ step_order: 1, event_name: 'step_one', count: 100 }),
      makeStep({ step_order: 2, event_name: 'step_two', count: 60, conversion: 0.6, drop_off: 0.4 }),
    ]
    render(<FunnelChart results={steps} />)
    expect(screen.getByText(/dropped off/i)).toBeInTheDocument()
  })
})
