import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { ProjectMenu } from './ProjectMenu'
import { FAVOURITE_PROJECTS_KEY } from '../../lib/favouriteProjects'
import type { Project } from '../../lib/api'

const projects: Project[] = [
  { id: 'p3', name: 'zebra', slug: 'zebra', status: 'active' },
  { id: 'p1', name: 'Alpha', slug: 'alpha', status: 'active' },
  { id: 'p2', name: 'beta', slug: 'the-b-site', status: 'active' },
]

function names(): string[] {
  return screen.getAllByTitle(/.+/).map((el) => el.textContent ?? '')
}

describe('ProjectMenu', () => {
  beforeEach(() => window.localStorage.clear())

  it('lists projects alphabetically, case-insensitive', () => {
    render(<ProjectMenu projects={projects} onPick={vi.fn()} />)
    expect(names()).toEqual(['Alpha', 'beta', 'zebra'])
  })

  it('focuses the search field when asked to', () => {
    render(<ProjectMenu projects={projects} onPick={vi.fn()} autoFocusSearch />)
    expect(screen.getByRole('searchbox', { name: /search projects/i })).toHaveFocus()
  })

  it('filters on name and slug as you type', () => {
    render(<ProjectMenu projects={projects} onPick={vi.fn()} />)
    const box = screen.getByRole('searchbox')
    fireEvent.change(box, { target: { value: 'the-b' } })
    expect(names()).toEqual(['beta'])
  })

  it('shows "No projects match" with no hits', () => {
    render(<ProjectMenu projects={projects} onPick={vi.fn()} />)
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'nope' } })
    expect(screen.getByText('No projects match')).toBeInTheDocument()
  })

  it('picks the first match on Enter and closes on Escape', () => {
    const onPick = vi.fn()
    const onClose = vi.fn()
    render(<ProjectMenu projects={projects} onPick={onPick} onClose={onClose} />)
    const box = screen.getByRole('searchbox')
    fireEvent.change(box, { target: { value: 'a' } })
    fireEvent.keyDown(box, { key: 'Enter' })
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ id: 'p1' }))
    fireEvent.keyDown(box, { key: 'Escape' })
    expect(onClose).toHaveBeenCalled()
  })

  it('shows starred projects first in their own group, both alphabetical, and stores them', () => {
    render(<ProjectMenu projects={projects} onPick={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: /add zebra to favourites/i }))
    expect(names()).toEqual(['zebra', 'Alpha', 'beta'])
    expect(within(screen.getByRole('group', { name: 'Favourites' })).getByText('zebra')).toBeInTheDocument()
    expect(JSON.parse(window.localStorage.getItem(FAVOURITE_PROJECTS_KEY) ?? '[]')).toEqual(['p3'])
    fireEvent.click(screen.getByRole('button', { name: /remove zebra from favourites/i }))
    expect(names()).toEqual(['Alpha', 'beta', 'zebra'])
  })

  it('Enter picks a favourite before alphabetically earlier projects', () => {
    window.localStorage.setItem(FAVOURITE_PROJECTS_KEY, JSON.stringify(['p3']))
    const onPick = vi.fn()
    render(<ProjectMenu projects={projects} onPick={onPick} />)
    fireEvent.keyDown(screen.getByRole('searchbox'), { key: 'Enter' })
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ id: 'p3' }))
  })

  it('marks the current project and caps the list height so it scrolls inside', () => {
    const { container } = render(<ProjectMenu projects={projects} currentId="p2" onPick={vi.fn()} />)
    expect(screen.getByTitle('the-b-site')).toHaveAttribute('aria-current', 'true')
    const scroller = Array.from(container.querySelectorAll('div')).find((d) => d.style.maxHeight === '60vh')
    expect(scroller).toBeDefined()
    expect(scroller?.style.overflowY).toBe('auto')
  })
})
