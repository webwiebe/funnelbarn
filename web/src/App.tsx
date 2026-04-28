import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ReactNode } from 'react'
import { AuthProvider, useAuth } from './lib/auth'
import Landing from './pages/Landing'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Funnels from './pages/Funnels'
import Live from './pages/Live'
import Settings from './pages/Settings'

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

function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<RootRedirect />} />
      <Route path="/login" element={<Login />} />
      <Route
        path="/dashboard"
        element={<ProtectedRoute><Dashboard /></ProtectedRoute>}
      />
      <Route
        path="/dashboard/:projectId"
        element={<ProtectedRoute><Dashboard /></ProtectedRoute>}
      />
      <Route
        path="/funnels"
        element={<ProtectedRoute><Funnels /></ProtectedRoute>}
      />
      <Route
        path="/funnels/:projectId"
        element={<ProtectedRoute><Funnels /></ProtectedRoute>}
      />
      <Route
        path="/live"
        element={<ProtectedRoute><Live /></ProtectedRoute>}
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
  )
}

function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  )
}

export default App
