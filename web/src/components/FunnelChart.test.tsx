import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import FunnelChart from './FunnelChart'
import { StepResult } from '../lib/api'

const sampleSteps: StepResult[] = [
  { step_order: 1, event_name: 'page_view', count: 1000, conversion: 1, drop_off: 0 },
  { step_order: 2, event_name: 'signup_start', count: 600, conversion: 0.6, drop_off: 0.4 },
  { step_order: 3, event_name: 'signup_complete', count: 300, conversion: 0.5, drop_off: 0.5 },
]

describe('FunnelChart', () => {
  it('renders a row for each step', () => {
    render(<FunnelChart results={sampleSteps} />)
    expect(screen.getByText('page_view')).toBeTruthy()
    expect(screen.getByText('signup_start')).toBeTruthy()
    expect(screen.getByText('signup_complete')).toBeTruthy()
  })

  it('shows "No step data." when results are empty', () => {
    render(<FunnelChart results={[]} />)
    expect(screen.getByText('No step data.')).toBeTruthy()
  })

  it('shows user counts for each step', () => {
    render(<FunnelChart results={sampleSteps} />)
    // 1,000 users for step 1
    expect(screen.getByText(/1,000 users/)).toBeTruthy()
    // 600 users for step 2
    expect(screen.getByText(/600 users/)).toBeTruthy()
  })

  it('shows drop-off information between steps', () => {
    render(<FunnelChart results={sampleSteps} />)
    // "dropped off" text should appear between steps
    const dropOffs = screen.getAllByText(/dropped off/)
    expect(dropOffs.length).toBe(2) // between step 1→2 and step 2→3
  })

  it('shows 100% for the first step', () => {
    render(<FunnelChart results={sampleSteps} />)
    expect(screen.getByText('100%')).toBeTruthy()
  })
})
