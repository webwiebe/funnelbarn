import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { ReactNode, useEffect, useState } from 'react'
import { AuthProvider, useAuth } from './lib/auth'
import { ProjectProvider, useProjects } from './lib/projects'
import Landing from './pages/Landing'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Funnels from './pages/Funnels'
import Live from './pages/Live'
import Settings from './pages/Settings'
import IntegrationHealth from './pages/IntegrationHealth'
import Flags from './pages/Flags'
import Insights from './pages/Insights'
import Flows from './pages/Flows'
import Sessions from './pages/Sessions'
import FirstRunWizard from './components/wizards/FirstRunWizard'
import { ErrorBoundary } from './components/ui/ErrorBoundary'
import { LAST_PROJECT_ID_KEY } from './components/ui/ProjectPicker'
import { trackPageView } from './lib/analytics'

function ProtectedRoute({ children }: { children: ReactNode }) {
  const { user, isLoading } = useAuth()

  if (isLoading) {
    return (
      <div style={{
        minHeight: '100vh',
        background: '#0f1117',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}>
        <div style={{
          width: 32,
          height: 32,
          border: '3px solid #2a2d3a',
          borderTopColor: '#f59e0b',
          borderRadius: '50%',
          animation: 'spin 0.7s linear infinite',
        }} />
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      </div>
    )
  }

  if (!user) {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}

function RootRedirect() {
  const { user, isLoading } = useAuth()

  if (isLoading) {
    return (
      <div style={{
        minHeight: '100vh',
        background: '#0f1117',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}>
        <div style={{
          width: 32,
          height: 32,
          border: '3px solid #2a2d3a',
          borderTopColor: '#f59e0b',
          borderRadius: '50%',
          animation: 'spin 0.7s linear infinite',
        }} />
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      </div>
    )
  }

  // Authenticated users skip the marketing landing and go straight to their
  // dashboard. Reuse DefaultProjectRoute so they land on /dashboard/<projectId>
  // when a default/last-visited project is known, rather than bouncing through
  // a generic /dashboard hop.
  if (user) {
    return <DefaultProjectRoute base="/dashboard" />
  }

  return <Landing />
}

function DefaultProjectRoute({ base }: { base: string }) {
  const { projects, isLoading, defaultProjectId } = useProjects()

  if (isLoading) {
    return (
      <div style={{
        minHeight: '100vh',
        background: '#0f1117',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}>
        <div style={{
          width: 32,
          height: 32,
          border: '3px solid #2a2d3a',
          borderTopColor: '#f59e0b',
          borderRadius: '50%',
          animation: 'spin 0.7s linear infinite',
        }} />
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      </div>
    )
  }

  if (projects.length === 0) {
    // No projects yet — render Dashboard which handles the empty/create-project state
    return <Dashboard />
  }

  // Priority: last-visited > explicit default > first project.
  // LAST_PROJECT_ID_KEY is written by Shell on every project-scoped page, so it
  // always reflects where the user just was. defaultProjectId is the persistent
  // preference from Settings — used as a fallback when there is no recent history
  // (e.g. fresh session or first login).
  let lastId: string | null = null
  try { lastId = localStorage.getItem(LAST_PROJECT_ID_KEY) } catch { /* quota / private mode */ }

  const target =
    (lastId && projects.some((p) => p.id === lastId)) ? lastId :
    (defaultProjectId && projects.some((p) => p.id === defaultProjectId)) ? defaultProjectId :
    projects[0].id

  return <Navigate to={`${base}/${target}`} replace />
}

function PageTracker() {
  const location = useLocation()
  useEffect(() => {
    trackPageView()
  }, [location.pathname])
  return null
}

function AppRoutes() {
  const { user } = useAuth()
  const { refetch } = useProjects()
  const [wizardDismissed, setWizardDismissed] = useState(false)

  // Show wizard if user is logged in but has no projects (has_projects === false)
  const showWizard = user && (user as any).has_projects === false && !wizardDismissed

  return (
    <>
      <PageTracker />
      {showWizard && (
        <FirstRunWizard onComplete={() => { refetch(); setWizardDismissed(true) }} />
      )}
      <Routes>
        <Route path="/" element={<RootRedirect />} />
        <Route path="/login" element={<Login />} />
        <Route
          path="/dashboard"
          element={<ProtectedRoute><DefaultProjectRoute base="/dashboard" /></ProtectedRoute>}
        />
        <Route
          path="/dashboard/:projectId"
          element={<ProtectedRoute><Dashboard /></ProtectedRoute>}
        />
        <Route
          path="/funnels"
          element={<ProtectedRoute><DefaultProjectRoute base="/funnels" /></ProtectedRoute>}
        />
        <Route
          path="/funnels/:projectId"
          element={<ProtectedRoute><Funnels /></ProtectedRoute>}
        />
        <Route
          path="/flags"
          element={<ProtectedRoute><DefaultProjectRoute base="/flags" /></ProtectedRoute>}
        />
        <Route
          path="/flags/:projectId"
          element={<ProtectedRoute><Flags /></ProtectedRoute>}
        />
        <Route
          path="/insights"
          element={<ProtectedRoute><DefaultProjectRoute base="/insights" /></ProtectedRoute>}
        />
        <Route
          path="/insights/:projectId"
          element={<ProtectedRoute><Insights /></ProtectedRoute>}
        />
        <Route
          path="/flows"
          element={<ProtectedRoute><DefaultProjectRoute base="/flows" /></ProtectedRoute>}
        />
        <Route
          path="/flows/:projectId"
          element={<ProtectedRoute><Flows /></ProtectedRoute>}
        />
        <Route
          path="/live"
          element={<ProtectedRoute><DefaultProjectRoute base="/live" /></ProtectedRoute>}
        />
        <Route
          path="/live/:projectId"
          element={<ProtectedRoute><Live /></ProtectedRoute>}
        />
        <Route
          path="/sessions"
          element={<ProtectedRoute><DefaultProjectRoute base="/sessions" /></ProtectedRoute>}
        />
        <Route
          path="/sessions/:projectId"
          element={<ProtectedRoute><Sessions /></ProtectedRoute>}
        />
        <Route
          path="/integrations"
          element={<ProtectedRoute><IntegrationHealth /></ProtectedRoute>}
        />
        <Route
          path="/settings"
          element={<ProtectedRoute><Settings /></ProtectedRoute>}
        />
      </Routes>
    </>
  )
}

function App() {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <AuthProvider>
          <ProjectProvider>
            <AppRoutes />
          </ProjectProvider>
        </AuthProvider>
      </BrowserRouter>
    </ErrorBoundary>
  )
}

export default App
