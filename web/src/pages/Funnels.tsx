import { useEffect, useState, useRef } from 'react'
import { useParams } from 'react-router-dom'
import { Plus, X, Layers, Pencil, Trash2, ChevronDown, ChevronRight } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api, ApiError, Funnel, FunnelAnalysis, FunnelStepInput, Segment, SegmentRule } from '../lib/api'
import { useProjects } from '../lib/projects'
import { trackEvent } from '../lib/analytics'
import { reportError } from '../lib/bugbarn'

const LANGS = ['JS', 'React', 'Go', 'Python', 'Swift', 'Kotlin'] as const
type Lang = typeof LANGS[number]

// ─── Templates ────────────────────────────────────────────────────────────────

interface FunnelTemplate {
  name: string
  scope: 'session' | 'page_view'
  steps: string[]
}

const FUNNEL_TEMPLATES: FunnelTemplate[] = [
  {
    name: 'Page Engagement',
    scope: 'page_view',
    steps: ['page_view', 'page_engaged', 'your_goal'],
  },
  {
    name: 'Lead Capture',
    scope: 'session',
    steps: ['page_view', 'form_submit'],
  },
  {
    name: 'E-commerce',
    scope: 'session',
    steps: ['page_view', 'add_to_cart', 'purchase'],
  },
  {
    name: 'SaaS Signup',
    scope: 'session',
    steps: ['page_view', 'signup_started', 'signup_completed'],
  },
]

// ─── Segment rule fields / operators ──────────────────────────────────────────

const SEGMENT_FIELD_META: Record<string, { label: string; placeholder: string; hint: string }> = {
  country_code:     { label: 'Country code',       placeholder: 'NL, US, DE, FR…',            hint: 'ISO 3166-1 alpha-2 code from visitor IP' },
  city:             { label: 'City',                placeholder: 'Amsterdam, London…',          hint: 'City resolved from visitor IP (needs geo DB)' },
  device_type:      { label: 'Device type',         placeholder: 'mobile, desktop, tablet',    hint: 'Detected from browser User-Agent' },
  browser:          { label: 'Browser',             placeholder: 'Chrome, Firefox, Safari…',   hint: 'Browser name from User-Agent' },
  os:               { label: 'OS',                  placeholder: 'Windows, macOS, iOS…',       hint: 'Operating system from User-Agent' },
  connection_class: { label: 'Connection class',    placeholder: 'residential, mobile, datacenter', hint: 'Inferred from ASN — datacenter often means bots or VPNs' },
  dark_mode:        { label: 'Dark mode',           placeholder: '1  (dark)  or  0  (light)',  hint: 'Collected by the SDK from prefers-color-scheme' },
  browser_timezone: { label: 'Browser timezone',    placeholder: 'Europe/Amsterdam, UTC…',     hint: 'IANA timezone from Intl.DateTimeFormat, collected by SDK' },
}

const SEGMENT_FIELDS = Object.keys(SEGMENT_FIELD_META) as (keyof typeof SEGMENT_FIELD_META)[]

const SEGMENT_OPERATORS = [
  { value: 'eq', label: '= equals' },
  { value: 'neq', label: '≠ not equals' },
  { value: 'contains', label: '⊇ contains' },
  { value: 'not_contains', label: '⊅ not contains' },
  { value: 'is_null', label: '∅ is empty' },
  { value: 'is_not_null', label: '∃ is present' },
] as const

type SegmentOperator = SegmentRule['operator']

// Starter segment templates — shown when the project has no segments yet.
const SEGMENT_TEMPLATES: Array<{ name: string; description: string; rules: SegmentRule[] }> = [
  {
    name: 'Mobile visitors',
    description: 'Phones and tablets only',
    rules: [{ field: 'device_type', operator: 'eq', value: 'mobile' }],
  },
  {
    name: 'Desktop visitors',
    description: 'Desktop browsers',
    rules: [{ field: 'device_type', operator: 'eq', value: 'desktop' }],
  },
  {
    name: 'Dark mode users',
    description: 'Users with dark mode enabled',
    rules: [{ field: 'dark_mode', operator: 'eq', value: '1' }],
  },
  {
    name: 'Datacenter / bots',
    description: 'Traffic from cloud IPs — good to exclude',
    rules: [{ field: 'connection_class', operator: 'eq', value: 'datacenter' }],
  },
  {
    name: 'Netherlands',
    description: 'Visitors from NL',
    rules: [{ field: 'country_code', operator: 'eq', value: 'NL' }],
  },
  {
    name: 'Chrome users',
    description: 'Chrome browser only',
    rules: [{ field: 'browser', operator: 'eq', value: 'Chrome' }],
  },
]

// ─── Colours ─────────────────────────────────────────────────────────────────

function ImplementationSnippet({ funnel, apiKey }: { funnel: Funnel; apiKey?: string }) {
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

const PRESET_SEGMENTS = [
  { id: 'all', label: 'All', tip: 'All sessions, no filtering applied' },
  { id: 'logged_in', label: 'Logged in', tip: 'Sessions where identify() was called with a user ID. Requires: analytics.identify("user-123") in your tracking code.' },
  { id: 'not_logged_in', label: 'Not logged in', tip: 'Sessions with no user ID. If you never call identify(), all sessions appear here.' },
  { id: 'mobile', label: 'Mobile', tip: 'Detected automatically from the User-Agent header — no code changes needed.' },
  { id: 'desktop', label: 'Desktop', tip: 'Detected automatically from the User-Agent header — no code changes needed.' },
  { id: 'tablet', label: 'Tablet', tip: 'Detected automatically from the User-Agent header — no code changes needed.' },
  { id: 'new_visitor', label: 'New visitors', tip: 'First-time sessions (1 event only). Tracked automatically via the SDK session in localStorage.' },
  { id: 'returning', label: 'Returning', tip: 'Sessions with more than one event. Tracked automatically via the SDK session in localStorage.' },
]

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  success: '#10b981',
  error: '#ef4444',
}

function conversionColor(pct: number) {
  if (pct >= 0.6) return C.success
  if (pct >= 0.3) return C.amber
  return C.error
}

// ─── Scope toggle ─────────────────────────────────────────────────────────────

function ScopeToggle({
  value,
  onChange,
}: {
  value: 'session' | 'page_view'
  onChange: (v: 'session' | 'page_view') => void
}) {
  return (
    <div style={{ marginBottom: '1.25rem' }}>
      <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>Scope</label>
      <div style={{
        display: 'inline-flex',
        background: C.bg,
        border: `1px solid ${C.border}`,
        borderRadius: 8,
        overflow: 'hidden',
      }}>
        {(['session', 'page_view'] as const).map((opt) => {
          const active = value === opt
          return (
            <button
              key={opt}
              onClick={() => onChange(opt)}
              style={{
                padding: '0.45rem 1rem',
                border: 'none',
                background: active ? 'rgba(245,158,11,0.18)' : 'transparent',
                color: active ? C.amber : C.muted,
                fontWeight: active ? 700 : 400,
                fontSize: 13,
                cursor: 'pointer',
                borderRight: opt === 'session' ? `1px solid ${C.border}` : 'none',
                transition: 'all 0.15s',
              }}
            >
              {opt === 'session' ? 'Session' : 'Page view'}
            </button>
          )
        })}
      </div>
    </div>
  )
}

// ─── Templates dropdown ───────────────────────────────────────────────────────

function TemplatesButton({
  onSelect,
}: {
  onSelect: (tpl: FunnelTemplate) => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button
        onClick={() => setOpen((o) => !o)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          background: 'transparent',
          border: `1px solid ${C.border}`,
          borderRadius: 8,
          color: C.muted,
          padding: '0.6rem 1.1rem',
          cursor: 'pointer',
          fontSize: 14,
          fontWeight: 600,
        }}
      >
        Templates
        <ChevronDown size={14} />
      </button>
      {open && (
        <div style={{
          position: 'absolute',
          top: 'calc(100% + 6px)',
          right: 0,
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 10,
          boxShadow: '0 12px 32px rgba(0,0,0,0.4)',
          zIndex: 300,
          minWidth: 260,
          overflow: 'hidden',
        }}>
          {FUNNEL_TEMPLATES.map((tpl) => (
            <button
              key={tpl.name}
              onClick={() => { onSelect(tpl); setOpen(false) }}
              style={{
                display: 'block',
                width: '100%',
                textAlign: 'left',
                background: 'none',
                border: 'none',
                borderBottom: `1px solid ${C.border}`,
                padding: '0.75rem 1rem',
                cursor: 'pointer',
                color: C.text,
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(245,158,11,0.08)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'none')}
            >
              <div style={{ fontWeight: 700, fontSize: 13, marginBottom: 2 }}>{tpl.name}</div>
              <div style={{ fontSize: 11, color: C.muted }}>
                {tpl.steps.join(' → ')}
                <span style={{
                  marginLeft: 6,
                  background: 'rgba(245,158,11,0.15)',
                  color: C.amber,
                  borderRadius: 4,
                  padding: '0.1rem 0.4rem',
                  fontSize: 10,
                  fontWeight: 600,
                }}>
                  {tpl.scope === 'page_view' ? 'page view' : 'session'}
                </span>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── EventNameInput ───────────────────────────────────────────────────────────

function EventNameInput({
  value,
  onChange,
  placeholder,
  eventNames,
}: {
  value: string
  onChange: (val: string) => void
  placeholder?: string
  eventNames: string[]
}) {
  const [open, setOpen] = useState(false)
  const [focused, setFocused] = useState(false)
  const wrapperRef = useRef<HTMLDivElement>(null)

  const filtered = eventNames.filter(
    (n) => n.toLowerCase().includes(value.toLowerCase()) && n !== value
  )

  return (
    <div ref={wrapperRef} style={{ position: 'relative', flex: 1 }}>
      <input
        value={value}
        onChange={(e) => { onChange(e.target.value); setOpen(true) }}
        onFocus={(e) => { setFocused(true); setOpen(true); e.target.style.borderColor = C.amber }}
        onBlur={(e) => { setFocused(false); e.target.style.borderColor = C.border; setTimeout(() => setOpen(false), 150) }}
        placeholder={placeholder || 'Event name, e.g. page_view'}
        style={{
          width: '100%',
          background: C.bg,
          border: `1px solid ${C.border}`,
          borderRadius: 8,
          padding: '0.55rem 0.875rem',
          color: C.text,
          fontSize: 14,
          outline: 'none',
          boxSizing: 'border-box',
        }}
      />
      {open && focused && filtered.length > 0 && (
        <div style={{
          position: 'absolute',
          top: '100%',
          left: 0,
          right: 0,
          marginTop: 4,
          background: C.bg,
          border: `1px solid ${C.border}`,
          borderRadius: 8,
          maxHeight: 160,
          overflowY: 'auto',
          zIndex: 100,
        }}>
          {filtered.slice(0, 10).map((n) => (
            <div
              key={n}
              onMouseDown={(e) => { e.preventDefault(); onChange(n); setOpen(false) }}
              style={{
                padding: '0.45rem 0.875rem',
                fontSize: 13,
                color: C.text,
                cursor: 'pointer',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(245,158,11,0.1)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              {n}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function AutocompleteInput({
  value,
  onChange,
  placeholder,
  suggestions,
  width,
}: {
  value: string
  onChange: (val: string) => void
  placeholder?: string
  suggestions: string[]
  width?: number
}) {
  const [open, setOpen] = useState(false)
  const [focused, setFocused] = useState(false)

  const filtered = suggestions.filter(
    (s) => s.toLowerCase().includes(value.toLowerCase()) && s !== value
  )

  return (
    <div style={{ position: 'relative' }}>
      <input
        value={value}
        onChange={(e) => { onChange(e.target.value); setOpen(true) }}
        onFocus={() => { setFocused(true); setOpen(true) }}
        onBlur={() => { setFocused(false); setTimeout(() => setOpen(false), 150) }}
        placeholder={placeholder}
        style={{
          width: width ?? 120,
          background: C.bg,
          border: `1px solid ${C.border}`,
          borderRadius: 6,
          padding: '0.3rem 0.5rem',
          color: C.text,
          fontSize: 12,
          outline: 'none',
          boxSizing: 'border-box',
        }}
      />
      {open && focused && filtered.length > 0 && (
        <div style={{
          position: 'absolute',
          top: '100%',
          left: 0,
          marginTop: 2,
          background: C.bg,
          border: `1px solid ${C.border}`,
          borderRadius: 6,
          maxHeight: 120,
          overflowY: 'auto',
          zIndex: 200,
          minWidth: width ?? 120,
        }}>
          {filtered.slice(0, 10).map((s) => (
            <div
              key={s}
              onMouseDown={(e) => { e.preventDefault(); onChange(s); setOpen(false) }}
              style={{
                padding: '0.3rem 0.5rem',
                fontSize: 12,
                color: C.text,
                cursor: 'pointer',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(245,158,11,0.1)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              {s}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function AvailableEventsPanel({
  eventNames,
  onSelect,
}: {
  eventNames: string[]
  onSelect: (name: string) => void
}) {
  const [expanded, setExpanded] = useState(false)

  if (eventNames.length === 0) return null

  return (
    <div style={{
      marginBottom: '1.25rem',
      background: C.bg,
      border: `1px solid ${C.border}`,
      borderRadius: 8,
    }}>
      <button
        onClick={() => setExpanded(!expanded)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          width: '100%',
          background: 'none',
          border: 'none',
          color: C.muted,
          cursor: 'pointer',
          padding: '0.6rem 0.75rem',
          fontSize: 13,
          fontWeight: 600,
          textAlign: 'left',
        }}
      >
        {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        Available events ({eventNames.length})
      </button>
      {expanded && (
        <div style={{
          padding: '0 0.75rem 0.6rem',
          display: 'flex',
          flexWrap: 'wrap',
          gap: 6,
        }}>
          {eventNames.map((name) => (
            <button
              key={name}
              onClick={() => onSelect(name)}
              style={{
                background: 'rgba(245,158,11,0.08)',
                border: `1px solid rgba(245,158,11,0.25)`,
                borderRadius: 6,
                color: C.amber,
                padding: '0.3rem 0.6rem',
                fontSize: 12,
                cursor: 'pointer',
                fontWeight: 500,
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.background = 'rgba(245,158,11,0.18)'
                e.currentTarget.style.borderColor = 'rgba(245,158,11,0.5)'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.background = 'rgba(245,158,11,0.08)'
                e.currentTarget.style.borderColor = 'rgba(245,158,11,0.25)'
              }}
            >
              {name}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

interface StepWithFilters {
  event_name: string
  filters: { property: string; value: string }[]
}

// ─── Create Funnel Modal ───────────────────────────────────────────────────────

function CreateFunnelModal({
  projectId,
  onClose,
  onCreated,
  initialTemplate,
}: {
  projectId: string
  onClose: () => void
  onCreated: (f: Funnel) => void
  initialTemplate?: FunnelTemplate
}) {
  const [name, setName] = useState(initialTemplate?.name ?? '')
  const [scope, setScope] = useState<'session' | 'page_view'>(initialTemplate?.scope ?? 'session')
  const [steps, setSteps] = useState<StepWithFilters[]>(
    initialTemplate
      ? initialTemplate.steps.map((s) => ({ event_name: s, filters: [] }))
      : [{ event_name: '', filters: [] }, { event_name: '', filters: [] }]
  )
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [eventNames, setEventNames] = useState<string[]>([])
  const [stepProperties, setStepProperties] = useState<Record<number, string[]>>({})
  const [filterValues, setFilterValues] = useState<Record<string, string[]>>({})

  useEffect(() => {
    api.getEventNames(projectId).then((d) => setEventNames(d.event_names || [])).catch(() => {})
  }, [projectId])

  const fetchProperties = (stepIdx: number, eventName: string) => {
    if (!eventName) return
    api.getEventProperties(projectId, eventName)
      .then((d) => setStepProperties((prev) => ({ ...prev, [stepIdx]: d.properties || [] })))
      .catch(() => {})
  }

  const fetchPropertyValues = (eventName: string, property: string) => {
    if (!eventName || !property) return
    const cacheKey = `${eventName}:${property}`
    if (filterValues[cacheKey]) return
    api.getEventPropertyValues(projectId, eventName, property)
      .then((d) => setFilterValues((prev) => ({ ...prev, [cacheKey]: d.values || [] })))
      .catch(() => {})
  }

  const addStep = () => setSteps((s) => [...s, { event_name: '', filters: [] }])
  const removeStep = (i: number) => setSteps((s) => s.filter((_, idx) => idx !== i))
  const updateStep = (i: number, val: string) => {
    setSteps((s) => s.map((st, idx) => (idx === i ? { ...st, event_name: val } : st)))
    fetchProperties(i, val)
  }
  const addFilter = (i: number) =>
    setSteps((s) => s.map((st, idx) => idx === i ? { ...st, filters: [...st.filters, { property: '', value: '' }] } : st))
  const addFilterWithProperty = (stepIdx: number, property: string) => {
    setSteps((s) => s.map((st, i) => i === stepIdx ? { ...st, filters: [...st.filters, { property, value: '' }] } : st))
    const eventName = steps[stepIdx]?.event_name
    if (eventName) fetchPropertyValues(eventName, property)
  }
  const removeFilter = (stepIdx: number, filterIdx: number) =>
    setSteps((s) => s.map((st, i) => i === stepIdx ? { ...st, filters: st.filters.filter((_, fi) => fi !== filterIdx) } : st))
  const updateFilter = (stepIdx: number, filterIdx: number, field: 'property' | 'value', val: string) => {
    setSteps((s) => s.map((st, i) => i === stepIdx ? { ...st, filters: st.filters.map((f, fi) => fi === filterIdx ? { ...f, [field]: val } : f) } : st))
    if (field === 'property') {
      const eventName = steps[stepIdx]?.event_name
      if (eventName && val) fetchPropertyValues(eventName, val)
    }
  }

  const handleCreate = async () => {
    if (!name.trim()) { setError('Name is required'); return }
    if (steps.some((s) => !s.event_name.trim())) { setError('All steps need an event name'); return }
    setSaving(true)
    setError(null)
    try {
      const apiSteps: FunnelStepInput[] = steps.map((s) => ({
        event_name: s.event_name,
        ...(s.filters.length > 0 ? { filters: s.filters.filter((f) => f.property && f.value) } : {}),
      }))
      const f = await api.createFunnel(projectId, name, apiSteps, scope)
      trackEvent('funnel_created', { funnel_name: name, step_count: steps.length })
      onCreated(f)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      background: 'rgba(0,0,0,0.7)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 500,
      padding: '1rem',
    }}>
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 14,
        padding: '2rem',
        width: '100%',
        maxWidth: 500,
        boxShadow: '0 25px 60px rgba(0,0,0,0.5)',
        maxHeight: '90vh',
        overflowY: 'auto',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>Create funnel</h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer' }}>
            <X size={18} />
          </button>
        </div>

        {error && (
          <div style={{
            background: 'rgba(239,68,68,0.1)',
            border: `1px solid rgba(239,68,68,0.3)`,
            borderRadius: 8,
            padding: '0.6rem 0.875rem',
            color: C.error,
            fontSize: 14,
            marginBottom: '1rem',
          }}>
            {error}
          </div>
        )}

        <div style={{ marginBottom: '1.25rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>Funnel name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Signup flow"
            style={{
              width: '100%',
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              padding: '0.6rem 0.875rem',
              color: C.text,
              fontSize: 14,
              boxSizing: 'border-box',
              outline: 'none',
            }}
            onFocus={(e) => (e.target.style.borderColor = C.amber)}
            onBlur={(e) => (e.target.style.borderColor = C.border)}
          />
        </div>

        <ScopeToggle value={scope} onChange={setScope} />

        <AvailableEventsPanel
          eventNames={eventNames}
          onSelect={(eName) => {
            const emptyIdx = steps.findIndex((s) => !s.event_name)
            if (emptyIdx >= 0) {
              updateStep(emptyIdx, eName)
            } else {
              setSteps((prev) => [...prev, { event_name: eName, filters: [] }])
              fetchProperties(steps.length, eName)
            }
          }}
        />

        <div style={{ marginBottom: '1.25rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 10 }}>Steps</label>
          {steps.map((s, i) => (
            <div key={i} style={{ marginBottom: 10 }}>
              <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <div style={{
                  width: 24,
                  height: 24,
                  background: C.amber,
                  color: '#0f1117',
                  borderRadius: '50%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 12,
                  fontWeight: 800,
                  flexShrink: 0,
                }}>
                  {i + 1}
                </div>
                <EventNameInput
                  value={s.event_name}
                  onChange={(val) => updateStep(i, val)}
                  eventNames={eventNames}
                />
                {steps.length > 2 && (
                  <button
                    onClick={() => removeStep(i)}
                    style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer', padding: 4 }}
                  >
                    <X size={14} />
                  </button>
                )}
              </div>
              <div style={{ marginLeft: 32, marginTop: 4 }}>
                {(stepProperties[i] || []).length > 0 && (
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginBottom: 4 }}>
                    {(stepProperties[i] || [])
                      .filter((p) => !s.filters.some((f) => f.property === p))
                      .map((prop) => (
                        <button
                          key={prop}
                          onClick={() => addFilterWithProperty(i, prop)}
                          style={{
                            background: 'rgba(148,163,184,0.08)',
                            border: `1px solid ${C.border}`,
                            borderRadius: 4,
                            color: C.muted,
                            padding: '0.15rem 0.4rem',
                            fontSize: 11,
                            cursor: 'pointer',
                          }}
                          onMouseEnter={(e) => {
                            e.currentTarget.style.borderColor = C.amber
                            e.currentTarget.style.color = C.amber
                          }}
                          onMouseLeave={(e) => {
                            e.currentTarget.style.borderColor = C.border
                            e.currentTarget.style.color = C.muted
                          }}
                        >
                          + {prop}
                        </button>
                      ))}
                  </div>
                )}
                {s.filters.map((f, fi) => (
                  <div key={fi} style={{ display: 'flex', gap: 4, alignItems: 'center', marginBottom: 4 }}>
                    <AutocompleteInput
                      value={f.property}
                      onChange={(val) => updateFilter(i, fi, 'property', val)}
                      placeholder="Property"
                      suggestions={stepProperties[i] || []}
                      width={120}
                    />
                    <AutocompleteInput
                      value={f.value}
                      onChange={(val) => updateFilter(i, fi, 'value', val)}
                      placeholder="Value"
                      suggestions={filterValues[`${s.event_name}:${f.property}`] || []}
                      width={120}
                    />
                    <button
                      onClick={() => removeFilter(i, fi)}
                      style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer', padding: 2 }}
                    >
                      <X size={12} />
                    </button>
                  </div>
                ))}
                <button
                  onClick={() => addFilter(i)}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: C.muted,
                    cursor: 'pointer',
                    fontSize: 12,
                    padding: '2px 0',
                  }}
                >
                  + Add filter
                </button>
              </div>
            </div>
          ))}
          <button
            onClick={addStep}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              background: 'transparent',
              border: `1px dashed ${C.border}`,
              borderRadius: 8,
              color: C.muted,
              padding: '0.5rem 1rem',
              cursor: 'pointer',
              fontSize: 13,
              width: '100%',
              justifyContent: 'center',
              marginTop: 4,
            }}
          >
            <Plus size={14} /> Add step
          </button>
        </div>

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button
            onClick={onClose}
            style={{
              background: 'transparent',
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              color: C.muted,
              padding: '0.6rem 1.25rem',
              cursor: 'pointer',
              fontSize: 14,
            }}
          >
            Cancel
          </button>
          <button
            onClick={handleCreate}
            disabled={saving}
            style={{
              background: saving ? '#78481a' : C.amber,
              border: 'none',
              borderRadius: 8,
              color: '#0f1117',
              padding: '0.6rem 1.25rem',
              cursor: saving ? 'not-allowed' : 'pointer',
              fontSize: 14,
              fontWeight: 700,
            }}
          >
            {saving ? 'Creating…' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── Edit Funnel Modal ────────────────────────────────────────────────────────

function EditFunnelModal({
  projectId,
  funnel,
  onClose,
  onUpdated,
}: {
  projectId: string
  funnel: Funnel
  onClose: () => void
  onUpdated: (f: Funnel) => void
}) {
  const [name, setName] = useState(funnel.name)
  const [scope, setScope] = useState<'session' | 'page_view'>(funnel.scope ?? 'session')
  const [steps, setSteps] = useState<StepWithFilters[]>(
    funnel.steps?.length > 0
      ? funnel.steps.map((s) => ({ event_name: s.event_name, filters: s.filters || [] }))
      : [{ event_name: '', filters: [] }, { event_name: '', filters: [] }]
  )
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [eventNames, setEventNames] = useState<string[]>([])
  const [stepProperties, setStepProperties] = useState<Record<number, string[]>>({})
  const [filterValues, setFilterValues] = useState<Record<string, string[]>>({})

  const fetchProperties = (stepIdx: number, eventName: string) => {
    if (!eventName) return
    api.getEventProperties(projectId, eventName)
      .then((d) => setStepProperties((prev) => ({ ...prev, [stepIdx]: d.properties || [] })))
      .catch(() => {})
  }

  const fetchPropertyValues = (eventName: string, property: string) => {
    if (!eventName || !property) return
    const cacheKey = `${eventName}:${property}`
    if (filterValues[cacheKey]) return
    api.getEventPropertyValues(projectId, eventName, property)
      .then((d) => setFilterValues((prev) => ({ ...prev, [cacheKey]: d.values || [] })))
      .catch(() => {})
  }

  useEffect(() => {
    api.getEventNames(projectId).then((d) => setEventNames(d.event_names || [])).catch(() => {})
  }, [projectId])

  useEffect(() => {
    steps.forEach((s, i) => {
      if (s.event_name) fetchProperties(i, s.event_name)
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const addStep = () => setSteps((s) => [...s, { event_name: '', filters: [] }])
  const removeStep = (i: number) => setSteps((s) => s.filter((_, idx) => idx !== i))
  const updateStep = (i: number, val: string) => {
    setSteps((s) => s.map((st, idx) => (idx === i ? { ...st, event_name: val } : st)))
    fetchProperties(i, val)
  }
  const addFilter = (i: number) =>
    setSteps((s) => s.map((st, idx) => idx === i ? { ...st, filters: [...st.filters, { property: '', value: '' }] } : st))
  const addFilterWithProperty = (stepIdx: number, property: string) => {
    setSteps((s) => s.map((st, i) => i === stepIdx ? { ...st, filters: [...st.filters, { property, value: '' }] } : st))
    const eventName = steps[stepIdx]?.event_name
    if (eventName) fetchPropertyValues(eventName, property)
  }
  const removeFilter = (stepIdx: number, filterIdx: number) =>
    setSteps((s) => s.map((st, i) => i === stepIdx ? { ...st, filters: st.filters.filter((_, fi) => fi !== filterIdx) } : st))
  const updateFilter = (stepIdx: number, filterIdx: number, field: 'property' | 'value', val: string) => {
    setSteps((s) => s.map((st, i) => i === stepIdx ? { ...st, filters: st.filters.map((f, fi) => fi === filterIdx ? { ...f, [field]: val } : f) } : st))
    if (field === 'property') {
      const eventName = steps[stepIdx]?.event_name
      if (eventName && val) fetchPropertyValues(eventName, val)
    }
  }

  const handleSave = async () => {
    if (!name.trim()) { setError('Name is required'); return }
    if (steps.some((s) => !s.event_name.trim())) { setError('All steps need an event name'); return }
    setSaving(true)
    setError(null)
    try {
      const apiSteps: FunnelStepInput[] = steps.map((s) => ({
        event_name: s.event_name,
        ...(s.filters.length > 0 ? { filters: s.filters.filter((f) => f.property && f.value) } : {}),
      }))
      const updated = await api.updateFunnel(projectId, funnel.id, { name, steps: apiSteps, scope })
      onUpdated(updated)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      background: 'rgba(0,0,0,0.7)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 500,
      padding: '1rem',
    }}>
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 14,
        padding: '2rem',
        width: '100%',
        maxWidth: 500,
        boxShadow: '0 25px 60px rgba(0,0,0,0.5)',
        maxHeight: '90vh',
        overflowY: 'auto',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>Edit funnel</h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer' }}>
            <X size={18} />
          </button>
        </div>

        {error && (
          <div style={{
            background: 'rgba(239,68,68,0.1)',
            border: `1px solid rgba(239,68,68,0.3)`,
            borderRadius: 8,
            padding: '0.6rem 0.875rem',
            color: C.error,
            fontSize: 14,
            marginBottom: '1rem',
          }}>
            {error}
          </div>
        )}

        <div style={{ marginBottom: '1.25rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>Funnel name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Signup flow"
            style={{
              width: '100%',
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              padding: '0.6rem 0.875rem',
              color: C.text,
              fontSize: 14,
              boxSizing: 'border-box',
              outline: 'none',
            }}
            onFocus={(e) => (e.target.style.borderColor = C.amber)}
            onBlur={(e) => (e.target.style.borderColor = C.border)}
          />
        </div>

        <ScopeToggle value={scope} onChange={setScope} />

        <AvailableEventsPanel
          eventNames={eventNames}
          onSelect={(eName) => {
            const emptyIdx = steps.findIndex((s) => !s.event_name)
            if (emptyIdx >= 0) {
              updateStep(emptyIdx, eName)
            } else {
              setSteps((prev) => [...prev, { event_name: eName, filters: [] }])
              fetchProperties(steps.length, eName)
            }
          }}
        />

        <div style={{ marginBottom: '1.25rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 10 }}>Steps</label>
          {steps.map((s, i) => (
            <div key={i} style={{ marginBottom: 10 }}>
              <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <div style={{
                  width: 24,
                  height: 24,
                  background: C.amber,
                  color: '#0f1117',
                  borderRadius: '50%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 12,
                  fontWeight: 800,
                  flexShrink: 0,
                }}>
                  {i + 1}
                </div>
                <EventNameInput
                  value={s.event_name}
                  onChange={(val) => updateStep(i, val)}
                  eventNames={eventNames}
                />
                {steps.length > 1 && (
                  <button
                    onClick={() => removeStep(i)}
                    style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer', padding: 4 }}
                  >
                    <X size={14} />
                  </button>
                )}
              </div>
              <div style={{ marginLeft: 32, marginTop: 4 }}>
                {(stepProperties[i] || []).length > 0 && (
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginBottom: 4 }}>
                    {(stepProperties[i] || [])
                      .filter((p) => !s.filters.some((f) => f.property === p))
                      .map((prop) => (
                        <button
                          key={prop}
                          onClick={() => addFilterWithProperty(i, prop)}
                          style={{
                            background: 'rgba(148,163,184,0.08)',
                            border: `1px solid ${C.border}`,
                            borderRadius: 4,
                            color: C.muted,
                            padding: '0.15rem 0.4rem',
                            fontSize: 11,
                            cursor: 'pointer',
                          }}
                          onMouseEnter={(e) => {
                            e.currentTarget.style.borderColor = C.amber
                            e.currentTarget.style.color = C.amber
                          }}
                          onMouseLeave={(e) => {
                            e.currentTarget.style.borderColor = C.border
                            e.currentTarget.style.color = C.muted
                          }}
                        >
                          + {prop}
                        </button>
                      ))}
                  </div>
                )}
                {s.filters.map((f, fi) => (
                  <div key={fi} style={{ display: 'flex', gap: 4, alignItems: 'center', marginBottom: 4 }}>
                    <AutocompleteInput
                      value={f.property}
                      onChange={(val) => updateFilter(i, fi, 'property', val)}
                      placeholder="Property"
                      suggestions={stepProperties[i] || []}
                      width={120}
                    />
                    <AutocompleteInput
                      value={f.value}
                      onChange={(val) => updateFilter(i, fi, 'value', val)}
                      placeholder="Value"
                      suggestions={filterValues[`${s.event_name}:${f.property}`] || []}
                      width={120}
                    />
                    <button
                      onClick={() => removeFilter(i, fi)}
                      style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer', padding: 2 }}
                    >
                      <X size={12} />
                    </button>
                  </div>
                ))}
                <button
                  onClick={() => addFilter(i)}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: C.muted,
                    cursor: 'pointer',
                    fontSize: 12,
                    padding: '2px 0',
                  }}
                >
                  + Add filter
                </button>
              </div>
            </div>
          ))}
          <button
            onClick={addStep}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              background: 'transparent',
              border: `1px dashed ${C.border}`,
              borderRadius: 8,
              color: C.muted,
              padding: '0.5rem 1rem',
              cursor: 'pointer',
              fontSize: 13,
              width: '100%',
              justifyContent: 'center',
              marginTop: 4,
            }}
          >
            <Plus size={14} /> Add step
          </button>
        </div>

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button
            onClick={onClose}
            style={{
              background: 'transparent',
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              color: C.muted,
              padding: '0.6rem 1.25rem',
              cursor: 'pointer',
              fontSize: 14,
            }}
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            style={{
              background: saving ? '#78481a' : C.amber,
              border: 'none',
              borderRadius: 8,
              color: '#0f1117',
              padding: '0.6rem 1.25rem',
              cursor: saving ? 'not-allowed' : 'pointer',
              fontSize: 14,
              fontWeight: 700,
            }}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── Segment Manager ──────────────────────────────────────────────────────────

function SegmentManager({
  projectId,
  segments,
  onSegmentsChange,
}: {
  projectId: string
  segments: Segment[]
  onSegmentsChange: (segs: Segment[]) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  const [newName, setNewName] = useState('')
  const [newRules, setNewRules] = useState<SegmentRule[]>([{ field: 'country_code', operator: 'eq', value: '' }])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  const addRule = () =>
    setNewRules((r) => [...r, { field: 'country_code', operator: 'eq', value: '' }])

  const removeRule = (i: number) =>
    setNewRules((r) => r.filter((_, idx) => idx !== i))

  const updateRule = <K extends keyof SegmentRule>(i: number, field: K, val: SegmentRule[K]) =>
    setNewRules((r) => r.map((rule, idx) => idx === i ? { ...rule, [field]: val } : rule))

  const handleCreate = async () => {
    if (!newName.trim()) { setError('Name is required'); return }
    setSaving(true)
    setError(null)
    try {
      const seg = await api.createSegment(projectId, newName, newRules)
      onSegmentsChange([...segments, seg])
      setNewName('')
      setNewRules([{ field: 'country_code', operator: 'eq', value: '' }])
      setShowCreate(false)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (seg: Segment) => {
    if (!window.confirm(`Delete segment "${seg.name}"?`)) return
    setDeletingId(seg.id)
    try {
      await api.deleteSegment(projectId, seg.id)
      onSegmentsChange(segments.filter((s) => s.id !== seg.id))
    } catch (e) {
      alert('Failed to delete: ' + String(e))
    } finally {
      setDeletingId(null)
    }
  }

  const valueHidden = (op: SegmentOperator) => op === 'is_null' || op === 'is_not_null'

  return (
    <div style={{
      marginTop: '2rem',
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
    }}>
      {/* Header */}
      <button
        onClick={() => setExpanded((e) => !e)}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          width: '100%',
          background: 'none',
          border: 'none',
          padding: '1rem 1.25rem',
          cursor: 'pointer',
          color: C.text,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {expanded ? <ChevronDown size={16} color={C.muted} /> : <ChevronRight size={16} color={C.muted} />}
          <span style={{ fontWeight: 700, fontSize: 15 }}>Segments</span>
          {segments.length > 0 && (
            <span style={{
              background: 'rgba(245,158,11,0.15)',
              color: C.amber,
              borderRadius: 99,
              padding: '0.1rem 0.5rem',
              fontSize: 12,
              fontWeight: 600,
            }}>
              {segments.length}
            </span>
          )}
        </div>
        <span style={{ fontSize: 12, color: C.muted }}>Custom segments for funnel analysis</span>
      </button>

      {expanded && (
        <div style={{ padding: '0 1.25rem 1.25rem' }}>
          {/* Existing segments */}
          {segments.length === 0 && !showCreate && (
            <p style={{ color: C.muted, fontSize: 13, margin: '0 0 1rem' }}>No segments yet.</p>
          )}
          {segments.map((seg) => (
            <div
              key={seg.id}
              style={{
                background: C.bg,
                border: `1px solid ${C.border}`,
                borderRadius: 8,
                padding: '0.75rem 1rem',
                marginBottom: 8,
              }}
            >
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontWeight: 700, fontSize: 14, color: C.text, marginBottom: 2 }}>{seg.name}</div>
                  <div style={{
                    fontFamily: 'ui-monospace, monospace',
                    fontSize: 10,
                    color: C.muted,
                    marginBottom: 6,
                    wordBreak: 'break-all',
                  }}>
                    ID: {seg.id}
                  </div>
                  {seg.rules.map((r, i) => (
                    <div key={i} style={{ fontSize: 12, color: C.muted, marginBottom: 2 }}>
                      <span style={{ color: C.text }}>{SEGMENT_FIELD_META[r.field]?.label ?? r.field}</span>
                      {' '}
                      <span style={{ color: C.amber }}>
                        {SEGMENT_OPERATORS.find((o) => o.value === r.operator)?.label ?? r.operator}
                      </span>
                      {!valueHidden(r.operator) && (
                        <> <span style={{ color: C.text, fontFamily: '"SF Mono","Fira Code",monospace' }}>{r.value}</span></>
                      )}
                    </div>
                  ))}
                </div>
                <button
                  onClick={() => handleDelete(seg)}
                  disabled={deletingId === seg.id}
                  title="Delete segment"
                  style={{
                    background: 'none',
                    border: `1px solid rgba(239,68,68,0.3)`,
                    borderRadius: 6,
                    color: C.error,
                    cursor: deletingId === seg.id ? 'not-allowed' : 'pointer',
                    padding: '0.3rem 0.5rem',
                    fontSize: 12,
                    flexShrink: 0,
                    opacity: deletingId === seg.id ? 0.5 : 1,
                  }}
                >
                  <Trash2 size={12} />
                </button>
              </div>
            </div>
          ))}

          {/* Starter templates — shown only when there are no segments yet */}
          {segments.length === 0 && !showCreate && (
            <div style={{ marginBottom: '1rem' }}>
              <div style={{ fontSize: 12, color: C.muted, marginBottom: 8, fontWeight: 600 }}>
                Quick-start templates
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))', gap: 6 }}>
                {SEGMENT_TEMPLATES.map((tpl) => (
                  <button
                    key={tpl.name}
                    onClick={() => {
                      setNewName(tpl.name)
                      setNewRules(tpl.rules.map((r) => ({ ...r })))
                      setShowCreate(true)
                    }}
                    style={{
                      background: C.bg,
                      border: `1px solid ${C.border}`,
                      borderRadius: 8,
                      padding: '0.6rem 0.75rem',
                      textAlign: 'left',
                      cursor: 'pointer',
                      transition: 'border-color 0.15s',
                    }}
                    onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.borderColor = 'rgba(245,158,11,0.5)')}
                    onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.borderColor = C.border)}
                  >
                    <div style={{ fontSize: 13, fontWeight: 600, color: C.text, marginBottom: 2 }}>{tpl.name}</div>
                    <div style={{ fontSize: 11, color: C.muted }}>{tpl.description}</div>
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Create new segment */}
          {showCreate && (
            <div style={{
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 8,
              padding: '1rem',
              marginBottom: 8,
            }}>
              {error && (
                <div style={{
                  background: 'rgba(239,68,68,0.1)',
                  border: `1px solid rgba(239,68,68,0.3)`,
                  borderRadius: 6,
                  padding: '0.5rem 0.75rem',
                  color: C.error,
                  fontSize: 13,
                  marginBottom: '0.75rem',
                }}>
                  {error}
                </div>
              )}
              <div style={{ marginBottom: '0.75rem' }}>
                <label style={{ display: 'block', fontSize: 12, color: C.muted, marginBottom: 4 }}>Name</label>
                <input
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="e.g. EU mobile users"
                  autoFocus
                  style={{
                    width: '100%',
                    background: C.surface,
                    border: `1px solid ${C.border}`,
                    borderRadius: 6,
                    padding: '0.45rem 0.75rem',
                    color: C.text,
                    fontSize: 13,
                    boxSizing: 'border-box',
                    outline: 'none',
                  }}
                  onFocus={(e) => (e.target.style.borderColor = C.amber)}
                  onBlur={(e) => (e.target.style.borderColor = C.border)}
                />
              </div>
              <div style={{ marginBottom: '0.75rem' }}>
                <label style={{ display: 'block', fontSize: 12, color: C.muted, marginBottom: 6 }}>Rules</label>
                {newRules.map((rule, i) => {
                  const meta = SEGMENT_FIELD_META[rule.field]
                  return (
                    <div key={i} style={{ marginBottom: 10 }}>
                      <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>
                        <select
                          value={rule.field}
                          onChange={(e) => updateRule(i, 'field', e.target.value)}
                          style={{
                            background: C.surface,
                            border: `1px solid ${C.border}`,
                            borderRadius: 6,
                            color: C.text,
                            padding: '0.3rem 0.5rem',
                            fontSize: 12,
                            outline: 'none',
                            cursor: 'pointer',
                          }}
                        >
                          {SEGMENT_FIELDS.map((f) => (
                            <option key={f} value={f}>{SEGMENT_FIELD_META[f]?.label ?? f}</option>
                          ))}
                        </select>
                        <select
                          value={rule.operator}
                          onChange={(e) => updateRule(i, 'operator', e.target.value as SegmentOperator)}
                          style={{
                            background: C.surface,
                            border: `1px solid ${C.border}`,
                            borderRadius: 6,
                            color: C.text,
                            padding: '0.3rem 0.5rem',
                            fontSize: 12,
                            outline: 'none',
                            cursor: 'pointer',
                          }}
                        >
                          {SEGMENT_OPERATORS.map((op) => (
                            <option key={op.value} value={op.value}>{op.label}</option>
                          ))}
                        </select>
                        {!valueHidden(rule.operator) && (
                          <input
                            value={rule.value}
                            onChange={(e) => updateRule(i, 'value', e.target.value)}
                            placeholder={meta?.placeholder ?? 'Value'}
                            style={{
                              background: C.surface,
                              border: `1px solid ${C.border}`,
                              borderRadius: 6,
                              color: C.text,
                              padding: '0.3rem 0.5rem',
                              fontSize: 12,
                              outline: 'none',
                              minWidth: 120,
                              flex: 1,
                            }}
                            onFocus={(e) => (e.target.style.borderColor = C.amber)}
                            onBlur={(e) => (e.target.style.borderColor = C.border)}
                          />
                        )}
                        {newRules.length > 1 && (
                          <button
                            onClick={() => removeRule(i)}
                            style={{ background: 'none', border: 'none', color: C.muted, cursor: 'pointer', padding: 2 }}
                          >
                            <X size={12} />
                          </button>
                        )}
                      </div>
                      {meta && (
                        <div style={{ fontSize: 11, color: C.muted, marginTop: 3, paddingLeft: 2 }}>
                          {meta.hint}
                        </div>
                      )}
                    </div>
                  )
                })}
                <button
                  onClick={addRule}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: C.muted,
                    cursor: 'pointer',
                    fontSize: 12,
                    padding: '2px 0',
                  }}
                >
                  + Add rule
                </button>
              </div>
              <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
                <button
                  onClick={() => { setShowCreate(false); setError(null) }}
                  style={{
                    background: 'transparent',
                    border: `1px solid ${C.border}`,
                    borderRadius: 6,
                    color: C.muted,
                    padding: '0.4rem 0.875rem',
                    cursor: 'pointer',
                    fontSize: 13,
                  }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleCreate}
                  disabled={saving}
                  style={{
                    background: saving ? '#78481a' : C.amber,
                    border: 'none',
                    borderRadius: 6,
                    color: '#0f1117',
                    padding: '0.4rem 0.875rem',
                    cursor: saving ? 'not-allowed' : 'pointer',
                    fontSize: 13,
                    fontWeight: 700,
                  }}
                >
                  {saving ? 'Saving…' : 'Save segment'}
                </button>
              </div>
            </div>
          )}

          {!showCreate && (
            <button
              onClick={() => setShowCreate(true)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                background: 'transparent',
                border: `1px dashed ${C.border}`,
                borderRadius: 8,
                color: C.muted,
                padding: '0.5rem 1rem',
                cursor: 'pointer',
                fontSize: 13,
                width: '100%',
                justifyContent: 'center',
              }}
            >
              <Plus size={14} /> New segment
            </button>
          )}
        </div>
      )}
    </div>
  )
}

// ─── Funnel Detail ────────────────────────────────────────────────────────────

function FunnelDetail({ analysis, activeSegment, apiKey }: { analysis: FunnelAnalysis; activeSegment: string; apiKey?: string }) {
  const hasData = analysis.results && analysis.results.length > 0 && analysis.results[0]?.count > 0

  if (!hasData) {
    return (
      <div>
        <div style={{ padding: '2rem', textAlign: 'center', color: C.muted, maxWidth: '100%', overflow: 'hidden' }}>
          <div style={{ fontSize: 40, marginBottom: 16 }}>📭</div>
          <div style={{ fontWeight: 700, fontSize: 16, color: C.text, marginBottom: 8, wordBreak: 'break-word' }}>
            No data yet for "{analysis.funnel?.name}"
          </div>
          <div style={{ fontSize: 14, lineHeight: 1.6, maxWidth: '100%', wordBreak: 'break-word' }}>
            Start sending events using your tracking snippet.<br />
            This funnel tracks: {analysis.funnel?.steps?.map(s => s.event_name).filter(Boolean).join(' → ') || '(no steps defined)'}
          </div>
        </div>
        {analysis.funnel && <ImplementationSnippet funnel={analysis.funnel} apiKey={apiKey} />}
      </div>
    )
  }

  const maxCount = Math.max(...(analysis.results?.map((r) => r.count) ?? [1]))
  const segLabel = PRESET_SEGMENTS.find((s) => s.id === activeSegment)?.label ?? activeSegment
  const entryCount = analysis.results?.[0]?.count ?? 0

  return (
    <div>
      <h2 style={{ fontSize: 20, fontWeight: 800, marginBottom: 4 }}>{analysis.funnel.name}</h2>
      <p style={{ color: C.muted, fontSize: 13, marginBottom: '0.75rem' }}>
        {analysis.from?.slice(0, 10)} → {analysis.to?.slice(0, 10)}
      </p>
      {activeSegment !== 'all' && (
        <p style={{ color: C.amber, fontSize: 13, marginBottom: '1.25rem', fontWeight: 500 }}>
          Showing {entryCount.toLocaleString()} sessions · {segLabel} segment
        </p>
      )}
      {activeSegment === 'all' && (
        <p style={{ color: C.muted, fontSize: 13, marginBottom: '1.25rem' }}>
          {entryCount.toLocaleString()} sessions total
        </p>
      )}

      {analysis.results?.map((step, i) => (
        <div key={step.step_order} style={{ marginBottom: '1.25rem' }}>
          {/* Drop-off indicator between steps */}
          {i > 0 && step.drop_off > 0 && (
            <div style={{
              fontSize: 12,
              color: C.error,
              marginBottom: 6,
              paddingLeft: 40,
              display: 'flex',
              alignItems: 'center',
              gap: 4,
            }}>
              <span>▼</span>
              {(step.drop_off * 100).toFixed(1)}% dropped off
            </div>
          )}

          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{
              width: 32,
              height: 32,
              background: conversionColor(step.conversion),
              color: '#0f1117',
              borderRadius: '50%',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontWeight: 800,
              fontSize: 13,
              flexShrink: 0,
            }}>
              {i + 1}
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 14, fontWeight: 600, color: C.text, marginBottom: 6 }}>
                {step.event_name}
              </div>
              <div style={{ height: 20, background: '#2a2d3a', borderRadius: 4, overflow: 'hidden', position: 'relative' }}>
                <div style={{
                  position: 'absolute',
                  inset: 0,
                  width: `${(step.count / maxCount) * 100}%`,
                  background: conversionColor(step.conversion),
                  borderRadius: 4,
                  display: 'flex',
                  alignItems: 'center',
                  paddingLeft: 8,
                  transition: 'width 0.5s',
                }}>
                  <span style={{ fontSize: 11, fontWeight: 700, color: '#0f1117', whiteSpace: 'nowrap' }}>
                    {(step.conversion * 100).toFixed(1)}%
                  </span>
                </div>
              </div>
            </div>
            <div style={{ textAlign: 'right', minWidth: 70 }}>
              <div style={{ fontSize: 18, fontWeight: 800, color: C.text }}>{step.count.toLocaleString()}</div>
              <div style={{ fontSize: 12, color: C.muted }}>users</div>
            </div>
          </div>
        </div>
      ))}

      <ImplementationSnippet funnel={analysis.funnel} apiKey={apiKey} />
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function Funnels() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const [funnels, setFunnels] = useState<Funnel[]>([])
  const [segments, setSegments] = useState<Segment[]>([])
  const [selected, setSelected] = useState<Funnel | null>(null)
  const [analysis, setAnalysis] = useState<FunnelAnalysis | null>(null)
  const [analysisLoading, setAnalysisLoading] = useState(false)
  const [analysisError, setAnalysisError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [showEdit, setShowEdit] = useState(false)
  const [showDetail, setShowDetail] = useState(false)
  const [activeSegment, setActiveSegment] = useState('all')
  // activeSegmentId is set when a stored segment is chosen; activeSegment stays 'all' in that case
  const [activeSegmentId, setActiveSegmentId] = useState<string | undefined>(undefined)
  const [ingestKey, setIngestKey] = useState<string | undefined>(undefined)
  const [deleting, setDeleting] = useState(false)
  const [pendingTemplate, setPendingTemplate] = useState<FunnelTemplate | undefined>(undefined)

  useEffect(() => {
    api.listApiKeys()
      .then((d) => {
        const key = (d.api_keys || []).find((k) => k.scope === 'ingest')
        if (key) setIngestKey(key.id)
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    if (!projectId) return
    setSelected(null)
    setAnalysis(null)
    setAnalysisError(null)
    setShowDetail(false)
    setActiveSegment('all')
    setActiveSegmentId(undefined)
    api.listFunnels(projectId)
      .then((d) => setFunnels(d.funnels || []))
      .catch((e) => { console.error(e); reportError(e, { source: 'Funnels.listFunnels' }) })
    api.listSegments(projectId)
      .then((d) => setSegments(d.segments || []))
      .catch(() => {})
  }, [projectId])

  // Re-fetch analysis whenever selected funnel or active segment changes.
  useEffect(() => {
    if (!selected || !projectId) return
    setAnalysis(null)
    setAnalysisError(null)
    setAnalysisLoading(true)
    api.getFunnelAnalysis(projectId, selected.id, activeSegmentId ? undefined : activeSegment, activeSegmentId)
      .then(setAnalysis)
      .catch((e) => {
        console.error(e)
        const isNoise = e instanceof ApiError && (e.status === 404 || e.status === 0)
        if (!isNoise) reportError(e, { source: 'Funnels.getFunnelAnalysis' })
        setAnalysisError(String(e))
      })
      .finally(() => setAnalysisLoading(false))
  }, [selected, projectId, activeSegment, activeSegmentId])

  const loadAnalysis = (funnel: Funnel) => {
    if (!projectId) return
    trackEvent('funnel_viewed', { funnel_name: funnel.name })
    setSelected(funnel)
    setShowDetail(true)
    // Analysis will be fetched by the effect above.
  }

  const handleDeleteFunnel = async () => {
    if (!selected || !projectId) return
    if (!window.confirm(`Delete funnel "${selected.name}"? This cannot be undone.`)) return
    setDeleting(true)
    try {
      await api.deleteFunnel(projectId, selected.id)
      setFunnels((prev) => prev.filter((f) => f.id !== selected.id))
      setSelected(null)
      setAnalysis(null)
      setShowDetail(false)
    } catch (e) {
      console.error(e)
      reportError(e, { source: 'Funnels.deleteFunnel' })
      alert('Failed to delete funnel: ' + String(e))
    } finally {
      setDeleting(false)
    }
  }

  // Segment selection — preset vs stored
  const handleSegmentSelect = (presetId: string) => {
    setActiveSegment(presetId)
    setActiveSegmentId(undefined)
  }
  const handleStoredSegmentSelect = (seg: Segment) => {
    setActiveSegment('all') // reset preset
    setActiveSegmentId(seg.id)
  }

  // Determine label for the currently active segment in FunnelDetail
  const activeSegmentLabel: string = activeSegmentId
    ? (segments.find((s) => s.id === activeSegmentId)?.name ?? activeSegmentId)
    : activeSegment

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, padding: '2rem', textAlign: 'center' }}>
          Select a project to view funnels.
        </div>
      </Shell>
    )
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <style>{`
        @media (max-width: 767px) {
          .funnels-layout { grid-template-columns: 1fr !important; }
          .funnels-list.detail-open { display: none !important; }
          .funnels-detail.no-selection { display: none !important; }
          .back-btn { display: block !important; }
        }
        @media (min-width: 768px) {
          .back-btn { display: none !important; }
        }
        .funnels-detail {
          min-width: 0;
          max-width: 100%;
          box-sizing: border-box;
        }
        .funnels-detail > * {
          max-width: 100%;
          box-sizing: border-box;
        }
      `}</style>

      {showCreate && projectId && (
        <CreateFunnelModal
          projectId={projectId}
          initialTemplate={pendingTemplate}
          onClose={() => { setShowCreate(false); setPendingTemplate(undefined) }}
          onCreated={(f) => {
            setFunnels((prev) => [...prev, f])
            setShowCreate(false)
            setPendingTemplate(undefined)
            loadAnalysis(f)
          }}
        />
      )}

      {showEdit && selected && projectId && (
        <EditFunnelModal
          projectId={projectId}
          funnel={selected}
          onClose={() => setShowEdit(false)}
          onUpdated={(updated) => {
            setFunnels((prev) => prev.map((f) => f.id === updated.id ? updated : f))
            setSelected(updated)
            setShowEdit(false)
            setAnalysis(null)
          }}
        />
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem', gap: 8, flexWrap: 'wrap' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Funnels</h1>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <TemplatesButton
            onSelect={(tpl) => {
              setPendingTemplate(tpl)
              setShowCreate(true)
            }}
          />
          <button
            onClick={() => { setPendingTemplate(undefined); setShowCreate(true) }}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              background: C.amber,
              border: 'none',
              borderRadius: 8,
              color: '#0f1117',
              padding: '0.6rem 1.1rem',
              cursor: 'pointer',
              fontSize: 14,
              fontWeight: 700,
            }}
          >
            <Plus size={16} />
            Create funnel
          </button>
        </div>
      </div>

      <div className="funnels-layout" style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '1.5rem' }}>
        {/* Funnel list */}
        <div className={`funnels-list${showDetail ? ' detail-open' : ''}`}>
          {funnels.length === 0 ? (
            <div style={{
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 12,
              padding: '2rem',
              textAlign: 'center',
              color: C.muted,
            }}>
              <Layers size={32} style={{ opacity: 0.3, marginBottom: 8 }} />
              <div style={{ fontSize: 14 }}>No funnels yet</div>
              <button
                onClick={() => setShowCreate(true)}
                style={{
                  marginTop: 12,
                  background: 'transparent',
                  border: `1px solid ${C.border}`,
                  borderRadius: 7,
                  color: C.amber,
                  padding: '0.4rem 0.8rem',
                  cursor: 'pointer',
                  fontSize: 13,
                  fontWeight: 600,
                }}
              >
                Create one
              </button>
            </div>
          ) : (
            funnels.map((f) => (
              <div
                key={f.id}
                onClick={() => loadAnalysis(f)}
                style={{
                  padding: '0.875rem 1rem',
                  marginBottom: '0.5rem',
                  cursor: 'pointer',
                  background: selected?.id === f.id ? 'rgba(245,158,11,0.1)' : C.surface,
                  border: `1px solid ${selected?.id === f.id ? 'rgba(245,158,11,0.4)' : C.border}`,
                  borderRadius: 10,
                  transition: 'all 0.15s',
                }}
              >
                <div style={{ fontWeight: 700, fontSize: 14, color: C.text }}>{f.name}</div>
                <div style={{ fontSize: 12, color: C.muted, marginTop: 3 }}>
                  {f.steps?.length ?? 0} steps
                  {f.scope && f.scope !== 'session' && (
                    <span style={{
                      marginLeft: 6,
                      background: 'rgba(245,158,11,0.12)',
                      color: C.amber,
                      borderRadius: 4,
                      padding: '0.05rem 0.35rem',
                      fontSize: 10,
                      fontWeight: 600,
                    }}>
                      page view
                    </span>
                  )}
                </div>
              </div>
            ))
          )}
        </div>

        {/* Analysis panel */}
        <div className={`funnels-detail${!selected ? ' no-selection' : ''}`} style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '1.5rem',
          minHeight: 300,
          minWidth: 0,
          overflow: 'hidden',
          maxWidth: '100%',
          boxSizing: 'border-box',
        }}>
          <button
            className="back-btn"
            onClick={() => setShowDetail(false)}
            style={{
              display: 'none',
              background: 'none',
              border: 'none',
              color: C.amber,
              cursor: 'pointer',
              fontSize: 14,
              fontWeight: 600,
              padding: '0 0 1rem 0',
            }}
          >
            ← Back
          </button>
          {!selected && (
            <div style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: 300,
              color: C.muted,
              gap: 8,
            }}>
              <Layers size={40} opacity={0.3} />
              <div>Select a funnel to view analysis</div>
            </div>
          )}
          {selected && (
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1rem', gap: 8, flexWrap: 'wrap' }}>
              <div style={{ fontWeight: 700, fontSize: 16, color: C.text, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {selected.name}
              </div>
              <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
                <button
                  onClick={() => setShowEdit(true)}
                  title="Edit funnel"
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    background: 'transparent',
                    border: `1px solid ${C.border}`,
                    borderRadius: 7,
                    color: C.muted,
                    padding: '0.35rem 0.7rem',
                    cursor: 'pointer',
                    fontSize: 13,
                  }}
                >
                  <Pencil size={13} /> Edit
                </button>
                <button
                  onClick={handleDeleteFunnel}
                  disabled={deleting}
                  title="Delete funnel"
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    background: 'transparent',
                    border: `1px solid rgba(239,68,68,0.4)`,
                    borderRadius: 7,
                    color: C.error,
                    padding: '0.35rem 0.7rem',
                    cursor: deleting ? 'not-allowed' : 'pointer',
                    fontSize: 13,
                    opacity: deleting ? 0.5 : 1,
                  }}
                >
                  <Trash2 size={13} /> {deleting ? 'Deleting…' : 'Delete'}
                </button>
              </div>
            </div>
          )}
          {selected && (
            <div style={{ marginBottom: '1.25rem' }}>
              {/* Preset segment pills */}
              <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', maxWidth: '100%', boxSizing: 'border-box', marginBottom: segments.length > 0 ? 6 : 0 }}>
                {PRESET_SEGMENTS.map((seg) => {
                  const active = !activeSegmentId && activeSegment === seg.id
                  return (
                    <button
                      key={seg.id}
                      onClick={() => handleSegmentSelect(seg.id)}
                      title={seg.tip}
                      style={{
                        padding: '0.35rem 0.875rem',
                        borderRadius: 99,
                        border: `1px solid ${active ? C.amber : C.border}`,
                        background: active ? 'rgba(245,158,11,0.15)' : 'transparent',
                        color: active ? C.amber : C.muted,
                        cursor: 'pointer',
                        fontSize: 13,
                        fontWeight: active ? 600 : 400,
                        transition: 'all 0.15s',
                      }}
                    >
                      {seg.label}
                    </button>
                  )
                })}
              </div>
              {/* Stored segment pills */}
              {segments.length > 0 && (
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', maxWidth: '100%', boxSizing: 'border-box' }}>
                  {segments.map((seg) => {
                    const active = activeSegmentId === seg.id
                    return (
                      <button
                        key={seg.id}
                        onClick={() => handleStoredSegmentSelect(seg)}
                        title={`Segment: ${seg.name}`}
                        style={{
                          padding: '0.35rem 0.875rem',
                          borderRadius: 99,
                          border: `1px solid ${active ? C.amber : C.border}`,
                          background: active ? 'rgba(245,158,11,0.15)' : 'rgba(245,158,11,0.04)',
                          color: active ? C.amber : C.muted,
                          cursor: 'pointer',
                          fontSize: 13,
                          fontWeight: active ? 600 : 400,
                          transition: 'all 0.15s',
                        }}
                      >
                        {seg.name}
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          )}
          {analysisLoading && (
            <div style={{ color: C.muted, padding: '2rem' }}>Loading analysis…</div>
          )}
          {analysisError && !analysisLoading && (
            <div style={{
              background: 'rgba(239,68,68,0.1)',
              border: `1px solid rgba(239,68,68,0.3)`,
              borderRadius: 8,
              padding: '1rem 1.25rem',
              color: C.error,
              fontSize: 14,
            }}>
              Failed to load analysis: {analysisError}
            </div>
          )}
          {analysis && !analysisLoading && !analysisError && (
            <FunnelDetail analysis={analysis} activeSegment={activeSegmentLabel} apiKey={ingestKey} />
          )}
        </div>
      </div>

      {/* Segment manager */}
      {projectId && (
        <SegmentManager
          projectId={projectId}
          segments={segments}
          onSegmentsChange={setSegments}
        />
      )}
    </Shell>
  )
}
