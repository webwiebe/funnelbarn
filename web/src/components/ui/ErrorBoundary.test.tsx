import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ErrorBoundary } from './ErrorBoundary'

function Bomb({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) throw new Error('test explosion')
  return <div>safe content</div>
}

describe('ErrorBoundary', () => {
  it('renders children when no error', () => {
    render(<ErrorBoundary><Bomb shouldThrow={false} /></ErrorBoundary>)
    expect(screen.getByText('safe content')).toBeInTheDocument()
  })

  it('shows fallback UI when child throws', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(<ErrorBoundary><Bomb shouldThrow={true} /></ErrorBoundary>)
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(screen.getByText('test explosion')).toBeInTheDocument()
    spy.mockRestore()
  })

  it('try again button resets the error state', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { rerender } = render(<ErrorBoundary><Bomb shouldThrow={true} /></ErrorBoundary>)
    fireEvent.click(screen.getByText('Try again'))
    rerender(<ErrorBoundary><Bomb shouldThrow={false} /></ErrorBoundary>)
    expect(screen.getByText('safe content')).toBeInTheDocument()
    spy.mockRestore()
  })

  it('renders custom fallback when provided', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(
      <ErrorBoundary fallback={<div>custom fallback</div>}>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>
    )
    expect(screen.getByText('custom fallback')).toBeInTheDocument()
    spy.mockRestore()
  })
})
