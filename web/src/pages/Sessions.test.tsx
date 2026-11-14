import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup, fireEvent, act } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Sessions from './Sessions'
import type { Recording } from '../lib/api'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('../lib/projects', () => ({
  useProjects: () => ({
    projects: [{ id: 'p1', name: 'My Site', slug: 'my-site', status: 'active' }],
    isLoading: false,
    refetch: vi.fn(),
    defaultProjectId: 'p1',
    selectedEnvironment: '',
    setSelectedEnvironment: vi.fn(),
  }),
}))

// The page chrome has its own tests; rendering it here only slows these down.
vi.mock('../components/shell/Shell', () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('../components/sessions/SessionReplay', () => ({ SessionReplay: () => null }))

vi.mock('../lib/analytics', () => ({ trackEvent: vi.fn(), trackPageView: vi.fn() }))

const mockReportError = vi.hoisted(() => vi.fn())
vi.mock('../lib/bugbarn', () => ({ reportError: mockReportError }))

const mockApi = vi.hoisted(() => ({
  listRecordings: vi.fn(),
  getFunnelStepSessions: vi.fn(),
  getFlowPageSessions: vi.fn(),
  deleteRecording: vi.fn(),
  getEnvironments: vi.fn().mockResolvedValue({ environments: [] }),
}))

vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return { ...actual, api: mockApi }
})

function recording(over: Partial<Recording>): Recording {
  return {
    id: 'r1',
    project_id: 'p1',
    session_id: 'abcdefghijklmnopqrstuvwxyz',
    environment: 'production',
    first_chunk_index: 0,
    last_chunk_index: 0,
    chunk_count: 1,
    has_snapshot: true,
    duration_ms: 12_000,
    started_at: '2026-11-01T10:00:00Z',
    device_type: 'desktop',
    is_bot: false,
    ...over,
  }
}

function renderSessions() {
  return render(
    <MemoryRouter initialEntries={['/projects/p1/sessions']}>
      <Routes>
        <Route path="/projects/:projectId/sessions" element={<Sessions />} />
      </Routes>
    </MemoryRouter>,
  )
}

function setWidth(px: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: px })
}

describe('Sessions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setWidth(1024)
  })
  afterEach(() => cleanup())

  describe('page url in the list', () => {
    // A recording's page_url comes from the browser that sent it, so a relative
    // or truncated value is possible. It used to throw inside the render map and
    // take the whole page down.
    it.each([
      ['an absolute url', 'https://example.com/pricing?x=1', '/pricing'],
      ['a relative path', '/relative', '/relative'],
      ['a truncated url', 'https://exa', '/'],
      ['garbage', 'not a url at all', 'not a url at all'],
    ])('shows a path for %s on desktop', async (_name, pageUrl, shown) => {
      mockApi.listRecordings.mockResolvedValue({ recordings: [recording({ page_url: pageUrl })] })
      renderSessions()
      expect(await screen.findByText(shown)).toBeInTheDocument()
    })

    it('shows a path for a relative page_url on mobile', async () => {
      setWidth(400)
      mockApi.listRecordings.mockResolvedValue({ recordings: [recording({ page_url: '/relative' })] })
      renderSessions()
      expect(await screen.findByText('/relative')).toBeInTheDocument()
    })

    it('keeps listing the other recordings when one has a bad page_url', async () => {
      mockApi.listRecordings.mockResolvedValue({
        recordings: [
          recording({ id: 'r1', page_url: '/relative' }),
          recording({ id: 'r2', session_id: 'zzzzzzzzzzzzzzzz', page_url: 'https://example.com/ok' }),
        ],
      })
      renderSessions()
      await waitFor(() => {
        expect(screen.getByText('/relative')).toBeInTheDocument()
        expect(screen.getByText('/ok')).toBeInTheDocument()
      })
    })

    it('falls back to the session id when there is no page_url', async () => {
      mockApi.listRecordings.mockResolvedValue({ recordings: [recording({})] })
      renderSessions()
      expect(await screen.findByText('abcdefghijkl…')).toBeInTheDocument()
    })
  })
  describe('loading', () => {
    function deferred<T>() {
      let resolve!: (v: T) => void
      let reject!: (e: unknown) => void
      const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
      return { promise, resolve, reject }
    }

    // Two loads in flight, the newer one answers first. The older answer
    // used to land last and replace the list the filters on screen asked for.
    it('keeps the newest load when an older one answers last', async () => {
      const first = deferred<{ recordings: Recording[] }>()
      const second = deferred<{ recordings: Recording[] }>()
      mockApi.listRecordings.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
      renderSessions()
      await waitFor(() => expect(mockApi.listRecordings).toHaveBeenCalledTimes(1))

      fireEvent.click(screen.getByRole('button', { name: 'Mobile' }))
      await waitFor(() => expect(mockApi.listRecordings).toHaveBeenCalledTimes(2))

      await act(async () => { second.resolve({ recordings: [recording({ id: 'new', page_url: '/newest' })] }) })
      expect(await screen.findByText('/newest')).toBeInTheDocument()

      await act(async () => { first.resolve({ recordings: [recording({ id: 'old', page_url: '/stale' })] }) })
      expect(screen.getByText('/newest')).toBeInTheDocument()
      expect(screen.queryByText('/stale')).not.toBeInTheDocument()
    })

    it('does not let an older failure replace a newer list', async () => {
      const first = deferred<{ recordings: Recording[] }>()
      const second = deferred<{ recordings: Recording[] }>()
      mockApi.listRecordings.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
      renderSessions()
      await waitFor(() => expect(mockApi.listRecordings).toHaveBeenCalledTimes(1))
      fireEvent.click(screen.getByRole('button', { name: 'Mobile' }))
      await waitFor(() => expect(mockApi.listRecordings).toHaveBeenCalledTimes(2))

      await act(async () => { second.resolve({ recordings: [recording({ page_url: '/newest' })] }) })
      await act(async () => { first.reject(new Error('late failure')) })

      expect(screen.getByText('/newest')).toBeInTheDocument()
      expect(mockReportError).not.toHaveBeenCalled()
    })

    // A failed load used to read as an empty project and was never reported.
    it('reports a failed load and says so instead of "No recordings found"', async () => {
      const boom = new Error('listRecordings failed')
      mockApi.listRecordings.mockRejectedValue(boom)
      renderSessions()

      expect(await screen.findByText(/could not load recordings/i)).toBeInTheDocument()
      expect(screen.queryByText(/no recordings found/i)).not.toBeInTheDocument()
      expect(mockReportError).toHaveBeenCalledWith(boom, { source: 'Sessions.load' })
    })

    it('shows "No recordings found" for a project that has none', async () => {
      mockApi.listRecordings.mockResolvedValue({ recordings: [] })
      renderSessions()
      expect(await screen.findByText(/no recordings found/i)).toBeInTheDocument()
      expect(mockReportError).not.toHaveBeenCalled()
    })

    it('clears the failure after the next load succeeds', async () => {
      mockApi.listRecordings.mockRejectedValueOnce(new Error('x'))
      renderSessions()
      await screen.findByText(/could not load recordings/i)

      mockApi.listRecordings.mockResolvedValueOnce({ recordings: [recording({ page_url: '/back' })] })
      fireEvent.click(screen.getByRole('button', { name: 'Mobile' }))
      expect(await screen.findByText('/back')).toBeInTheDocument()
      expect(screen.queryByText(/could not load recordings/i)).not.toBeInTheDocument()
    })
  })
})
