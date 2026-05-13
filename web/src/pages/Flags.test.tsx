import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { DefaultToggle, alignBooleanSplit } from './Flags'

describe('alignBooleanSplit', () => {
  it('self-heals legacy state where split contradicted default', () => {
    // Legacy: stored default=off but split says 100% on (the bug we shipped
    // from the old modal). Toggling to default=on should produce a
    // consistent state, not flip the split to {on:0, off:100} via mirror.
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 100, off: 0 }), 'on')))
      .toEqual({ on: 100, off: 0 })
  })

  it('preserves rollout percentage when swapping default', () => {
    // 80% on default, 20% rolled out → toggling default flips which side is
    // the rollout but keeps the percentage.
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 80, off: 20 }), 'off')))
      .toEqual({ off: 80, on: 20 })
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 20, off: 80 }), 'on')))
      .toEqual({ on: 80, off: 20 })
  })

  it('handles the trivial 100/0 case', () => {
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 0, off: 100 }), 'on')))
      .toEqual({ on: 100, off: 0 })
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 100, off: 0 }), 'off')))
      .toEqual({ off: 100, on: 0 })
  })

  it('handles 50/50 splits (no clear majority)', () => {
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 50, off: 50 }), 'on')))
      .toEqual({ on: 50, off: 50 })
  })

  it('treats missing keys as 0', () => {
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 30 }), 'on')))
      .toEqual({ on: 30, off: 0 })
  })

  it('falls back to all-zero on invalid JSON instead of throwing', () => {
    expect(JSON.parse(alignBooleanSplit('', 'on'))).toEqual({ on: 0, off: 0 })
    expect(JSON.parse(alignBooleanSplit('not json', 'off'))).toEqual({ off: 0, on: 0 })
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
