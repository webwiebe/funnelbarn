import { useState, useEffect, FormEvent } from 'react'
import { useNavigate, Navigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { useProjects } from '../lib/projects'
import { api, ApiError } from '../lib/api'
import { trackEvent } from '../lib/analytics'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  error: '#ef4444',
}

type AuthMode =
  | { kind: 'loading' }
  | { kind: 'redirect'; url: string }   // any configured OIDC/SSO provider
  | { kind: 'local' }                   // username + password only

function useAuthMode(): { mode: AuthMode; authError: boolean } {
  const [mode, setMode] = useState<AuthMode>({ kind: 'loading' })
  const authError = new URLSearchParams(window.location.search).get('error') === 'auth_failed'

  useEffect(() => {
    api.getClientConfig().then((cfg) => {
      if (cfg.iambarn_enabled) {
        setMode({ kind: 'redirect', url: '/api/v1/auth/oidc/login' })
        // Only auto-redirect when there's no error to display.
        if (!authError) {
          window.location.assign('/api/v1/auth/oidc/login')
        }
      } else if (cfg.oidc?.enabled && cfg.oidc.loginURL) {
        setMode({ kind: 'redirect', url: cfg.oidc.loginURL })
        if (!authError) {
          window.location.assign(cfg.oidc.loginURL)
        }
      } else {
        setMode({ kind: 'local' })
      }
    }).catch(() => {
      setMode({ kind: 'local' })
    })
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return { mode, authError }
}

function Card({ children }: { children: React.ReactNode }) {
  return (
    <div style={{
      minHeight: '100vh',
      background: C.bg,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontFamily: 'system-ui, -apple-system, sans-serif',
      padding: '1rem',
    }}>
      <div style={{
        position: 'fixed',
        inset: 0,
        backgroundImage: 'radial-gradient(circle, #2a2d3a 1px, transparent 1px)',
        backgroundSize: '32px 32px',
        opacity: 0.4,
        pointerEvents: 'none',
      }} />
      <div style={{
        position: 'relative',
        width: '100%',
        maxWidth: 400,
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 16,
        padding: '2.5rem',
        boxShadow: '0 25px 60px rgba(0,0,0,0.5)',
      }}>
        <div style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <div style={{ fontSize: 40, marginBottom: 8 }}>⬡</div>
          <div style={{ fontSize: 26, fontWeight: 800, color: C.text, letterSpacing: '-0.5px' }}>
            Funnel<span style={{ color: C.amber }}>Barn</span>
          </div>
          <div style={{ fontSize: 14, color: C.muted, marginTop: 6 }}>
            Own your analytics
          </div>
        </div>
        {children}
      </div>
    </div>
  )
}

function RedirectView({ url, authError }: { url: string; authError: boolean }) {
  return (
    <Card>
      {authError ? (
        <>
          <div style={{
            background: 'rgba(239,68,68,0.1)',
            border: '1px solid rgba(239,68,68,0.3)',
            borderRadius: 8,
            padding: '0.75rem 1rem',
            color: C.error,
            fontSize: 14,
            marginBottom: '1.25rem',
            textAlign: 'center',
          }}>
            Login failed. Please try again.
          </div>
          <a
            href={url}
            style={{
              display: 'block',
              width: '100%',
              background: C.amber,
              border: 'none',
              borderRadius: 8,
              padding: '0.75rem',
              color: '#0f1117',
              fontSize: 15,
              fontWeight: 700,
              textAlign: 'center',
              textDecoration: 'none',
              boxSizing: 'border-box',
            }}
          >
            Try again
          </a>
        </>
      ) : (
        <div style={{ textAlign: 'center', color: C.muted, fontSize: 14 }}>
          <div style={{
            width: 28,
            height: 28,
            border: `3px solid ${C.border}`,
            borderTopColor: C.amber,
            borderRadius: '50%',
            animation: 'spin 0.7s linear infinite',
            margin: '0 auto 1rem',
          }} />
          Redirecting…
          <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
        </div>
      )}
    </Card>
  )
}

function LocalLoginForm() {
  const { login } = useAuth()
  const { refetch: refetchProjects } = useProjects()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const authError = new URLSearchParams(window.location.search).get('error') === 'auth_failed'
  useEffect(() => {
    if (authError) setError('Login failed. Please try again.')
  }, [authError])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await login(username, password)
      trackEvent('login', { method: 'password' })
      refetchProjects()
      navigate('/dashboard', { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        trackEvent('login_failed', { reason: 'invalid_credentials' })
        setError('Invalid username or password')
      } else {
        trackEvent('login_failed', { reason: 'error' })
        setError('Something went wrong. Please try again.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card>
      {error && (
        <div style={{
          background: 'rgba(239,68,68,0.1)',
          border: '1px solid rgba(239,68,68,0.3)',
          borderRadius: 8,
          padding: '0.75rem 1rem',
          color: C.error,
          fontSize: 14,
          marginBottom: '1.25rem',
        }}>
          {error}
        </div>
      )}
      <form onSubmit={handleSubmit}>
        <div style={{ marginBottom: '1rem' }}>
          <label htmlFor="username" style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6, fontWeight: 500 }}>
            Username
          </label>
          <input
            id="username"
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
            autoComplete="username"
            style={{
              width: '100%',
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              padding: '0.65rem 0.875rem',
              color: C.text,
              fontSize: 15,
              outline: 'none',
              boxSizing: 'border-box',
              transition: 'border-color 0.15s',
            }}
            onFocus={(e) => (e.target.style.borderColor = C.amber)}
            onBlur={(e) => (e.target.style.borderColor = C.border)}
          />
        </div>
        <div style={{ marginBottom: '1.5rem' }}>
          <label htmlFor="password" style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6, fontWeight: 500 }}>
            Password
          </label>
          <input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
            style={{
              width: '100%',
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              padding: '0.65rem 0.875rem',
              color: C.text,
              fontSize: 15,
              outline: 'none',
              boxSizing: 'border-box',
              transition: 'border-color 0.15s',
            }}
            onFocus={(e) => (e.target.style.borderColor = C.amber)}
            onBlur={(e) => (e.target.style.borderColor = C.border)}
          />
        </div>
        <button
          type="submit"
          disabled={submitting}
          style={{
            width: '100%',
            background: submitting ? '#78481a' : C.amber,
            border: 'none',
            borderRadius: 8,
            padding: '0.75rem',
            color: '#0f1117',
            fontSize: 15,
            fontWeight: 700,
            cursor: submitting ? 'not-allowed' : 'pointer',
            transition: 'background 0.15s',
          }}
        >
          {submitting ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </Card>
  )
}

export default function Login() {
  const { user, isLoading } = useAuth()
  const { mode, authError } = useAuthMode()

  if (!isLoading && user) {
    return <Navigate to="/dashboard" replace />
  }

  if (mode.kind === 'loading') {
    return (
      <div style={{ minHeight: '100vh', background: '#0f1117', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <div style={{
          width: 32, height: 32,
          border: '3px solid #2a2d3a',
          borderTopColor: '#f59e0b',
          borderRadius: '50%',
          animation: 'spin 0.7s linear infinite',
        }} />
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      </div>
    )
  }

  if (mode.kind === 'redirect') {
    return <RedirectView url={mode.url} authError={authError} />
  }

  return <LocalLoginForm />
}
