import { createContext, useContext, useEffect, useState, ReactNode, useCallback } from 'react'
import { api, ApiError, Project } from './api'
import { reportError } from './bugbarn'

const STORAGE_KEY = 'funnelbarn_default_project'
const ENV_STORAGE_KEY = 'funnelbarn_environment'

interface ProjectContextValue {
  projects: Project[]
  isLoading: boolean
  refetch: () => void
  defaultProjectId: string | null
  setDefaultProjectId: (id: string) => void
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
  const [selectedEnvironment, setSelectedEnvironmentState] = useState<string>(
    () => localStorage.getItem(ENV_STORAGE_KEY) ?? ''
  )

  const setDefaultProjectId = useCallback((id: string) => {
    setDefaultProjectIdState(id)
    localStorage.setItem(STORAGE_KEY, id)
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
        // Skip noise:
        // - 401: expected when session expired / before login. api.request already redirects.
        // - 0:   network failure (TypeError: Failed to fetch). User's network, not our code.
        const isNoise = e instanceof ApiError && (e.status === 401 || e.status === 0)
        if (!isNoise) {
          reportError(e, { source: 'ProjectProvider.listProjects' })
        }
        setProjects([])
      })
      .finally(() => setIsLoading(false))
  }, [])

  useEffect(() => { refetch() }, [refetch])

  return (
    <ProjectContext.Provider value={{ projects, isLoading, refetch, defaultProjectId, setDefaultProjectId, selectedEnvironment, setSelectedEnvironment }}>
      {children}
    </ProjectContext.Provider>
  )
}

export function useProjects(): ProjectContextValue {
  const ctx = useContext(ProjectContext)
  if (!ctx) throw new Error('useProjects must be used inside ProjectProvider')
  return ctx
}

export function useEffectiveProjectId(urlProjectId?: string): string | undefined {
  const { projects, defaultProjectId } = useProjects()
  if (urlProjectId) return urlProjectId
  const defaultExists = defaultProjectId && projects.some((p) => p.id === defaultProjectId)
  if (defaultExists) return defaultProjectId!
  return projects[0]?.id
}
