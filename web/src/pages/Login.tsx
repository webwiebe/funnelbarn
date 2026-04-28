import { useState, FormEvent } from 'react'
import { useNavigate, Navigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'

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
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  if (!isLoading && user) {
    return <Navigate to="/dashboard" replace />
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await login(username, password)
      navigate('/dashboard', { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError('Invalid username or password')
      } else {
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
            <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6, fontWeight: 500 }}>
              Username
            </label>
            <input
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
            <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6, fontWeight: 500 }}>
              Password
            </label>
            <input
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
      </div>
    </div>
  )
}
