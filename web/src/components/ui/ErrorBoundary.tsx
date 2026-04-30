import { Component, ErrorInfo, ReactNode } from 'react'

interface Props {
  children: ReactNode
  fallback?: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
  pendingReset: boolean
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null, pendingReset: false }

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { hasError: true, error, pendingReset: false }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Unhandled render error:', error, info.componentStack)
  }

  componentDidUpdate(_prevProps: Props, prevState: State) {
    if (prevState.pendingReset && this.state.pendingReset && !this.state.hasError) {
      this.setState({ pendingReset: false })
    }
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback
      return (
        <div style={{
          display: 'flex', flexDirection: 'column', alignItems: 'center',
          justifyContent: 'center', minHeight: '100vh',
          background: '#0f1117', color: '#e2e8f0', fontFamily: 'system-ui, sans-serif', gap: 16,
        }}>
          <div style={{ fontSize: 32 }}>⚠</div>
          <h2 style={{ margin: 0, fontWeight: 600 }}>Something went wrong</h2>
          <p style={{ margin: 0, color: '#94a3b8', fontSize: 14 }}>
            {this.state.error?.message ?? 'An unexpected error occurred'}
          </p>
          <button
            onClick={() => this.setState({ hasError: false, error: null, pendingReset: true })}
            style={{
              marginTop: 8, padding: '0.5rem 1.25rem', borderRadius: 6,
              background: '#f59e0b', color: '#0f1117', border: 'none',
              cursor: 'pointer', fontWeight: 600, fontSize: 14,
            }}
          >
            Try again
          </button>
        </div>
      )
    }
    // While pending reset, render nothing to avoid re-throwing with stale children
    if (this.state.pendingReset) {
      return null
    }
    return this.props.children
  }
}
