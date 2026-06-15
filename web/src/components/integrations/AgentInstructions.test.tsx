import { describe, it, expect } from 'vitest'
import { buildAgentPrompt, FEATURES } from './features'
import { Project, ProjectHealth } from '../../lib/api'

const project: Project = { id: 'p1', name: 'Acme', slug: 'acme', status: 'active' }

function health(overrides: Partial<ProjectHealth>): ProjectHealth {
  return {
    project_id: 'p1',
    setup_called: false,
    events_received: false,
    flags_evaluated: false,
    recordings_received: false,
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

const ORIGIN = 'https://funnelbarn.example.com'

describe('buildAgentPrompt', () => {
  it('includes the setup URL with the project slug', () => {
    const prompt = buildAgentPrompt(health({}), project, ORIGIN)
    expect(prompt).toContain(`${ORIGIN}/api/v1/setup/acme`)
  })

  it('lists only the missing capabilities as numbered tasks', () => {
    // setup + events done, flags + recordings missing.
    const prompt = buildAgentPrompt(
      health({ setup_called: true, events_received: true }),
      project,
      ORIGIN,
    )

    const flagsTask = FEATURES.find((f) => f.key === 'flags_evaluated')!.agentTask
    const recordingsTask = FEATURES.find((f) => f.key === 'recordings_received')!.agentTask
    const eventsTask = FEATURES.find((f) => f.key === 'events_received')!.agentTask

    expect(prompt).toContain(`1. ${flagsTask}`)
    expect(prompt).toContain(`2. ${recordingsTask}`)
    // Completed capabilities must not appear as a to-do.
    expect(prompt).not.toContain(eventsTask)
  })

  it('shows a ✅/❌ status line for every capability', () => {
    const prompt = buildAgentPrompt(
      health({ setup_called: true }),
      project,
      ORIGIN,
    )
    expect(prompt).toContain('✅ Setup guide fetched')
    expect(prompt).toContain('❌ Events received')
  })

  it('falls back to a placeholder slug when no project is selected', () => {
    const prompt = buildAgentPrompt(null, null, ORIGIN)
    expect(prompt).toContain(`${ORIGIN}/api/v1/setup/your-project`)
  })
})
