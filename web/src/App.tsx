import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ReactNode, useState } from 'react'
import { AuthProvider, useAuth } from './lib/auth'
import { ProjectProvider, useProjects } from './lib/projects'
import Landing from './pages/Landing'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Funnels from './pages/Funnels'
import Live from './pages/Live'
import Settings from './pages/Settings'
import ABTests from './pages/ABTests'
import FirstRunWizard from './components/wizards/FirstRunWizard'
import { ErrorBoundary } from './components/ui/ErrorBoundary'

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

  if (user) {
    return <Navigate to="/dashboard" replace />
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

  const target =
    defaultProjectId && projects.some((p) => p.id === defaultProjectId)
      ? defaultProjectId
      : projects[0].id

  return <Navigate to={`${base}/${target}`} replace />
}

function AppRoutes() {
  const { user } = useAuth()
  const { refetch } = useProjects()
  const [wizardDismissed, setWizardDismissed] = useState(false)

  // Show wizard if user is logged in but has no projects (has_projects === false)
  const showWizard = user && (user as any).has_projects === false && !wizardDismissed

  return (
    <>
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
          path="/abtests"
          element={<ProtectedRoute><DefaultProjectRoute base="/abtests" /></ProtectedRoute>}
        />
        <Route
          path="/abtests/:projectId"
          element={<ProtectedRoute><ABTests /></ProtectedRoute>}
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
