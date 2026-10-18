import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import FlagUsage from './FlagUsage'
import type { FlagUsageReport } from '../lib/api.flagusage'

// The page chrome has its own tests. Rendering it here put the first test's
// cold render past the 5 s budget on a loaded CI host.
vi.mock('../components/shell/Shell', () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('../lib/auth', () => ({
  useAuth: () => ({ user: { id: 'u1', username: 'admin' }, isLoading: false, logout: vi.fn() }),
}))

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

const mockUsage = vi.hoisted(() => ({ getFlagUsage: vi.fn() }))
vi.mock('../lib/api.flagusage', () => mockUsage)

vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return {
    ...actual,
    api: { ...actual.api, getEnvironments: vi.fn().mockResolvedValue({ environments: [] }) },
  }
})

const report: FlagUsageReport = {
  window_days: 30,
  counts_note: 'Counts are recorded evaluations.',
  stale_note: 'Stale means no evaluations in the last 30 days.',
  flags: [
    {
      flag_id: 'a', flag_key: 'checkout', name: 'Checkout', kind: 'experiment', status: 'active',
      evaluations_7d: 4, evaluations_30d: 9, daily: new Array(30).fill(0), variants_30d: { on: 9 },
      last_evaluated_at: '2026-10-15T08:00:00Z', stale: false,
    },
    {
      flag_id: 'b', flag_key: 'legacy', name: 'Legacy', kind: 'experiment', status: 'active',
      evaluations_7d: 0, evaluations_30d: 0, daily: new Array(30).fill(0), variants_30d: {}, stale: true,
    },
  ],
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/flags/p1/usage']}>
      <Routes>
        <Route path="/flags/:projectId/usage" element={<FlagUsage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('FlagUsage page', () => {
  beforeEach(() => { mockUsage.getFlagUsage.mockReset() })

  it('shows the notes, the rows and the stale badge', async () => {
    mockUsage.getFlagUsage.mockResolvedValue(report)
    renderPage()
    expect(await screen.findByTestId('usage-row-checkout')).toBeTruthy()
    expect(mockUsage.getFlagUsage).toHaveBeenCalledWith('p1')
    expect(screen.getByText('Counts are recorded evaluations.')).toBeTruthy()
    expect(screen.getByText('Stale means no evaluations in the last 30 days.')).toBeTruthy()
    expect(screen.getAllByText('Stale: no evaluations in 30 d')).toHaveLength(1)
  })

  it('shows an error message when the report cannot be loaded', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mockUsage.getFlagUsage.mockRejectedValue(new Error('boom'))
    renderPage()
    await waitFor(() => expect(screen.getByText('Could not load flag usage.')).toBeTruthy())
  })
})
