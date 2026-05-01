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
  reportError(event.reason, {
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
