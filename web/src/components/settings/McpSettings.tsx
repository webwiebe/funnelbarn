import { useEffect, useState } from 'react'
import { api, Project } from '../../lib/api'
import { CopyButton } from '../ui/CopyButton'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
}

interface McpSettingsProps {
  projects: Project[]
  /** The project this section builds the config for, as named in the header. */
  projectId?: string
}

// buildMcpJSON returns the pretty-printed .mcp.json snippet for a project, so
// the tests can check its shape without re-parsing the rendered DOM.
export function buildMcpJSON(mcpURL: string, slug: string): string {
  return JSON.stringify(
    {
      mcpServers: {
        funnelbarn: {
          type: 'http',
          url: mcpURL,
          headers: { 'x-funnelbarn-project': slug },
        },
      },
    },
    null,
    2,
  )
}

// McpSettings shows the copy-paste config for connecting an AI assistant to
// this project over MCP. Hidden entirely when the instance has no OIDC
// configured, since sign-in happens through IAMBarn on first use and there is
// nothing to sign in to otherwise.
export function McpSettings({ projects, projectId }: McpSettingsProps) {
  const [oidcEnabled, setOidcEnabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    api.getClientConfig()
      .then((cfg) => { if (!cancelled) setOidcEnabled(cfg.oidc?.enabled === true) })
      .catch(() => { if (!cancelled) setOidcEnabled(false) })
    return () => { cancelled = true }
  }, [])

  if (!oidcEnabled) return null

  const project = projects.find((p) => p.id === projectId)
  const slug = project?.slug ?? 'your-project'
  const publicURL = window.location.origin
  const mcpURL = `${publicURL}/api/v1/mcp`
  const command = `claude mcp add --transport http funnelbarn ${mcpURL} --header "x-funnelbarn-project: ${slug}"`
  const mcpJSON = buildMcpJSON(mcpURL, slug)

  const codeBoxStyle: React.CSSProperties = {
    background: C.bg,
    border: `1px solid ${C.border}`,
    borderRadius: 10,
    padding: '1rem',
    marginBottom: '0.75rem',
    fontFamily: '"SF Mono", "Fira Code", monospace',
    fontSize: 13,
    color: '#a5f3fc',
    whiteSpace: 'pre-wrap',
    overflowX: 'auto',
    wordBreak: 'break-all',
  }

  return (
    <div style={{
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
      overflow: 'hidden',
      marginBottom: '2rem',
    }}>
      <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
        <div style={{ fontWeight: 700, fontSize: 15 }}>Connect an assistant</div>
        <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
          Give an AI assistant MCP access to{' '}
          {project
            ? <strong style={{ color: C.text, fontWeight: 600 }}>{project.name}</strong>
            : 'this project'}.
        </div>
      </div>

      <div style={{ padding: '1.25rem 1.5rem' }}>
        <div style={{ fontSize: 12, fontWeight: 600, color: C.muted, marginBottom: 6 }}>
          Command line
        </div>
        <div style={codeBoxStyle}>{command}</div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '1rem' }}>
          <CopyButton value={command} />
        </div>

        <div style={{ fontSize: 12, fontWeight: 600, color: C.muted, marginBottom: 6 }}>
          .mcp.json
        </div>
        <div style={codeBoxStyle} data-testid="mcp-json-snippet">{mcpJSON}</div>
        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <CopyButton value={mcpJSON} />
        </div>

        <div style={{ marginTop: '1rem', paddingTop: '0.75rem', borderTop: `1px solid ${C.border}`, fontSize: 12, color: C.muted, lineHeight: 1.6 }}>
          <div>
            This config holds no secret. Sign-in happens in the browser through IAMBarn the first time the assistant connects.
          </div>
          <div style={{ marginTop: 6 }}>
            <code style={{ fontFamily: '"SF Mono", "Fira Code", monospace', color: C.amber }}>mcp:read</code>{' '}
            reads stats, events, funnels, segments and flags.{' '}
            <code style={{ fontFamily: '"SF Mono", "Fira Code", monospace', color: C.amber }}>mcp:write</code>{' '}
            also creates, updates and deletes them.
          </div>
          <div style={{ marginTop: 6 }}>
            The assistant's <code style={{ fontFamily: '"SF Mono", "Fira Code", monospace', color: C.amber }}>project</code>{' '}
            argument overrides the default project set by the header above.
          </div>
        </div>
      </div>
    </div>
  )
}
