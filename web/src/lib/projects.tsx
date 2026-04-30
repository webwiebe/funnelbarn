import { createContext, useContext, useEffect, useState, ReactNode, useCallback } from 'react'
import { api, Project } from './api'

interface ProjectContextValue {
  projects: Project[]
  isLoading: boolean
  refetch: () => void
}

const ProjectContext = createContext<ProjectContextValue | null>(null)

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [projects, setProjects] = useState<Project[]>([])
  const [isLoading, setIsLoading] = useState(true)

  const refetch = useCallback(() => {
    setIsLoading(true)
    api.listProjects()
      .then((d) => setProjects(d.projects || []))
      .catch(() => setProjects([]))
      .finally(() => setIsLoading(false))
  }, [])

  useEffect(() => { refetch() }, [refetch])

  return (
    <ProjectContext.Provider value={{ projects, isLoading, refetch }}>
      {children}
    </ProjectContext.Provider>
  )
}

export function useProjects(): ProjectContextValue {
  const ctx = useContext(ProjectContext)
  if (!ctx) throw new Error('useProjects must be used inside ProjectProvider')
  return ctx
}

const DEFAULT_PROJECT_KEY = 'funnelbarn_default_project'

export function useEffectiveProjectId(urlProjectId?: string): string | undefined {
  const { projects } = useProjects()

  if (urlProjectId) return urlProjectId

  const stored = localStorage.getItem(DEFAULT_PROJECT_KEY)
  if (stored && projects.some((p) => p.id === stored)) return stored

  return projects[0]?.id
}
