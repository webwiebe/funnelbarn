import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { CopyButton } from './CopyButton'

describe('CopyButton', () => {
  it('shows "Copy" initially', () => {
    render(<CopyButton value="test-value" />)
    expect(screen.getByText('Copy')).toBeInTheDocument()
  })

  it('shows "Copied!" and calls clipboard.writeText after click', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    render(<CopyButton value="my-api-key" />)
    fireEvent.click(screen.getByText('Copy'))
    await waitFor(() => expect(screen.getByText('Copied!')).toBeInTheDocument())
    expect(writeText).toHaveBeenCalledWith('my-api-key')
  })
})
