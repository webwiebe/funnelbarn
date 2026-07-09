import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ImplementationSnippet } from './ImplementationSnippet'
import type { Funnel } from '../../lib/api'

const funnel: Funnel = {
  id: 'f1',
  name: 'Signup flow',
  scope: 'session',
  steps: [
    { step_order: 0, event_name: 'page_view' },
    { step_order: 1, event_name: 'signup_started' },
    { step_order: 2, event_name: 'signup_completed' },
  ],
}

describe('ImplementationSnippet', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the Implementation header and Copy button', () => {
    render(<ImplementationSnippet funnel={funnel} />)
    expect(screen.getByText('Implementation')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument()
  })

  it('renders a tab for every supported language', () => {
    render(<ImplementationSnippet funnel={funnel} />)
    for (const lang of ['JS', 'React', 'Go', 'Python', 'Swift', 'Kotlin']) {
      expect(screen.getByRole('button', { name: lang })).toBeInTheDocument()
    }
  })

  it('shows the JS snippet by default with a camelCased const and step entries', () => {
    render(<ImplementationSnippet funnel={funnel} />)
    const pre = document.querySelector('pre')!
    expect(pre.textContent).toContain('const SignupFlowFunnel = {')
    expect(pre.textContent).toContain("pageView: 'page_view'")
    expect(pre.textContent).toContain('funnelbarn.page')
  })

  it('uses the provided apiKey in the snippet', () => {
    render(<ImplementationSnippet funnel={funnel} apiKey="MY_SECRET_KEY" />)
    const pre = document.querySelector('pre')!
    expect(pre.textContent).toContain('MY_SECRET_KEY')
  })

  it('falls back to a placeholder api key when none is provided', () => {
    render(<ImplementationSnippet funnel={funnel} />)
    const pre = document.querySelector('pre')!
    expect(pre.textContent).toContain('YOUR_INGEST_API_KEY')
  })

  it('switches to the React snippet when the React tab is clicked', () => {
    render(<ImplementationSnippet funnel={funnel} />)
    fireEvent.click(screen.getByRole('button', { name: 'React' }))
    const pre = document.querySelector('pre')!
    expect(pre.textContent).toContain("import { useEffect } from 'react'")
    expect(pre.textContent).toContain('useFunnelTrack')
  })

  it('switches to the Go snippet when the Go tab is clicked', () => {
    render(<ImplementationSnippet funnel={funnel} />)
    fireEvent.click(screen.getByRole('button', { name: 'Go' }))
    const pre = document.querySelector('pre')!
    expect(pre.textContent).toContain('package main')
    expect(pre.textContent).toContain('funnelbarn.Init')
  })

  it('copies the snippet to the clipboard and shows a confirmation', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    render(<ImplementationSnippet funnel={funnel} />)
    fireEvent.click(screen.getByRole('button', { name: /copy/i }))
    await waitFor(() => expect(screen.getByText(/copied/i)).toBeInTheDocument())
    expect(writeText).toHaveBeenCalledTimes(1)
    expect(writeText.mock.calls[0][0]).toContain('const SignupFlowFunnel = {')
  })

  it('renders default fallback step names when the funnel has no steps', () => {
    const empty: Funnel = { id: 'f2', name: 'Empty', scope: 'session', steps: [] }
    render(<ImplementationSnippet funnel={empty} />)
    const pre = document.querySelector('pre')!
    // Const name is derived from the funnel name; empty steps still render a snippet.
    expect(pre.textContent).toContain('const EmptyFunnel = {')
    // midStep falls back to 'button_click', rendered camelCased in the track() call
    expect(pre.textContent).toContain('EmptyFunnel.buttonClick')
  })
})
