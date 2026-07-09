import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ScopeToggle } from './ScopeToggle'

describe('ScopeToggle', () => {
  it('renders the Scope label and both options', () => {
    render(<ScopeToggle value="session" onChange={vi.fn()} />)
    expect(screen.getByText('Scope')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Session' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Page view' })).toBeInTheDocument()
  })

  it('marks the active option bold when value is session', () => {
    render(<ScopeToggle value="session" onChange={vi.fn()} />)
    const sessionBtn = screen.getByRole('button', { name: 'Session' })
    const pageViewBtn = screen.getByRole('button', { name: 'Page view' })
    expect(sessionBtn).toHaveStyle({ fontWeight: '700' })
    expect(pageViewBtn).toHaveStyle({ fontWeight: '400' })
  })

  it('marks the active option bold when value is page_view', () => {
    render(<ScopeToggle value="page_view" onChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Page view' })).toHaveStyle({ fontWeight: '700' })
    expect(screen.getByRole('button', { name: 'Session' })).toHaveStyle({ fontWeight: '400' })
  })

  it('calls onChange with page_view when clicking the inactive option', () => {
    const onChange = vi.fn()
    render(<ScopeToggle value="session" onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Page view' }))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('page_view')
  })

  it('calls onChange with session when clicking Session while on page_view', () => {
    const onChange = vi.fn()
    render(<ScopeToggle value="page_view" onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Session' }))
    expect(onChange).toHaveBeenCalledWith('session')
  })

  it('still fires onChange when clicking the already-active option', () => {
    const onChange = vi.fn()
    render(<ScopeToggle value="session" onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Session' }))
    expect(onChange).toHaveBeenCalledWith('session')
  })
})
