import { Sparkles, CheckCircle } from 'lucide-react'
import { Project, ProjectHealth } from '../../lib/api'
import { CopyButton } from '../ui/CopyButton'
import { FEATURES, buildAgentPrompt } from './features'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  success: '#10b981',
}

interface AgentInstructionsProps {
  health: ProjectHealth | null
  project: Project | null
}

export function AgentInstructions({ health, project }: AgentInstructionsProps) {
  const allDone =
    health !== null && FEATURES.every((f) => health[f.key] as boolean)

  const prompt = buildAgentPrompt(health, project, window.location.origin)

  return (
    <div style={{
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
      overflow: 'hidden',
      marginBottom: '2rem',
    }}>
      {/* Header */}
      <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Sparkles size={16} color={C.amber} />
          <div style={{ fontWeight: 700, fontSize: 15 }}>Agent instructions</div>
        </div>
        <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
          Copy this prompt into an LLM coding agent (Claude Code, Cursor, …) to finish the
          steps this project hasn't completed yet.
        </div>
      </div>

      <div style={{ padding: '1.25rem 1.5rem' }}>
        {allDone ? (
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            background: 'rgba(16,185,129,0.08)',
            border: `1px solid rgba(16,185,129,0.25)`,
            borderRadius: 10,
            padding: '1rem',
            color: C.success,
            fontSize: 14,
            fontWeight: 600,
          }}>
            <CheckCircle size={18} />
            All capabilities integrated — nothing left to hand to an agent.
          </div>
        ) : (
          <>
            <div style={{
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 10,
              padding: '1rem',
              marginBottom: '0.75rem',
              fontFamily: '"SF Mono", "Fira Code", monospace',
              fontSize: 12.5,
              lineHeight: 1.6,
              color: '#a5f3fc',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              maxHeight: 360,
              overflowY: 'auto',
            }}>
              {prompt}
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <CopyButton value={prompt} />
            </div>
          </>
        )}
      </div>
    </div>
  )
}
