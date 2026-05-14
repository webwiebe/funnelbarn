import React from 'react'
import ReactDOM from 'react-dom/client'
import './index.css'
import App from './App'
import { reportError } from './lib/bugbarn'

// Global unhandled error handler — catches errors outside React's tree.
window.onerror = (message, source, lineno, colno, error) => {
  reportError(error ?? new Error(String(message)), {
    source: 'window.onerror',
    file: source,
    line: lineno,
    col: colno,
  })
  // Return false so the browser still logs to the console.
  return false
}

// Global unhandled promise rejection handler.
window.onunhandledrejection = (event: PromiseRejectionEvent) => {
  const reason = event.reason
  const msg = reason instanceof Error ? reason.message : String(reason)
  const stack = reason instanceof Error ? (reason.stack ?? '') : ''

  // Filters for things that aren't application bugs:
  // - "Failed to register a ServiceWorker": untrusted TLS in dev/test envs.
  // - registerSW.js / ServiceWorkerContainer.register: same root cause, but
  //   the rejection sometimes surfaces with a generic message like "Rejected"
  //   so we also match on the stack.
  // - "Failed to fetch" (TypeError): browser-level network blip; the API
  //   client already handles this as ApiError(0). Surfacing it as an
  //   unhandled rejection here means a non-api call dropped — still not
  //   actionable from app code.
  if (msg.includes('Failed to register a ServiceWorker')) return
  if (stack.includes('registerSW.js') || stack.includes('ServiceWorkerContainer.register')) return
  if (reason instanceof TypeError && msg.includes('Failed to fetch')) return

  reportError(reason, {
    source: 'window.onunhandledrejection',
  })
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)

// When a new service worker takes control (skipWaiting + clientsClaim), reload
// the page so users immediately see the latest version instead of staying on
// a stale cached build.
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    window.location.reload()
  })
}
