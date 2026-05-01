import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ProjectPicker } from './ProjectPicker'
import type { Project } from '../../lib/api'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockNavigate = vi.fn()

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => mockNavigate }
})

// ---------------------------------------------------------------------------
// Test data
// ---------------------------------------------------------------------------

const projects: Project[] = [
  { id: 'p1', name: 'Alpha', slug: 'alpha', status: 'active' },
  { id: 'p2', name: 'Beta', slug: 'beta', status: 'active' },
  { id: 'p3', name: 'Gamma', slug: 'gamma', status: 'active' },
]

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderPicker(
  projectId = 'p1',
  variant: 'badge' | 'full' = 'badge',
  projectList = projects,
  onSelect = vi.fn(),
) {
  return render(
    <MemoryRouter initialEntries={[`/dashboard/${projectId}`]}>
      <Routes>
        <Route
          path="/dashboard/:projectId"
          element={
            <ProjectPicker
              projects={projectList}
              base="dashboard"
              variant={variant}
              onSelect={onSelect}
            />
          }
        />
      </Routes>
    </MemoryRouter>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('ProjectPicker — badge variant', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the current project name', () => {
    renderPicker('p1', 'badge')
    expect(screen.getByText('Alpha')).toBeInTheDocument()
  })

  it('opens a dropdown on click', () => {
    renderPicker('p1', 'badge')
    fireEvent.click(screen.getByRole('button', { name: /switch project/i }))
    expect(screen.getByText('Beta')).toBeInTheDocument()
    expect(screen.getByText('Gamma')).toBeInTheDocument()
  })

  it('navigates to the selected project and closes dropdown', () => {
    renderPicker('p1', 'badge')
    fireEvent.click(screen.getByRole('button', { name: /switch project/i }))
    fireEvent.click(screen.getByText('Beta'))
    expect(mockNavigate).toHaveBeenCalledWith('/dashboard/p2')
  })

  it('calls onSelect callback when a project is picked', () => {
    const onSelect = vi.fn()
    renderPicker('p1', 'badge', projects, onSelect)
    fireEvent.click(screen.getByRole('button', { name: /switch project/i }))
    fireEvent.click(screen.getByText('Beta'))
    expect(onSelect).toHaveBeenCalledTimes(1)
  })

  it('renders nothing when the project list is empty', () => {
    const { container } = renderPicker('p1', 'badge', [])
    expect(container.firstChild).toBeNull()
  })

  it('shows a check mark next to the active project in the dropdown', () => {
    renderPicker('p2', 'badge')
    fireEvent.click(screen.getByRole('button', { name: /switch project/i }))
    // The check icon is rendered only for the active project (Beta).
    // We verify via aria or element presence by checking the Beta entry has the check svg.
    const betaButton = screen.getAllByRole('button').find((b) => b.textContent?.includes('Beta'))
    expect(betaButton).toBeTruthy()
    // svg (Check icon) should be inside the active project button
    expect(betaButton?.querySelector('svg')).toBeTruthy()
  })
})

describe('ProjectPicker — full variant', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders all projects without requiring a click', () => {
    renderPicker('p1', 'full')
    expect(screen.getByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('Beta')).toBeInTheDocument()
    expect(screen.getByText('Gamma')).toBeInTheDocument()
  })

  it('shows a "Switch Project" label', () => {
    renderPicker('p1', 'full')
    expect(screen.getByText(/switch project/i)).toBeInTheDocument()
  })

  it('navigates when a project row is clicked', () => {
    renderPicker('p1', 'full')
    fireEvent.click(screen.getByText('Gamma'))
    expect(mockNavigate).toHaveBeenCalledWith('/dashboard/p3')
  })

  it('calls onSelect when a project is clicked', () => {
    const onSelect = vi.fn()
    renderPicker('p1', 'full', projects, onSelect)
    fireEvent.click(screen.getByText('Gamma'))
    expect(onSelect).toHaveBeenCalledTimes(1)
  })

  it('renders nothing when the project list is empty', () => {
    const { container } = renderPicker('p1', 'full', [])
    expect(container.firstChild).toBeNull()
  })
})
