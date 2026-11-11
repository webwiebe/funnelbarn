import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Agents from './Agents'
import { formatSeconds } from '../lib/duration'

vi.mock('../components/shell/Shell', () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))
vi.mock('../lib/projects', () => ({
  useProjects: () => ({ projects: [{ id: 'p1', name: 'Agents', slug: 'agents', status: 'active' }] }),
}))
const mockApi = vi.hoisted(() => ({ getAgentLeadTimes: vi.fn(), getAgentScoreboard: vi.fn() }))
vi.mock('../lib/api.agents', () => mockApi)

const stats = { count: 3, median_seconds: 7200, p90_seconds: 90000 }
const lead = {
  from: '2026-10-12T00:00:00Z', to: '2026-11-11T00:00:00Z',
  issue_to_prod: stats, pr_to_merge: { count: 0, median_seconds: 0, p90_seconds: 0 }, owner_wait: stats,
  weekly: [{ week: '2026-W45', count: 3, median_seconds: 7200, p90_seconds: 90000 }],
  shipped_per_day: [{ date: '2026-11-09', count: 2 }, { date: '2026-11-10', count: 1 }],
  first_pass_ci: [{ repo: 'funnelbarn', prs: 4, first_pass_green: 3, rate: 0.75 }],
  open_waits: [{ issue_ref: 'vibecortex#134', since: '2026-11-10T00:00:00Z', waiting_seconds: 3600 }],
}
const board = {
  from: lead.from, to: lead.to,
  sessions: [{ session_id: 's1', points: 12, entries: 3, by_rule: {}, first_pass_streak: 2, best_first_pass_streak: 4 }],
  unattributed_points: 0,
  unscored: [{ rule: 'rollback', reason: 'No rollback events yet.' }],
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/agents/p1']}>
      <Routes><Route path="/agents/:projectId" element={<Agents />} /></Routes>
    </MemoryRouter>,
  )
}

describe('Agents page', () => {
  beforeEach(() => { mockApi.getAgentLeadTimes.mockReset(); mockApi.getAgentScoreboard.mockReset() })

  it('formats durations', () => {
    expect(formatSeconds(45)).toBe('45 s')
    expect(formatSeconds(7200)).toBe('2 h 0 min')
    expect(formatSeconds(90000)).toBe('1 d 1 h')
  })

  it('shows lead times, CI rate, waits and the scoreboard', async () => {
    mockApi.getAgentLeadTimes.mockResolvedValue(lead)
    mockApi.getAgentScoreboard.mockResolvedValue(board)
    renderPage()
    expect(await screen.findByTestId('session-s1')).toBeTruthy()
    expect(screen.getByTestId('stat-issue-to-prod').textContent).toContain('2 h 0 min')
    expect(screen.getByTestId('stat-pr-to-merge').textContent).toContain('No data in range')
    expect(screen.getByTestId('ci-funnelbarn').textContent).toContain('75%')
    expect(screen.getByText(/vibecortex#134/)).toBeTruthy()
    expect(screen.getByText(/No rollback events yet/)).toBeTruthy()
    expect(screen.getByTestId('week-2026-W45').textContent).toContain('1 d 1 h')
    expect(screen.getByTestId('shipped-per-day').children).toHaveLength(2)
    expect(screen.getByText(/3 issues from 2026-11-09 to 2026-11-10/)).toBeTruthy()
    expect(mockApi.getAgentLeadTimes).toHaveBeenCalledWith('p1')
  })

  it('shows an error when the report cannot be loaded', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mockApi.getAgentLeadTimes.mockRejectedValue(new Error('boom'))
    mockApi.getAgentScoreboard.mockResolvedValue(board)
    renderPage()
    await waitFor(() => expect(screen.getByText('Could not load the agent report.')).toBeTruthy())
  })
})
