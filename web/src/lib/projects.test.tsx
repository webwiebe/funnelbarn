import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import { ProjectProvider, useEffectiveProjectId, useProjects } from './projects'
import type { Project } from './api'

// ---------------------------------------------------------------------------
// Mock the API so ProjectProvider doesn't make real HTTP calls
// ---------------------------------------------------------------------------

const mockListProjects = vi.fn()

vi.mock('./api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listProjects: () => mockListProjects(),
    },
  }
})

// ---------------------------------------------------------------------------
// Mock localStorage with an in-memory store
// ---------------------------------------------------------------------------

const store: Record<string, string> = {}
vi.stubGlobal('localStorage', {
  getItem: (k: string) => store[k] ?? null,
  setItem: (k: string, v: string) => { store[k] = v },
  removeItem: (k: string) => { delete store[k] },
  clear: () => { Object.keys(store).forEach((k) => delete store[k]) },
})

// ---------------------------------------------------------------------------
// Helper component + render helper
// ---------------------------------------------------------------------------

function EffectiveIdDisplay({ urlProjectId }: { urlProjectId?: string }) {
  const id = useEffectiveProjectId(urlProjectId)
  return <div data-testid="result">{id ?? 'undefined'}</div>
}

function renderWithProjects(projects: Project[], urlProjectId?: string) {
  mockListProjects.mockResolvedValue({ projects })
  return render(
    <ProjectProvider>
      <EffectiveIdDisplay urlProjectId={urlProjectId} />
    </ProjectProvider>,
  )
}

// ---------------------------------------------------------------------------
// Fake projects
// ---------------------------------------------------------------------------

const projectA: Project = { id: 'proj-a', name: 'Alpha', slug: 'alpha', status: 'active' }
const projectB: Project = { id: 'proj-b', name: 'Beta', slug: 'beta', status: 'active' }

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useEffectiveProjectId', () => {
  beforeEach(() => {
    delete store['funnelbarn_default_project']
    vi.clearAllMocks()
  })

  it('returns urlProjectId when provided', async () => {
    renderWithProjects([projectA, projectB], 'proj-b')
    await waitFor(() => expect(screen.getByTestId('result').textContent).toBe('proj-b'))
  })

  it('falls back to defaultProjectId stored in localStorage when no urlProjectId', async () => {
    store['funnelbarn_default_project'] = 'proj-b'
    renderWithProjects([projectA, projectB])
    await waitFor(() => expect(screen.getByTestId('result').textContent).toBe('proj-b'))
  })

  it('ignores localStorage value when project is not in the list', async () => {
    store['funnelbarn_default_project'] = 'proj-gone'
    renderWithProjects([projectA, projectB])
    // Falls through to first project
    await waitFor(() => expect(screen.getByTestId('result').textContent).toBe('proj-a'))
  })

  it('falls back to the first project when no urlProjectId and no localStorage', async () => {
    renderWithProjects([projectA, projectB])
    await waitFor(() => expect(screen.getByTestId('result').textContent).toBe('proj-a'))
  })

  it('returns undefined when the project list is empty', async () => {
    renderWithProjects([])
    await waitFor(() => expect(screen.getByTestId('result').textContent).toBe('undefined'))
  })
})

// ---------------------------------------------------------------------------
// setDefaultProjectId tests
// ---------------------------------------------------------------------------

function DefaultIdConsumer({ onReady }: { onReady?: (fn: (id: string) => void) => void }) {
  const { defaultProjectId, setDefaultProjectId } = useProjects()
  if (onReady) onReady(setDefaultProjectId)
  return <div data-testid="default-id">{defaultProjectId ?? 'null'}</div>
}

describe('setDefaultProjectId', () => {
  beforeEach(() => {
    delete store['funnelbarn_default_project']
    vi.clearAllMocks()
  })

  it('persists to localStorage', async () => {
    mockListProjects.mockResolvedValue({ projects: [projectA, projectB] })
    let setter: ((id: string) => void) | undefined
    render(
      <ProjectProvider>
        <DefaultIdConsumer onReady={(fn) => { setter = fn }} />
      </ProjectProvider>
    )
    await waitFor(() => expect(screen.getByTestId('default-id')).toBeInTheDocument())
    act(() => { setter!('proj-b') })
    expect(store['funnelbarn_default_project']).toBe('proj-b')
  })

  it('updates defaultProjectId in context', async () => {
    mockListProjects.mockResolvedValue({ projects: [projectA, projectB] })
    let setter: ((id: string) => void) | undefined
    render(
      <ProjectProvider>
        <DefaultIdConsumer onReady={(fn) => { setter = fn }} />
      </ProjectProvider>
    )
    await waitFor(() => expect(screen.getByTestId('default-id')).toBeInTheDocument())
    act(() => { setter!('proj-b') })
    await waitFor(() => expect(screen.getByTestId('default-id').textContent).toBe('proj-b'))
  })
})
