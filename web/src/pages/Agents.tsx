import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import Shell from '../components/shell/Shell'
import { useProjects } from '../lib/projects'
import { reportError } from '../lib/bugbarn'
import { getAgentLeadTimes, getAgentScoreboard } from '../lib/api.agents'
import type { AgentLeadTimes, AgentScoreboard, DurationStats } from '../lib/api.agents'
import { Skeleton } from '../components/ui/Skeleton'
import { C } from '../components/flags/shared'
import { formatSeconds } from '../lib/duration'

const card = { background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, padding: '1.25rem' }
const th = { textAlign: 'left' as const, color: C.muted, fontSize: 12, padding: '0.4rem 0.5rem' }
const td = { padding: '0.4rem 0.5rem', fontSize: 14 }

function Stat({ label, stats, id }: { label: string; stats: DurationStats; id: string }) {
  return (
    <div style={{ ...card, flex: '1 1 200px' }} data-testid={`stat-${id}`}>
      <div style={{ color: C.muted, fontSize: 12 }}>{label}</div>
      {stats.count === 0 ? (
        <div style={{ color: C.muted, marginTop: 6 }}>No data in range</div>
      ) : (
        <>
          <div style={{ fontSize: 22, fontWeight: 700, marginTop: 4 }}>{formatSeconds(stats.median_seconds)}</div>
          <div style={{ color: C.muted, fontSize: 12 }}>
            median, p90 {formatSeconds(stats.p90_seconds)}, {stats.count} items
          </div>
        </>
      )}
    </div>
  )
}

export default function Agents() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const [lead, setLead] = useState<AgentLeadTimes | null>(null)
  const [board, setBoard] = useState<AgentScoreboard | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (!projectId) return
    setLead(null)
    setBoard(null)
    setFailed(false)
    Promise.all([getAgentLeadTimes(projectId), getAgentScoreboard(projectId)])
      .then(([l, b]) => { setLead(l); setBoard(b) })
      .catch((e) => { console.error(e); setFailed(true); reportError(e, { source: 'Agents' }) })
  }, [projectId])

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, padding: '2rem', textAlign: 'center' }}>Select a project to see agent delivery.</div>
      </Shell>
    )
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <h1 style={{ fontSize: 20, margin: '0 0 1rem' }}>Agents</h1>
      {failed ? (
        <div style={{ ...card, color: C.error }}>Could not load the agent report.</div>
      ) : !lead || !board ? (
        <Skeleton height={240} />
      ) : (
        <div style={{ display: 'grid', gap: '1rem' }}>
          <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
            <Stat id="issue-to-prod" label="Issue to production" stats={lead.issue_to_prod} />
            <Stat id="pr-to-merge" label="PR to merge" stats={lead.pr_to_merge} />
            <Stat id="owner-wait" label="Owner wait" stats={lead.owner_wait} />
          </div>

          <div style={card}>
            <h2 style={{ fontSize: 15, margin: '0 0 0.5rem' }}>First-pass CI per repository</h2>
            {(lead.first_pass_ci ?? []).length === 0 ? (
              <div style={{ color: C.muted }}>No pull requests in range.</div>
            ) : (
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead><tr><th style={th}>Repository</th><th style={th}>PRs</th><th style={th}>First pass green</th><th style={th}>Rate</th></tr></thead>
                <tbody>
                  {(lead.first_pass_ci ?? []).map((r) => (
                    <tr key={r.repo} data-testid={`ci-${r.repo}`}>
                      <td style={td}>{r.repo}</td><td style={td}>{r.prs}</td>
                      <td style={td}>{r.first_pass_green}</td><td style={td}>{Math.round(r.rate * 100)}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {(lead.open_waits ?? []).length > 0 && (
            <div style={card}>
              <h2 style={{ fontSize: 15, margin: '0 0 0.5rem' }}>Waiting on the owner</h2>
              <ul style={{ margin: 0, paddingLeft: '1.2rem' }}>
                {(lead.open_waits ?? []).map((w) => (
                  <li key={w.issue_ref} style={td}>{w.issue_ref}: {formatSeconds(w.waiting_seconds)}</li>
                ))}
              </ul>
            </div>
          )}

          <div style={card}>
            <h2 style={{ fontSize: 15, margin: '0 0 0.5rem' }}>Scoreboard</h2>
            {(board.sessions ?? []).length === 0 ? (
              <div style={{ color: C.muted }}>No scored sessions this week.</div>
            ) : (
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead><tr><th style={th}>Session</th><th style={th}>Points</th><th style={th}>Entries</th><th style={th}>Streak (best)</th></tr></thead>
                <tbody>
                  {(board.sessions ?? []).map((s) => (
                    <tr key={s.session_id} data-testid={`session-${s.session_id}`}>
                      <td style={td}>{s.session_id}</td><td style={td}>{s.points}</td>
                      <td style={td}>{s.entries}</td>
                      <td style={td}>{s.first_pass_streak} ({s.best_first_pass_streak})</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            {board.unattributed_points !== 0 && (
              <p style={{ color: C.muted, fontSize: 13 }}>Unattributed points: {board.unattributed_points}</p>
            )}
            {(board.unscored ?? []).map((u) => (
              <p key={u.rule} style={{ color: C.muted, fontSize: 13, margin: '0.25rem 0 0' }}>{u.rule}: not scored yet. {u.reason}</p>
            ))}
          </div>
        </div>
      )}
    </Shell>
  )
}
