import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ScopeToggle } from './ScopeToggle'
import { windowDaysFromSeconds } from './constants'

describe('ScopeToggle', () => {
  it('renders the Scope label and both options', () => {
    render(<ScopeToggle value="session" onChange={vi.fn()} />)
    expect(screen.getByText('Scope')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Session' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Page view' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'User' })).toBeInTheDocument()
  })

  it('calls onChange with user when clicking User', () => {
    const onChange = vi.fn()
    render(<ScopeToggle value="session" onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'User' }))
    expect(onChange).toHaveBeenCalledWith('user')
  })

  it('shows the conversion window only for user scope', () => {
    const { rerender } = render(<ScopeToggle value="session" onChange={vi.fn()} windowDays={7} onWindowDaysChange={vi.fn()} />)
    expect(screen.queryByLabelText(/conversion window/i)).not.toBeInTheDocument()
    rerender(<ScopeToggle value="user" onChange={vi.fn()} windowDays={7} onWindowDaysChange={vi.fn()} />)
    expect(screen.getByLabelText(/conversion window/i)).toHaveValue(7)
  })

  it('reports a changed window in days', () => {
    const onWindowDaysChange = vi.fn()
    render(<ScopeToggle value="user" onChange={vi.fn()} windowDays={30} onWindowDaysChange={onWindowDaysChange} />)
    fireEvent.change(screen.getByLabelText(/conversion window/i), { target: { value: '10' } })
    expect(onWindowDaysChange).toHaveBeenCalledWith(10)
  })

  it('falls back to 30 days for a funnel without a stored window', () => {
    expect(windowDaysFromSeconds(undefined)).toBe(30)
    expect(windowDaysFromSeconds(2 * 24 * 3600)).toBe(2)
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
