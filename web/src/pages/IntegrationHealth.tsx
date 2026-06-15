import { useState } from 'react'
import Shell from '../components/shell/Shell'
import { useProjects } from '../lib/projects'
import { Project, ProjectHealth } from '../lib/api'
import { ProjectHealthCard } from '../components/settings/ProjectHealthCard'
import { AgentInstructions } from '../components/integrations/AgentInstructions'

const C = {
  muted: '#94a3b8',
  text: '#e2e8f0',
}

export default function IntegrationHealth() {
  const { projects, defaultProjectId } = useProjects()
  const [health, setHealth] = useState<ProjectHealth | null>(null)
  const [project, setProject] = useState<Project | null>(null)

  return (
    <Shell>
      <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', marginBottom: '0.5rem' }}>
        Integration Health
      </h1>
      <p style={{ fontSize: 14, color: C.muted, marginBottom: '2rem', maxWidth: 680, lineHeight: 1.6 }}>
        FunnelBarn auto-detects which parts of the integration each project has actually exercised.
        Hand the generated prompt below to an LLM coding agent to finish whatever's still missing.
      </p>

      {projects.length === 0 ? (
        <div style={{ fontSize: 14, color: C.text }}>
          Create a project first — integration health is tracked per project.
        </div>
      ) : (
        <>
          <ProjectHealthCard
            projects={projects}
            defaultProjectId={defaultProjectId}
            onHealthChange={(h, p) => { setHealth(h); setProject(p) }}
          />
          <AgentInstructions health={health} project={project} />
        </>
      )}
    </Shell>
  )
}
