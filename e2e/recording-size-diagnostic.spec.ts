/**
 * Temporary read for the replay failure on main run 42138674519: logs the
 * recorded viewport of the newest playable recordings in the shared testing
 * project, as numbers only. It asserts nothing beyond the reads succeeding.
 * Remove it once the cause is recorded on the follow-up issue.
 */
import { test, expect } from '@playwright/test'

const PROJECT_ID = 'e7af9de5-04d3-4e00-a552-5fa49baca192'

test('log the recorded size of the newest shared recordings', async ({ page }) => {
  const login = await page.request.post('/api/v1/login', {
    data: { username: process.env.E2E_USERNAME, password: process.env.E2E_PASSWORD },
  })
  expect(login.ok(), `Login failed: ${login.status()}`).toBeTruthy()

  const list = await page.request.get(`/api/v1/projects/${PROJECT_ID}/recordings?human_only=true&limit=5`)
  expect(list.ok()).toBeTruthy()
  const { recordings = [] } = await list.json()
  for (const [i, rec] of recordings.entries()) {
    if (!rec.has_snapshot) {
      console.log(`recording-size row=${i} has_snapshot=false`)
      continue
    }
    const chunk = await page.request.get(
      `/api/v1/projects/${PROJECT_ID}/recordings/${rec.id}/chunks/${rec.first_chunk_index}`,
    )
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const events: any[] = chunk.ok() ? await chunk.json() : []
    const meta = events.find((e) => e?.type === 4)
    console.log(
      `recording-size row=${i} chunk_status=${chunk.status()} events=${events.length}` +
        ` meta=${meta ? 1 : 0} width=${meta?.data?.width} height=${meta?.data?.height}` +
        ` duration_ms=${rec.duration_ms} chunk_count=${rec.chunk_count} device=${rec.device_type}`,
    )
  }
})
