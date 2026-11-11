// Agent delivery report: lead times and the per-session scoreboard.
// Mirrors service.AgentLeadTimes and service.AgentScoreboard.
import { request } from './api'

export interface DurationStats {
  count: number
  median_seconds: number
  p90_seconds: number
}

export interface AgentWeekStats extends DurationStats {
  week: string
}

export interface AgentRepoCI {
  repo: string
  prs: number
  first_pass_green: number
  rate: number
}

export interface AgentOpenWait {
  issue_ref: string
  since: string
  waiting_seconds: number
}

export interface AgentLeadTimes {
  from: string
  to: string
  repo?: string
  issue_to_prod: DurationStats
  pr_to_merge: DurationStats
  owner_wait: DurationStats
  weekly: AgentWeekStats[] | null
  first_pass_ci: AgentRepoCI[] | null
  open_waits: AgentOpenWait[] | null
}

export interface AgentSessionScore {
  session_id: string
  points: number
  entries: number
  by_rule: Record<string, number>
  first_pass_streak: number
  best_first_pass_streak: number
}

export interface AgentUnscoredRule {
  rule: string
  reason: string
}

export interface AgentScoreboard {
  from: string
  to: string
  sessions: AgentSessionScore[] | null
  unattributed_points: number
  unscored: AgentUnscoredRule[] | null
}

export const getAgentLeadTimes = (projectId: string) =>
  request<AgentLeadTimes>(`/api/v1/projects/${projectId}/agents/lead-times`)

export const getAgentScoreboard = (projectId: string) =>
  request<AgentScoreboard>(`/api/v1/projects/${projectId}/agents/scoreboard`)
