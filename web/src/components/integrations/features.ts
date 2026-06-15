import { Project, ProjectHealth } from '../../lib/api'

// The four capabilities Integration Health tracks. Each maps to a boolean flag
// on ProjectHealth that flips to true the first time the implementing agent
// actually exercises that part of the FunnelBarn API.
//
// `description` is the wire-level endpoint shown in the checklist UI.
// `agentTask` is a one-line instruction handed to an integrating LLM agent,
// pointing it at the relevant section of the setup guide.
export interface HealthFeature {
  key: keyof ProjectHealth
  label: string
  description: string
  agentTask: string
}

export const FEATURES: HealthFeature[] = [
  {
    key: 'setup_called',
    label: 'Setup guide fetched',
    description: 'GET /api/v1/setup/{slug}',
    agentTask: 'Fetch the setup guide (the URL above) and use it as the source of truth for the API key, endpoint and headers.',
  },
  {
    key: 'events_received',
    label: 'Events received',
    description: 'POST /api/v1/events',
    agentTask: 'Send analytics events to POST /api/v1/events (page views + key user actions) — see the "Event body schema" and SDK sections of the setup guide.',
  },
  {
    key: 'flags_evaluated',
    label: 'Flag evaluations',
    description: 'POST /api/v1/evaluate',
    agentTask: 'Evaluate feature flags at runtime via POST /api/v1/evaluate, passing a `context.targeting_key` for sticky bucketing — see the "Feature Flags" section.',
  },
  {
    key: 'recordings_received',
    label: 'Session recordings',
    description: 'POST /api/v1/recordings/chunk',
    agentTask: 'Enable rrweb session recording (`recording: true` in the JS SDK, or `data-recording="true"` on the script tag) — see the "Session Recording" section.',
  },
]

/**
 * Build a concise, copy-pasteable prompt for an LLM coding agent that is
 * integrating FunnelBarn. It points the agent at the authoritative (public,
 * idempotent) setup guide and lists ONLY the capabilities not yet detected, so
 * the agent skips work that's already done.
 *
 * Pure + exported so it can be unit-tested without rendering.
 */
export function buildAgentPrompt(
  health: ProjectHealth | null,
  project: Project | null,
  origin: string,
): string {
  const slug = project?.slug ?? 'your-project'
  const setupURL = `${origin}/api/v1/setup/${slug}`
  const done = (key: HealthFeature['key']) =>
    health ? (health[key] as boolean) : false

  const missing = FEATURES.filter((f) => !done(f.key))

  const statusList = FEATURES.map(
    (f) => `${done(f.key) ? '✅' : '❌'} ${f.label}`,
  ).join('\n')

  const todoList = missing
    .map((f, i) => `${i + 1}. ${f.agentTask}`)
    .join('\n')

  return [
    `You are integrating FunnelBarn (analytics, feature flags and session recording) into this codebase.`,
    ``,
    `Authoritative setup guide — fetch this first and treat it as the source of truth. It is idempotent and includes the project's API key, the required request headers, and ready-to-paste SDK snippets for JavaScript/TypeScript, Python, Go and raw HTTP:`,
    `  ${setupURL}`,
    ``,
    `Integration status so far (auto-detected by FunnelBarn from real traffic):`,
    statusList,
    ``,
    `Complete ONLY the steps that are still missing:`,
    todoList,
    ``,
    `Notes:`,
    `- Wire each capability where it genuinely belongs in the app. Do NOT fabricate calls just to flip a check — the dashboard ticks each item automatically once real traffic arrives.`,
    `- Reuse the same API key and project across dev/staging/prod; tag events with an "environment" field so the dashboard can filter them.`,
  ].join('\n')
}
