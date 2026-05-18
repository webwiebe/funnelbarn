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

export default function Login() {
  const { user, login, isLoading } = useAuth()
  const { refetch: refetchProjects } = useProjects()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [iambarnEnabled, setIambarnEnabled] = useState(false)

  useEffect(() => {
    api.getClientConfig().then((cfg) => {
      setIambarnEnabled(cfg.iambarn_enabled)
      // If the bugbarn-style confidential OIDC flow is configured server-side
      // AND the legacy IAMBarn PKCE flow is not enabled, auto-redirect to the
      // OIDC login URL. When both are configured the legacy button-driven flow
      // takes priority; consolidation is a follow-up.
      const oc = cfg.oidc
      if (!cfg.iambarn_enabled && oc?.enabled && oc.loginURL) {
        window.location.assign(oc.loginURL)
      }
    }).catch(() => {
      // Non-fatal: IAMBarn button simply won't appear; fall back to local form.
    })

    const params = new URLSearchParams(window.location.search)
    if (params.get('error') === 'auth_failed') {
      setError('Login failed. Please try again.')
    }
  }, [])

  if (!isLoading && user) {
    return <Navigate to="/dashboard" replace />
  }

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
    <div style={{
      minHeight: '100vh',
      background: C.bg,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontFamily: 'system-ui, -apple-system, sans-serif',
      padding: '1rem',
    }}>
      {/* Background grid */}
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
        {/* Logo */}
        <div style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <div style={{ fontSize: 40, marginBottom: 8 }}>⬡</div>
          <div style={{ fontSize: 26, fontWeight: 800, color: C.text, letterSpacing: '-0.5px' }}>
            Funnel<span style={{ color: C.amber }}>Barn</span>
          </div>
          <div style={{ fontSize: 14, color: C.muted, marginTop: 6 }}>
            Own your analytics
          </div>
        </div>

        {/* Error */}
        {error && (
          <div style={{
            background: 'rgba(239,68,68,0.1)',
            border: `1px solid rgba(239,68,68,0.3)`,
            borderRadius: 8,
            padding: '0.75rem 1rem',
            color: C.error,
            fontSize: 14,
            marginBottom: '1.25rem',
          }}>
            {error}
          </div>
        )}

        {/* Form */}
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

        {iambarnEnabled && (
          <>
            <div style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.75rem',
              margin: '1.5rem 0 1.25rem',
            }}>
              <div style={{ flex: 1, height: 1, background: C.border }} />
              <span style={{ fontSize: 12, color: C.muted }}>or</span>
              <div style={{ flex: 1, height: 1, background: C.border }} />
            </div>
            <a
              href="/api/v1/auth/oidc/login"
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: '0.5rem',
                width: '100%',
                background: 'transparent',
                border: `1px solid ${C.border}`,
                borderRadius: 8,
                padding: '0.75rem',
                color: C.text,
                fontSize: 15,
                fontWeight: 600,
                textDecoration: 'none',
                transition: 'border-color 0.15s',
                boxSizing: 'border-box',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.borderColor = C.amber)}
              onMouseLeave={(e) => (e.currentTarget.style.borderColor = C.border)}
            >
              <span style={{ fontSize: 18 }}>⬡</span>
              Continue with IAMBarn
            </a>
          </>
        )}
      </div>
    </div>
  )
}
