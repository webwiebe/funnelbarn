import { api, type Recording } from '../../lib/api'

export const PAGE_SIZE = 50

// The filters and the funnel or flow selection the Sessions page lists by.
export interface RecordingQuery {
  traffic: 'all' | 'human' | 'bot'
  device: string
  pageUrl: string
  environment: string
}

export interface RecordingSelection {
  funnelId: string | null
  step: string | null
  page: string | null
}

// loadRecordingPage fetches one page of recordings. A funnel step or a flow
// page narrows it to the sessions that reached it first.
export async function loadRecordingPage(
  projectId: string,
  offset: number,
  query: RecordingQuery,
  selection: RecordingSelection,
): Promise<Recording[]> {
  const sessionIds = await selectedSessionIds(projectId, query, selection)
  const result = await api.listRecordings(projectId, {
    limit: PAGE_SIZE,
    offset,
    session_ids: sessionIds,
    device_type: query.device !== 'all' ? query.device : undefined,
    human_only: query.traffic === 'human',
    environment: query.environment || undefined,
    page_url: query.pageUrl || undefined,
  })
  return result.recordings
}

async function selectedSessionIds(
  projectId: string,
  query: RecordingQuery,
  { funnelId, step, page }: RecordingSelection,
): Promise<string[] | undefined> {
  if (funnelId && step) {
    const n = parseInt(step, 10)
    if (isNaN(n)) return undefined
    const r = await api.getFunnelStepSessions(projectId, funnelId, n, { environment: query.environment || undefined })
    return r.session_ids
  }
  if (page) {
    const r = await api.getFlowPageSessions(projectId, page)
    return r.session_ids
  }
  return undefined
}

// recordingPath is the path of a recording's page_url. The value comes from the
// visitor's browser, so a relative or malformed one is shown as sent instead of
// throwing during render.
export function recordingPath(pageUrl: string): string {
  try {
    return new URL(pageUrl).pathname
  } catch {
    return pageUrl
  }
}
