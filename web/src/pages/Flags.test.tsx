import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { DefaultToggle, mirrorBooleanSplit } from './Flags'

describe('mirrorBooleanSplit', () => {
  it('swaps on and off percentages', () => {
    expect(JSON.parse(mirrorBooleanSplit(JSON.stringify({ on: 20, off: 80 })))).toEqual({ on: 80, off: 20 })
  })

  it('preserves the 100/0 case (no rollout)', () => {
    expect(JSON.parse(mirrorBooleanSplit(JSON.stringify({ on: 0, off: 100 })))).toEqual({ on: 100, off: 0 })
    expect(JSON.parse(mirrorBooleanSplit(JSON.stringify({ on: 100, off: 0 })))).toEqual({ on: 0, off: 100 })
  })

  it('treats missing keys as 0', () => {
    // A flag stored with only one key (legacy or partial) shouldn't blow up.
    expect(JSON.parse(mirrorBooleanSplit(JSON.stringify({ on: 30 })))).toEqual({ on: 0, off: 30 })
    expect(JSON.parse(mirrorBooleanSplit(JSON.stringify({ off: 50 })))).toEqual({ on: 50, off: 0 })
  })

  it('falls back to {on:0, off:0} on invalid JSON instead of throwing', () => {
    expect(JSON.parse(mirrorBooleanSplit(''))).toEqual({ on: 0, off: 0 })
    expect(JSON.parse(mirrorBooleanSplit('not json'))).toEqual({ on: 0, off: 0 })
  })
})

describe('DefaultToggle', () => {
  it('renders "on" label when value is true', () => {
    render(<DefaultToggle value={true} onChange={() => {}} />)
    expect(screen.getByText('on')).toBeInTheDocument()
  })

  it('renders "off" label when value is false', () => {
    render(<DefaultToggle value={false} onChange={() => {}} />)
    expect(screen.getByText('off')).toBeInTheDocument()
  })

  it('calls onChange when clicked', () => {
    const onChange = vi.fn()
    render(<DefaultToggle value={false} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button'))
    expect(onChange).toHaveBeenCalledTimes(1)
  })

  it('does not call onChange when disabled', () => {
    const onChange = vi.fn()
    render(<DefaultToggle value={false} onChange={onChange} disabled />)
    fireEvent.click(screen.getByRole('button'))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('hint title reflects current state and proposed flip direction', () => {
    const { rerender } = render(<DefaultToggle value={true} onChange={() => {}} />)
    expect(screen.getByRole('button')).toHaveAttribute('title', expect.stringContaining('flip to off'))
    rerender(<DefaultToggle value={false} onChange={() => {}} />)
    expect(screen.getByRole('button')).toHaveAttribute('title', expect.stringContaining('flip to on'))
  })
})
