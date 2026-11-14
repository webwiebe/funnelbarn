import { describe, it, expect, vi, beforeEach } from 'vitest'
import { loadRecordingPage, PAGE_SIZE, type RecordingQuery } from './loadRecordings'

const mockApi = vi.hoisted(() => ({
  listRecordings: vi.fn(),
  getFunnelStepSessions: vi.fn(),
  getFlowPageSessions: vi.fn(),
}))

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return { ...actual, api: mockApi }
})

const query: RecordingQuery = { traffic: 'human', device: 'mobile', pageUrl: '/p', environment: 'staging' }
const none = { funnelId: null, step: null, page: null }

describe('loadRecordingPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.listRecordings.mockResolvedValue({ recordings: [] })
  })

  it('passes the filters to the listing', async () => {
    await loadRecordingPage('p1', 100, query, none)
    expect(mockApi.listRecordings).toHaveBeenCalledWith('p1', {
      limit: PAGE_SIZE,
      offset: 100,
      session_ids: undefined,
      device_type: 'mobile',
      human_only: true,
      environment: 'staging',
      page_url: '/p',
    })
  })

  it('leaves out the neutral filters', async () => {
    await loadRecordingPage('p1', 0, { traffic: 'all', device: 'all', pageUrl: '', environment: '' }, none)
    const params = mockApi.listRecordings.mock.calls[0][1]
    expect(params).toMatchObject({ device_type: undefined, human_only: false, environment: undefined, page_url: undefined })
  })

  it('narrows to the sessions of a funnel step in the selected environment', async () => {
    mockApi.getFunnelStepSessions.mockResolvedValue({ session_ids: ['s1', 's2'] })
    await loadRecordingPage('p1', 0, query, { funnelId: 'f1', step: '2', page: null })
    expect(mockApi.getFunnelStepSessions).toHaveBeenCalledWith('p1', 'f1', 2, { environment: 'staging' })
    expect(mockApi.listRecordings.mock.calls[0][1].session_ids).toEqual(['s1', 's2'])
  })

  it('ignores a funnel step that is not a number, and a flow page then does not apply', async () => {
    await loadRecordingPage('p1', 0, query, { funnelId: 'f1', step: 'x', page: '/home' })
    expect(mockApi.getFunnelStepSessions).not.toHaveBeenCalled()
    expect(mockApi.getFlowPageSessions).not.toHaveBeenCalled()
    expect(mockApi.listRecordings.mock.calls[0][1].session_ids).toBeUndefined()
  })

  it('narrows to the sessions of a flow page', async () => {
    mockApi.getFlowPageSessions.mockResolvedValue({ session_ids: ['s9'] })
    await loadRecordingPage('p1', 0, query, { funnelId: null, step: null, page: '/home' })
    expect(mockApi.getFlowPageSessions).toHaveBeenCalledWith('p1', '/home')
    expect(mockApi.listRecordings.mock.calls[0][1].session_ids).toEqual(['s9'])
  })
})
