// Loads the hosted IAMBarn web-component bundle at runtime. The bundle URL comes
// from /api/v1/client-config (see ClientConfig.iambarn.widget_url) so the Docker
// image stays environment-agnostic — no IAMBarn hostname is baked into the build.
//
// useIambarnWidget injects the <script> once (idempotent per URL) and reports
// readiness only after the custom elements have actually upgraded, so callers can
// render nothing / a skeleton until then.
import { useEffect, useState, type CSSProperties } from 'react'
import { C } from './theme'

// Re-exported so component code can import the config type alongside the hook.
export type { IambarnConfig } from './iambarn-config'

/**
 * CSS custom properties that map FunnelBarn's dark theme onto the hosted IAMBarn
 * components. Spread onto each `<iambarn-*>` element's `style`.
 */
export const iambarnThemeVars = {
  '--iambarn-accent': C.amber,
  '--iambarn-accent-hover': C.amber,
  '--iambarn-bg': C.bg,
  '--iambarn-surface': C.surface,
  '--iambarn-text': C.text,
  '--iambarn-border': C.border,
  '--iambarn-muted': C.muted,
  '--iambarn-danger': C.error,
  '--iambarn-radius': '8px',
  '--iambarn-font': 'system-ui, -apple-system, sans-serif',
} as CSSProperties

// One shared promise per bundle URL — repeated calls reuse the same load.
const loaders = new Map<string, Promise<boolean>>()

function whenDefined(): Promise<boolean> {
  if (typeof customElements === 'undefined') return Promise.resolve(false)
  return customElements
    .whenDefined('iambarn-user-menu')
    .then(() => true)
    .catch(() => false)
}

function loadWidget(url: string): Promise<boolean> {
  const existing = loaders.get(url)
  if (existing) return existing

  const promise = new Promise<boolean>((resolve) => {
    if (typeof document === 'undefined') {
      resolve(false)
      return
    }
    const selector = `script[data-iambarn-widget="${url}"]`
    if (document.querySelector(selector)) {
      // Tag already present (e.g. added by a prior mount) — just await upgrade.
      whenDefined().then(resolve)
      return
    }
    const script = document.createElement('script')
    script.src = url
    script.async = true
    script.dataset.iambarnWidget = url
    script.addEventListener('load', () => whenDefined().then(resolve))
    script.addEventListener('error', () => resolve(false))
    document.head.appendChild(script)
  })

  loaders.set(url, promise)
  return promise
}

/**
 * Ensures the IAMBarn widget bundle at `url` is loaded and its custom elements
 * are upgraded. Returns true once ready. A falsy url is a no-op (returns false),
 * which is the correct behaviour when IAMBarn config isn't available.
 */
export function useIambarnWidget(url: string | null | undefined): boolean {
  const [ready, setReady] = useState(false)
  useEffect(() => {
    if (!url) return
    let cancelled = false
    loadWidget(url).then((ok) => {
      if (!cancelled && ok) setReady(true)
    })
    return () => {
      cancelled = true
    }
  }, [url])
  return ready
}

// Exposed for tests to reset the module-level cache between cases.
export function __resetIambarnWidgetLoaders(): void {
  loaders.clear()
}
