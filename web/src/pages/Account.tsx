import { useEffect, useState } from 'react'
import Shell from '../components/shell/Shell'
import { api } from '../lib/api'
import { useIambarnWidget, iambarnThemeVars, type IambarnConfig } from '../lib/iambarn-widget'
import { C } from '../lib/theme'

// Is the current FunnelBarn session an IAMBarn/OIDC session? The OIDC callback
// sets a non-HttpOnly funnelbarn_auth_method=oidc cookie; without it the hosted
// <iambarn-profile> has no IAMBarn session to talk to and renders nothing.
function isOIDCSession(): boolean {
  return typeof document !== 'undefined' &&
    document.cookie.split(';').some((c) => c.trim() === 'funnelbarn_auth_method=oidc')
}

// Account page — hosts IAMBarn's <iambarn-profile> editor (profile, password,
// sessions, passkeys). We own the route/shell; IAMBarn owns the panel.
export default function Account() {
  const [cfg, setCfg] = useState<IambarnConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const oidc = isOIDCSession()

  useEffect(() => {
    let cancelled = false
    api.getClientConfig()
      .then((c) => { if (!cancelled) setCfg(c.iambarn ?? null) })
      .catch(() => { /* non-fatal — handled by the empty-config branch below */ })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [])

  const ready = useIambarnWidget(cfg?.widget_url)

  const message = (text: string) => (
    <div style={{ color: C.muted, fontSize: 14, padding: '2rem 0' }}>{text}</div>
  )

  let body: React.ReactNode
  if (!oidc) {
    body = message('Sign in with IAMBarn to manage your account.')
  } else if (loading || !cfg?.server_url) {
    body = message('Loading…')
  } else if (!ready) {
    body = message('Loading account editor…')
  } else {
    body = <iambarn-profile server-url={cfg.server_url} style={iambarnThemeVars} />
  }

  return (
    <Shell>
      <h1 style={{ fontSize: 22, fontWeight: 700, color: C.text, margin: '0 0 1rem' }}>Account</h1>
      {body}
    </Shell>
  )
}
