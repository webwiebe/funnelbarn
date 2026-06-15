import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { statusBadge, originBadge } from './shared'

describe('statusBadge', () => {
  it('renders the inactive status label', () => {
    const { getByText } = render(<>{statusBadge('inactive')}</>)
    expect(getByText('inactive')).toBeTruthy()
  })
})

describe('originBadge', () => {
  it('renders "auto-detected" for auto-created flags', () => {
    const { getByText } = render(<>{originBadge('auto')}</>)
    expect(getByText('auto-detected')).toBeTruthy()
  })

  it('renders nothing for manual flags', () => {
    const { container } = render(<>{originBadge('manual')}</>)
    expect(container.textContent).toBe('')
  })
})
