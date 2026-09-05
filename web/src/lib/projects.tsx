import { createContext, useContext, useEffect, useState, ReactNode, useCallback } from 'react'
import { api, Project } from './api'
import { reportError } from './bugbarn'
import { readLastProjectId, resolveProjectId, subscribeLastProjectId, writeLastProjectId } from './selectedProject'

const STORAGE_KEY = 'funnelbarn_default_project'
const ENV_STORAGE_KEY = 'funnelbarn_environment'

interface ProjectContextValue {
  projects: Project[]
  isLoading: boolean
  refetch: () => void
  defaultProjectId: string | null
  setDefaultProjectId: (id: string) => void
  /** The project last chosen in the header picker — see lib/selectedProject.ts. */
  selectedProjectId: string | null
  setSelectedProjectId: (id: string) => void
  selectedEnvironment: string
  setSelectedEnvironment: (env: string) => void
}

const ProjectContext = createContext<ProjectContextValue | null>(null)

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [projects, setProjects] = useState<Project[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [defaultProjectId, setDefaultProjectIdState] = useState<string | null>(
    () => localStorage.getItem(STORAGE_KEY)
  )
  const [selectedProjectId, setSelectedProjectIdState] = useState<string | null>(
    () => readLastProjectId() ?? null
  )
  const [selectedEnvironment, setSelectedEnvironmentState] = useState<string>(
    () => localStorage.getItem(ENV_STORAGE_KEY) ?? ''
  )

  const setDefaultProjectId = useCallback((id: string) => {
    setDefaultProjectIdState(id)
    localStorage.setItem(STORAGE_KEY, id)
  }, [])

  const setSelectedProjectId = useCallback((id: string) => {
    setSelectedProjectIdState(id)
    writeLastProjectId(id)
  }, [])

  const setSelectedEnvironment = useCallback((env: string) => {
    setSelectedEnvironmentState(env)
    if (env) {
      localStorage.setItem(ENV_STORAGE_KEY, env)
    } else {
      localStorage.removeItem(ENV_STORAGE_KEY)
    }
  }, [])

  const refetch = useCallback(() => {
    setIsLoading(true)
    api.listProjects()
      .then((d) => setProjects(d.projects || []))
      .catch((e) => {
        // Expired sessions and network blips are filtered inside reportError.
        reportError(e, { source: 'ProjectProvider.listProjects' })
        setProjects([])
      })
      .finally(() => setIsLoading(false))
  }, [])

  useEffect(() => { refetch() }, [refetch])

  // The header picker writes the selection directly, so mirror it back into
  // state — otherwise a page reading selectedProjectId would keep acting on the
  // project the user just switched away from.
  useEffect(() => subscribeLastProjectId(setSelectedProjectIdState), [])

  return (
    <ProjectContext.Provider value={{ projects, isLoading, refetch, defaultProjectId, setDefaultProjectId, selectedProjectId, setSelectedProjectId, selectedEnvironment, setSelectedEnvironment }}>
      {children}
    </ProjectContext.Provider>
  )
}

export function useProjects(): ProjectContextValue {
  const ctx = useContext(ProjectContext)
  if (!ctx) throw new Error('useProjects must be used inside ProjectProvider')
  return ctx
}

/** The project a page should act on — see resolveProjectId for the order. */
export function useEffectiveProjectId(urlProjectId?: string): string | undefined {
  const { projects, defaultProjectId, selectedProjectId } = useProjects()
  return resolveProjectId(projects, urlProjectId, selectedProjectId, defaultProjectId)
}
