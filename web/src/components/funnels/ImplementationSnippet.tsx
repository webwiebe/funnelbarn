import { useState } from 'react'
import { Funnel } from '../../lib/api'

const LANGS = ['JS', 'React', 'Go', 'Python', 'Swift', 'Kotlin'] as const
type Lang = typeof LANGS[number]

export function ImplementationSnippet({ funnel, apiKey }: { funnel: Funnel; apiKey?: string }) {
  const [copied, setCopied] = useState(false)
  const [activeLang, setActiveLang] = useState<Lang>('JS')
  const origin = typeof window !== 'undefined' ? window.location.origin : '${origin}'
  const steps = funnel.steps || []

  // Convert event_name to camelCase for the const key
  const toCamelCase = (s: string): string => {
    if (!s) return 'step'
    return (
      s
        .replace(/[^a-zA-Z0-9_]/g, '_')
        .replace(/_([a-z])/g, (_, c: string) => c.toUpperCase())
        .replace(/^[0-9]/, '_$&') || 'step'
    )
  }

  const key = apiKey ?? 'YOUR_INGEST_API_KEY'
  const constName = toCamelCase(funnel.name.replace(/\s+/g, '_')) + 'Funnel'
  const constNameCapitalized = constName.charAt(0).toUpperCase() + constName.slice(1)
  const stepEntries = steps.map(s => `  ${toCamelCase(s.event_name)}: '${s.event_name}'`).join(',\n')
  const firstStep = steps[0]?.event_name || 'page_view'
  const midStep = steps[1]?.event_name || 'button_click'
  const lastStep = steps[steps.length - 1]?.event_name || 'conversion'

  const toGoExportedCase = (s: string) =>
    s.replace(/_([a-z])/g, (_, c: string) => c.toUpperCase()).replace(/^\w/, (c: string) => c.toUpperCase())

  const getSnippet = (lang: Lang): string => {
    switch (lang) {
      case 'JS':
        return `<!-- Add to <head> -->
<script src="${origin}/sdk.js"
        data-api-key="${key}"></script>

<script>
// Funnel step constants
const ${constName} = {
${stepEntries}
}

// Track page view (url is auto-captured, add extra context as properties)
funnelbarn.page({ section: 'landing' })

// Button click example — pass context so you can filter in funnels
document.querySelector('#your-cta').addEventListener('click', () => {
  funnelbarn.track(${constName}.${toCamelCase(midStep)}, { page: location.pathname, label: 'hero-cta' })
})

// Form submit — include form name and page for filtering
document.querySelector('form').addEventListener('submit', () => {
  funnelbarn.track(${constName}.${toCamelCase(lastStep)}, { form: 'signup', page: location.pathname })
})

// Scroll depth (50%)
let _scrolled = false
window.addEventListener('scroll', () => {
  const pct = window.scrollY / (document.body.scrollHeight - window.innerHeight)
  if (pct >= 0.5 && !_scrolled) { _scrolled = true; funnelbarn.track('scroll_50') }
}, { passive: true })

// Identify logged-in users (enables "Logged in" / "Not logged in" segments)
// Call this after the user logs in
funnelbarn.identify('user-123')
</script>`

      case 'React':
        return `import { useEffect } from 'react'

// Funnel step constants
const ${constName} = {
${stepEntries}
} as const

type ${constNameCapitalized}Step = typeof ${constName}[keyof typeof ${constName}]

// Tracking hook
function useFunnelTrack() {
  return (step: ${constNameCapitalized}Step, props?: Record<string, unknown>) => {
    window.funnelbarn?.track(step, props)
  }
}

// Usage in your component
function YourComponent() {
  const track = useFunnelTrack()

  // page() auto-captures URL and referrer; add extra properties as needed
  useEffect(() => {
    window.funnelbarn?.page({ section: 'dashboard' })
  }, [])

  // Identify logged-in users (enables "Logged in" / "Not logged in" segments)
  useEffect(() => {
    if (currentUser) window.funnelbarn?.identify(currentUser.id)
  }, [currentUser])

  return (
    <button onClick={() => track(${constName}.${toCamelCase(midStep)}, { page: location.pathname })}>
      Continue
    </button>
  )
}`

      case 'Go':
        return `package main

import (
    "github.com/wiebe-xyz/funnelbarn-go"
)

// Funnel step constants
const (
${steps.map(s => `    ${toGoExportedCase(s.event_name)}Step = "${s.event_name}"`).join('\n')}
)

func main() {
    funnelbarn.Init(funnelbarn.Options{
        APIKey:   "${key}",
        Endpoint: "${origin}",
    })
    defer funnelbarn.Shutdown(2 * time.Second)

    // Track a funnel step
    funnelbarn.Track("${firstStep}", map[string]any{
        "user_id": "user_123",
    })
}`

      case 'Python':
        return `from funnelbarn import FunnelBarnClient

# Funnel step constants
class ${constNameCapitalized}:
${steps.map(s => `    ${s.event_name.toUpperCase()} = "${s.event_name}"`).join('\n')}

# Initialize client
client = FunnelBarnClient(
    api_key="${key}",
    endpoint="${origin}"
)

# Track a step (user_id enables "Logged in" / "Not logged in" segments)
client.track(${constNameCapitalized}.${firstStep.toUpperCase()}, properties={"user_id": "user_123"})

# With properties
client.track(
    ${constNameCapitalized}.${lastStep.toUpperCase().replace(/-/g, '_')},
    properties={"user_id": "user_123", "plan": "pro"}
)

client.flush()`

      case 'Swift':
        return `import Foundation

// Funnel step constants
enum ${constNameCapitalized}Step: String {
${steps.map(s => `    case ${toCamelCase(s.event_name)} = "${s.event_name}"`).join('\n')}
}

// Track a step
func trackFunnelStep(_ step: ${constNameCapitalized}Step,
                     properties: [String: Any] = [:]) {
    var body: [String: Any] = ["name": step.rawValue]
    body.merge(properties) { _, new in new }

    var request = URLRequest(url: URL(string: "${origin}/api/v1/events")!)
    request.httpMethod = "POST"
    request.setValue("application/json", forHTTPHeaderField: "Content-Type")
    request.setValue("${key}", forHTTPHeaderField: "X-FunnelBarn-Api-Key")
    request.httpBody = try? JSONSerialization.data(withJSONObject: body)

    URLSession.shared.dataTask(with: request).resume()
}

// Usage (user_id enables "Logged in" / "Not logged in" segments)
trackFunnelStep(.${toCamelCase(firstStep)}, properties: ["user_id": "user_123"])
trackFunnelStep(.${toCamelCase(lastStep)}, properties: ["plan": "pro"])`

      case 'Kotlin':
        return `import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

// Funnel step constants
object ${constNameCapitalized} {
${steps.map(s => `    const val ${s.event_name.toUpperCase()} = "${s.event_name}"`).join('\n')}
}

// FunnelBarn tracker
class FunnelBarnTracker(private val apiKey: String) {
    private val client = OkHttpClient()
    private val JSON = "application/json".toMediaType()
    private val endpoint = "${origin}/api/v1/events"

    fun track(eventName: String, properties: Map<String, Any> = emptyMap()) {
        val body = JSONObject(mapOf("name" to eventName) + properties)
        val request = Request.Builder()
            .url(endpoint)
            .addHeader("X-FunnelBarn-Api-Key", apiKey)
            .post(body.toString().toRequestBody(JSON))
            .build()
        client.newCall(request).enqueue(object : Callback {
            override fun onFailure(call: Call, e: IOException) {}
            override fun onResponse(call: Call, response: Response) {}
        })
    }
}

// Usage (user_id enables "Logged in" / "Not logged in" segments)
val tracker = FunnelBarnTracker("${key}")
tracker.track(${constNameCapitalized}.${firstStep.toUpperCase().replace(/-/g, '_')},
    mapOf("user_id" to "user_123"))
tracker.track(${constNameCapitalized}.${lastStep.toUpperCase().replace(/-/g, '_')},
    mapOf("user_id" to "user_123", "plan" to "pro"))`
    }
  }

  const snippet = getSnippet(activeLang)

  return (
    <div style={{
      marginTop: '1.5rem',
      background: '#0f1117',
      border: '1px solid #2a2d3a',
      borderRadius: 10,
      overflow: 'hidden',
      maxWidth: '100%',
    }}>
      <div style={{
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        padding: '0.75rem 1rem',
        borderBottom: '1px solid #2a2d3a',
      }}>
        <span style={{ fontSize: 13, fontWeight: 600, color: '#94a3b8' }}>Implementation</span>
        <button
          onClick={() => { navigator.clipboard.writeText(snippet); setCopied(true); setTimeout(() => setCopied(false), 2000) }}
          style={{
            background: 'none', border: '1px solid #2a2d3a', borderRadius: 6,
            color: copied ? '#10b981' : '#94a3b8', cursor: 'pointer',
            padding: '0.25rem 0.6rem', fontSize: 12,
          }}
        >
          {copied ? '✓ Copied' : 'Copy'}
        </button>
      </div>
      <div style={{ display: 'flex', gap: 0, borderBottom: '1px solid #2a2d3a', overflowX: 'auto' }}>
        {LANGS.map(lang => (
          <button key={lang} onClick={() => setActiveLang(lang)} style={{
            padding: '0.5rem 0.875rem',
            background: 'none',
            border: 'none',
            borderBottom: activeLang === lang ? '2px solid #f59e0b' : '2px solid transparent',
            color: activeLang === lang ? '#f59e0b' : '#94a3b8',
            cursor: 'pointer',
            fontSize: 12,
            fontWeight: activeLang === lang ? 700 : 400,
            whiteSpace: 'nowrap',
            flexShrink: 0,
          }}>{lang}</button>
        ))}
      </div>
      <pre style={{
        margin: 0, padding: '1rem',
        fontSize: 12, lineHeight: 1.6,
        color: '#e2e8f0',
        overflowX: 'auto',
        maxWidth: '100%',
        whiteSpace: 'pre',
        fontFamily: 'ui-monospace, SFMono-Regular, monospace',
        WebkitOverflowScrolling: 'touch' as React.CSSProperties['WebkitOverflowScrolling'],
      }}>
        {snippet}
      </pre>
    </div>
  )
}
