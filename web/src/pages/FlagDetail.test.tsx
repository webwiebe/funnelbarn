import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { FlagDetail } from './Flags'
import type { FeatureFlag, FlagAnalysis } from '../lib/api'

const mockApi = vi.hoisted(() => ({
  getFlagAnalysis: vi.fn(),
  getEnvironments: vi.fn().mockResolvedValue({ environments: [] }),
}))
vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return { ...actual, api: mockApi }
})
vi.mock('../lib/bugbarn', () => ({ reportError: vi.fn() }))

function makeFlag(id: string, name: string): FeatureFlag {
  return {
    id, project_id: 'p1', flag_key: id, name, flag_type: 'boolean',
    variants: '{"on":true,"off":false}', default_variant: 'off', split: '{"on":0,"off":100}',
    conversion_event: 'signup', targeting_rules: '[]', status: 'active', created_at: '',
  } as FeatureFlag
}

function analysisFor(flag: FeatureFlag, variant: string): FlagAnalysis {
  return { flag, results: [{ variant, sample: 10, conversions: 1, rate: 0.1 }] }
}

const flagA = makeFlag('fa', 'Flag A')
const flagB = makeFlag('fb', 'Flag B')

function view(flag: FeatureFlag) {
  return (
    <MemoryRouter>
      <FlagDetail flag={flag} projectId="p1" onUpdated={vi.fn()} onDeleted={vi.fn()} />
    </MemoryRouter>
  )
}

describe('FlagDetail analysis', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('does not keep the previous flag analysis when the next request fails', async () => {
    mockApi.getFlagAnalysis
      .mockResolvedValueOnce(analysisFor(flagA, 'variant-of-a'))
      .mockRejectedValueOnce(new Error('boom'))
    const { rerender } = render(view(flagA))
    expect(await screen.findByText('variant-of-a')).toBeInTheDocument()

    rerender(view(flagB))
    await waitFor(() => expect(mockApi.getFlagAnalysis).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.queryByText('variant-of-a')).not.toBeInTheDocument())
  })

  it('ignores a late response for the flag that was shown before', async () => {
    let resolveA: (a: FlagAnalysis) => void = () => {}
    mockApi.getFlagAnalysis
      .mockImplementationOnce(() => new Promise<FlagAnalysis>((res) => { resolveA = res }))
      .mockResolvedValueOnce(analysisFor(flagB, 'variant-of-b'))
    const { rerender } = render(view(flagA))
    await waitFor(() => expect(mockApi.getFlagAnalysis).toHaveBeenCalledTimes(1))
    rerender(view(flagB))
    expect(await screen.findByText('variant-of-b')).toBeInTheDocument()

    await act(async () => { resolveA(analysisFor(flagA, 'variant-of-a')) })
    expect(screen.queryByText('variant-of-a')).not.toBeInTheDocument()
    expect(screen.getByText('variant-of-b')).toBeInTheDocument()
  })
})
