import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import type { FeatureFlag } from '../../lib/api'
import { FlagList } from './FlagList'

function manyFlags(n: number): FeatureFlag[] {
  const statuses = ['active', 'paused', 'inactive']
  return Array.from({ length: n }, (_, i) => ({
    id: `id-${i}`, project_id: 'p', flag_key: `flag_${i}`, name: `Flag ${i}`, flag_type: 'boolean',
    variants: '{}', default_variant: 'off', split: '{}', targeting_rules: '[]',
    status: statuses[i % 3], created_at: '2026-10-01 10:00:00',
    origin: i >= 30 ? 'auto' : 'manual', flag_kind: 'experiment',
  }))
}

function renderList(props: Partial<Parameters<typeof FlagList>[0]> = {}) {
  const handlers = { onRetry: vi.fn(), onSelect: vi.fn(), onCreate: vi.fn() }
  render(<FlagList flags={manyFlags(40)} error={null} selectedId={null} {...handlers} {...props} />)
  return handlers
}

const rows = () => within(screen.getByRole('list', { name: 'Flags' })).queryAllByRole('listitem')

describe('FlagList', () => {
  it('renders all 40 flags with a count', () => {
    renderList()
    expect(rows()).toHaveLength(40)
    expect(screen.getByTestId('flag-count')).toHaveTextContent('40 flags')
  })

  it('narrows the list by search and shows "x of y"', () => {
    renderList()
    fireEvent.change(screen.getByRole('searchbox', { name: 'Search flags' }), { target: { value: 'flag_3' } })
    // flag_3 and flag_30..flag_39
    expect(rows()).toHaveLength(11)
    expect(screen.getByTestId('flag-count')).toHaveTextContent('11 of 40 flags')
  })

  it('filters by status chip and origin, with counts on the chips', () => {
    renderList()
    const paused = screen.getByRole('button', { name: /Paused 13/ })
    fireEvent.click(paused)
    expect(paused).toHaveAttribute('aria-pressed', 'true')
    expect(rows()).toHaveLength(13)
    fireEvent.change(screen.getByRole('combobox', { name: 'Origin' }), { target: { value: 'auto' } })
    expect(rows()).toHaveLength(3)
  })

  it('shows a no-match state that clears the filters', () => {
    renderList()
    fireEvent.change(screen.getByRole('searchbox', { name: 'Search flags' }), { target: { value: 'nothing-like-this' } })
    expect(screen.getByText('No flags match these filters')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(rows()).toHaveLength(40)
  })

  it('reports the clicked flag', () => {
    const { onSelect } = renderList()
    fireEvent.click(screen.getByText('Flag 39'))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ flag_key: 'flag_39' }))
  })

  it('shows a loading state, never "No flags yet", before the first response', () => {
    renderList({ flags: null })
    expect(screen.getByLabelText('Loading flags')).toBeInTheDocument()
    expect(screen.queryByText('No flags yet')).not.toBeInTheDocument()
  })

  it('shows a load error with retry, never "No flags yet"', () => {
    const { onRetry } = renderList({ flags: null, error: 'HTTP 502' })
    expect(screen.getByRole('alert')).toHaveTextContent('Could not load flags')
    expect(screen.getByRole('alert')).toHaveTextContent('HTTP 502')
    expect(screen.queryByText('No flags yet')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('shows the empty state for a project without flags', () => {
    const { onCreate } = renderList({ flags: [] })
    expect(screen.getByText('No flags yet')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Create one' }))
    expect(onCreate).toHaveBeenCalledTimes(1)
  })
})
