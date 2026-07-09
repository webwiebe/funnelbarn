import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FlowStatsBar } from './FlowStatsBar'

describe('FlowStatsBar', () => {
  it('renders all three stat labels', () => {
    render(<FlowStatsBar focusedPage="/home" totalSessions={1234} nodeCount={5} />)
    expect(screen.getByText('Focused page')).toBeInTheDocument()
    expect(screen.getByText('Sessions')).toBeInTheDocument()
    expect(screen.getByText('Nodes')).toBeInTheDocument()
  })

  it('shows the focused page value', () => {
    render(<FlowStatsBar focusedPage="/pricing" totalSessions={0} nodeCount={0} />)
    expect(screen.getByText('/pricing')).toBeInTheDocument()
  })

  it('falls back to an em dash when focusedPage is empty', () => {
    render(<FlowStatsBar focusedPage="" totalSessions={0} nodeCount={0} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('formats the session count with locale separators', () => {
    render(<FlowStatsBar focusedPage="/home" totalSessions={1234567} nodeCount={3} />)
    expect(screen.getByText((1234567).toLocaleString())).toBeInTheDocument()
  })

  it('renders the node count', () => {
    render(<FlowStatsBar focusedPage="/home" totalSessions={10} nodeCount={42} />)
    expect(screen.getByText('42')).toBeInTheDocument()
  })

  it('renders zero values without crashing', () => {
    render(<FlowStatsBar focusedPage="" totalSessions={0} nodeCount={0} />)
    // em-dash placeholder for the empty page, plus both numeric stats at 0
    expect(screen.getByText('—')).toBeInTheDocument()
    expect(screen.getAllByText('0')).toHaveLength(2)
  })
})
