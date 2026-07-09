import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { SankeyChart } from './SankeyChart'
import type { FlowData } from '../../lib/api'

// A small but valid flow: one focused page fanning out to another page and a
// drop-off (exit) node. d3-sankey computes a real layout from this data.
const flow: FlowData = {
  focused_page: 'https://example.com/',
  total_sessions: 100,
  nodes: [
    { id: 'home', label: 'https://example.com/', type: 'page', depth: 0, sessions: 100 },
    { id: 'pricing', label: 'https://example.com/pricing', type: 'page', depth: 1, sessions: 40 },
    { id: 'exit', label: '(drop-off)', type: 'exit', depth: 1, sessions: 60 },
  ],
  links: [
    { source: 'home', target: 'pricing', value: 40 },
    { source: 'home', target: 'exit', value: 60 },
  ],
}

describe('SankeyChart', () => {
  it('renders an svg with node labels for a valid dataset', () => {
    const { container } = render(<SankeyChart data={flow} onNodeClick={vi.fn()} width={600} />)
    expect(container.querySelector('svg')).toBeInTheDocument()
    // pathLabel strips the domain to the last path segment.
    expect(screen.getByText('pricing', { selector: 'text' })).toBeInTheDocument()
    // Special "(…)" labels are kept verbatim (both a <title> and a <text> carry it).
    expect(screen.getByText('(drop-off)', { selector: 'text' })).toBeInTheDocument()
  })

  it('fires onNodeClick with the node label when a clickable node is clicked', () => {
    const onNodeClick = vi.fn()
    render(<SankeyChart data={flow} onNodeClick={onNodeClick} width={600} />)
    // The click handler lives on the wrapping <g>; clicking the label bubbles up.
    fireEvent.click(screen.getByText('pricing'))
    expect(onNodeClick).toHaveBeenCalledWith('https://example.com/pricing')
  })

  it('does not fire onNodeClick for exit (drop-off) nodes', () => {
    const onNodeClick = vi.fn()
    render(<SankeyChart data={flow} onNodeClick={onNodeClick} width={600} />)
    fireEvent.click(screen.getByText('(drop-off)', { selector: 'text' }))
    expect(onNodeClick).not.toHaveBeenCalled()
  })

  it('renders a fallback message for a degenerate dataset with no links', () => {
    const empty: FlowData = { ...flow, nodes: flow.nodes, links: [] }
    render(<SankeyChart data={empty} onNodeClick={vi.fn()} width={600} />)
    expect(screen.getByText(/Not enough flow data/i)).toBeInTheDocument()
  })

  it('renders nothing when the available width is too small', () => {
    const { container } = render(<SankeyChart data={flow} onNodeClick={vi.fn()} width={80} />)
    // margins consume more than the width -> innerW <= 0 -> null
    expect(container).toBeEmptyDOMElement()
  })
})
