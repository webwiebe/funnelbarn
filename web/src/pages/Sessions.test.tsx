import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, act, fireEvent } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Sessions from './Sessions'
import type { Recording } from '../lib/api'

vi.mock('../components/shell/Shell', () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))
vi.mock('../components/sessions/SessionReplay', () => ({ SessionReplay: () => null }))
vi.mock('../lib/projects', () => ({
  useProjects: () => ({ projects: [{ id: 'p1', name: 'Site', slug: 'site', status: 'active' }] }),
}))
vi.mock('../lib/analytics', () => ({ trackEvent: vi.fn() }))

const mockReportError = vi.hoisted(() => vi.fn())
vi.mock('../lib/bugbarn', () => ({ reportError: mockReportError }))

const mockApi = vi.hoisted(() => ({
  listRecordings: vi.fn(),
  getFunnelStepSessions: vi.fn(),
  getFlowPageSessions: vi.fn(),
  deleteRecording: vi.fn(),
}))
vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return { ...actual, api: mockApi }
})

function rec(over: Partial<Recording>): Recording {
  return {
    id: 'r1', project_id: 'p1', session_id: 'abcdefghijklmnop', environment: '',
    first_chunk_index: 0, last_chunk_index: 0, chunk_count: 1, has_snapshot: true,
    duration_ms: 1000, started_at: '2026-11-10T10:00:00Z', device_type: 'desktop',
    is_bot: false, ...over,
  }
}

function renderSessions(path = '/sessions/p1') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/sessions/:projectId" element={<Sessions />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('Sessions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a recording whose page_url is not an absolute URL', async () => {
    mockApi.listRecordings.mockResolvedValue({
      recordings: [rec({ page_url: '/relative/path?x=1' }), rec({ id: 'r2', page_url: 'not a url' })],
    })
    renderSessions()
    expect(await screen.findByText('2 shown')).toBeInTheDocument()
    expect(screen.getByText('/relative/path')).toBeInTheDocument()
    expect(screen.getByText('not a url')).toBeInTheDocument()
  })

  it('shows the path of an absolute page_url', async () => {
    mockApi.listRecordings.mockResolvedValue({ recordings: [rec({ page_url: 'https://e.test/a/b?q=1' })] })
    renderSessions()
    expect(await screen.findByText('/a/b')).toBeInTheDocument()
  })

  it('reports a failed load and shows it instead of an empty list', async () => {
    mockApi.listRecordings.mockRejectedValue(new Error('boom'))
    renderSessions()
    expect(await screen.findByText(/could not load recordings/i)).toBeInTheDocument()
    expect(screen.queryByText('No recordings found')).not.toBeInTheDocument()
    expect(mockReportError).toHaveBeenCalled()
  })

  it('keeps the newest load when responses arrive out of order', async () => {
    let resolveFirst: (v: { recordings: Recording[] }) => void = () => {}
    mockApi.listRecordings
      .mockImplementationOnce(() => new Promise((res) => { resolveFirst = res }))
      .mockResolvedValueOnce({ recordings: [rec({ id: 'new', page_url: 'https://e.test/newer' })] })
    renderSessions()
    await waitFor(() => expect(mockApi.listRecordings).toHaveBeenCalledTimes(1))
    // A filter change starts a second load while the first is still pending.
    fireEvent.click(screen.getByRole('button', { name: 'Mobile' }))
    expect(await screen.findByText('/newer')).toBeInTheDocument()
    await act(async () => {
      resolveFirst({ recordings: [rec({ id: 'old', page_url: 'https://e.test/older' })] })
    })
    expect(screen.queryByText('/older')).not.toBeInTheDocument()
    expect(screen.getByText('/newer')).toBeInTheDocument()
  })
})
