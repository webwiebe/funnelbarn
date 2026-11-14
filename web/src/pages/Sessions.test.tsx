import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
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
})
