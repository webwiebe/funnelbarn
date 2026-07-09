import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { FlowBreadcrumb } from './FlowBreadcrumb'

describe('FlowBreadcrumb', () => {
  it('renders nothing when there are no crumbs', () => {
    const { container } = render(
      <FlowBreadcrumb crumbs={[]} onCrumbClick={vi.fn()} onReset={vi.fn()} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders each crumb as a button', () => {
    render(
      <FlowBreadcrumb
        crumbs={['/home', '/pricing', '/checkout']}
        onCrumbClick={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: '/home' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '/pricing' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '/checkout' })).toBeInTheDocument()
  })

  it('calls onCrumbClick with the crumb index', () => {
    const onCrumbClick = vi.fn()
    render(
      <FlowBreadcrumb
        crumbs={['/home', '/pricing', '/checkout']}
        onCrumbClick={onCrumbClick}
        onReset={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: '/pricing' }))
    expect(onCrumbClick).toHaveBeenCalledWith(1)
  })

  it('calls onReset when clicking the Home button', () => {
    const onReset = vi.fn()
    render(
      <FlowBreadcrumb crumbs={['/home']} onCrumbClick={vi.fn()} onReset={onReset} />,
    )
    fireEvent.click(screen.getByRole('button', { name: /reset to top page/i }))
    expect(onReset).toHaveBeenCalledTimes(1)
  })

  it('does not show the Reset button with a single crumb', () => {
    render(
      <FlowBreadcrumb crumbs={['/home']} onCrumbClick={vi.fn()} onReset={vi.fn()} />,
    )
    expect(screen.queryByRole('button', { name: /^reset$/i })).toBeNull()
  })

  it('shows the Reset button and calls onReset when more than one crumb', () => {
    const onReset = vi.fn()
    render(
      <FlowBreadcrumb
        crumbs={['/home', '/pricing']}
        onCrumbClick={vi.fn()}
        onReset={onReset}
      />,
    )
    const resetBtn = screen.getByRole('button', { name: /^reset$/i })
    expect(resetBtn).toBeInTheDocument()
    fireEvent.click(resetBtn)
    expect(onReset).toHaveBeenCalledTimes(1)
  })

  it('highlights the last crumb as active (amber, bold)', () => {
    render(
      <FlowBreadcrumb
        crumbs={['/home', '/pricing']}
        onCrumbClick={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: '/pricing' })).toHaveStyle({ fontWeight: '600' })
    expect(screen.getByRole('button', { name: '/home' })).toHaveStyle({ fontWeight: '400' })
  })
})
