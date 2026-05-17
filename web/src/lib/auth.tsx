import { createContext, useContext, useEffect, useState, ReactNode } from 'react'
import { api, User, ApiError } from './api'
import { trackEvent, identifyUser } from './analytics'

interface AuthContextValue {
  user: User | null
  isLoading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    api.me()
      .then((u) => { setUser(u); identifyUser(u.id) })
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 401) {
          setUser(null)
        } else {
          console.error('auth check failed', e)
          setUser(null)
        }
      })
      .finally(() => setIsLoading(false))
  }, [])

  const login = async (username: string, password: string) => {
    const u = await api.login({ username, password })
    setUser(u)
    identifyUser(u.id)
  }

  const logout = async () => {
    trackEvent('logout')
    await api.logout()
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{ user, isLoading, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}
