import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Plus, X, Layers, Pencil, Trash2 } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api, Funnel, FunnelAnalysis, FunnelStepInput } from '../lib/api'
import { useProjects } from '../lib/projects'

const LANGS = ['JS', 'React', 'Go', 'Python', 'Swift', 'Kotlin'] as const
type Lang = typeof LANGS[number]

function ImplementationSnippet({ funnel, apiKey }: { funnel: Funnel; apiKey?: string }) {
  const [copied, setCopied] = useState(false)
  const [activeLang, setActiveLang] = useState<Lang>('JS')
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
<script src="https://funnelbarn.wiebe.xyz/sdk.js"
        data-api-key="${key}"></script>

<script>
// Funnel step constants
const ${constName} = {
${stepEntries}
}

// Track page view on load
funnelbarn.track(${constName}.${toCamelCase(firstStep)})

// Button click example
document.querySelector('#your-cta').addEventListener('click', () => {
  funnelbarn.track(${constName}.${toCamelCase(midStep)})
})

// Form submit example
document.querySelector('form').addEventListener('submit', () => {
  funnelbarn.track(${constName}.${toCamelCase(lastStep)})
})

// Scroll depth (50%)
let _scrolled = false
window.addEventListener('scroll', () => {
  const pct = window.scrollY / (document.body.scrollHeight - window.innerHeight)
  if (pct >= 0.5 && !_scrolled) { _scrolled = true; funnelbarn.track('scroll_50') }
}, { passive: true })
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

  useEffect(() => {
    track(${constName}.${toCamelCase(firstStep)})
  }, [])

  return (
    <button onClick={() => track(${constName}.${toCamelCase(midStep)})}>
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
        Endpoint: "https://funnelbarn.wiebe.xyz",
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
    endpoint="https://funnelbarn.wiebe.xyz"
)

# Track a step
client.track(${constNameCapitalized}.${firstStep.toUpperCase()})

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

    var request = URLRequest(url: URL(string: "https://funnelbarn.wiebe.xyz/api/v1/events")!)
    request.httpMethod = "POST"
    request.setValue("application/json", forHTTPHeaderField: "Content-Type")
    request.setValue("${key}", forHTTPHeaderField: "X-FunnelBarn-Api-Key")
    request.httpBody = try? JSONSerialization.data(withJSONObject: body)

    URLSession.shared.dataTask(with: request).resume()
}

// Usage
trackFunnelStep(.${toCamelCase(firstStep)})
trackFunnelStep(.${toCamelCase(lastStep)}, properties: ["user_id": "user_123"])`

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
    private val endpoint = "https://funnelbarn.wiebe.xyz/api/v1/events"

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

// Usage
val tracker = FunnelBarnTracker("${key}")
tracker.track(${constNameCapitalized}.${firstStep.toUpperCase().replace(/-/g, '_')})
tracker.track(${constNameCapitalized}.${lastStep.toUpperCase().replace(/-/g, '_')},
    mapOf("user_id" to "user_123"))`
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
        WebkitOverflowScrolling: 'touch' as any,
      }}>
        {snippet}
      </pre>
    </div>
  )
}

const PRESET_SEGMENTS = [
  { id: 'all', label: 'All' },
  { id: 'logged_in', label: 'Logged in' },
  { id: 'not_logged_in', label: 'Not logged in' },
  { id: 'mobile', label: 'Mobile' },
  { id: 'desktop', label: 'Desktop' },
  { id: 'tablet', label: 'Tablet' },
  { id: 'new_visitor', label: 'New visitors' },
  { id: 'returning', label: 'Returning' },
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

function CreateFunnelModal({
  projectId,
  onClose,
  onCreated,
}: {
  projectId: string
  onClose: () => void
  onCreated: (f: Funnel) => void
}) {
  const [name, setName] = useState('')
  const [steps, setSteps] = useState<FunnelStepInput[]>([{ event_name: '' }, { event_name: '' }])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const addStep = () => setSteps((s) => [...s, { event_name: '' }])
  const removeStep = (i: number) => setSteps((s) => s.filter((_, idx) => idx !== i))
  const updateStep = (i: number, val: string) =>
    setSteps((s) => s.map((st, idx) => (idx === i ? { event_name: val } : st)))

  const handleCreate = async () => {
    if (!name.trim()) { setError('Name is required'); return }
    if (steps.some((s) => !s.event_name.trim())) { setError('All steps need an event name'); return }
    setSaving(true)
    setError(null)
    try {
      const f = await api.createFunnel(projectId, name, steps)
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

        <div style={{ marginBottom: '1.25rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 10 }}>Steps</label>
          {steps.map((s, i) => (
            <div key={i} style={{ display: 'flex', gap: 8, marginBottom: 8, alignItems: 'center' }}>
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
              <input
                value={s.event_name}
                onChange={(e) => updateStep(i, e.target.value)}
                placeholder={`Event name, e.g. page_view`}
                style={{
                  flex: 1,
                  background: C.bg,
                  border: `1px solid ${C.border}`,
                  borderRadius: 8,
                  padding: '0.55rem 0.875rem',
                  color: C.text,
                  fontSize: 14,
                  outline: 'none',
                }}
                onFocus={(e) => (e.target.style.borderColor = C.amber)}
                onBlur={(e) => (e.target.style.borderColor = C.border)}
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
  const [steps, setSteps] = useState<FunnelStepInput[]>(
    funnel.steps?.length > 0
      ? funnel.steps.map((s) => ({ event_name: s.event_name }))
      : [{ event_name: '' }, { event_name: '' }]
  )
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const addStep = () => setSteps((s) => [...s, { event_name: '' }])
  const removeStep = (i: number) => setSteps((s) => s.filter((_, idx) => idx !== i))
  const updateStep = (i: number, val: string) =>
    setSteps((s) => s.map((st, idx) => (idx === i ? { event_name: val } : st)))

  const handleSave = async () => {
    if (!name.trim()) { setError('Name is required'); return }
    if (steps.some((s) => !s.event_name.trim())) { setError('All steps need an event name'); return }
    setSaving(true)
    setError(null)
    try {
      const updated = await api.updateFunnel(projectId, funnel.id, { name, steps })
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

        <div style={{ marginBottom: '1.25rem' }}>
          <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 10 }}>Steps</label>
          {steps.map((s, i) => (
            <div key={i} style={{ display: 'flex', gap: 8, marginBottom: 8, alignItems: 'center' }}>
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
              <input
                value={s.event_name}
                onChange={(e) => updateStep(i, e.target.value)}
                placeholder={`Event name, e.g. page_view`}
                style={{
                  flex: 1,
                  background: C.bg,
                  border: `1px solid ${C.border}`,
                  borderRadius: 8,
                  padding: '0.55rem 0.875rem',
                  color: C.text,
                  fontSize: 14,
                  outline: 'none',
                }}
                onFocus={(e) => (e.target.style.borderColor = C.amber)}
                onBlur={(e) => (e.target.style.borderColor = C.border)}
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

export default function Funnels() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const [funnels, setFunnels] = useState<Funnel[]>([])
  const [selected, setSelected] = useState<Funnel | null>(null)
  const [analysis, setAnalysis] = useState<FunnelAnalysis | null>(null)
  const [analysisLoading, setAnalysisLoading] = useState(false)
  const [analysisError, setAnalysisError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [showEdit, setShowEdit] = useState(false)
  const [showDetail, setShowDetail] = useState(false)
  const [activeSegment, setActiveSegment] = useState('all')
  const [ingestKey, setIngestKey] = useState<string | undefined>(undefined)
  const [deleting, setDeleting] = useState(false)

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
    api.listFunnels(projectId)
      .then((d) => setFunnels(d.funnels || []))
      .catch(console.error)
  }, [projectId])

  // Re-fetch analysis whenever selected funnel or active segment changes.
  useEffect(() => {
    if (!selected || !projectId) return
    setAnalysis(null)
    setAnalysisError(null)
    setAnalysisLoading(true)
    api.getFunnelAnalysis(projectId, selected.id, activeSegment)
      .then(setAnalysis)
      .catch((e) => {
        console.error(e)
        setAnalysisError(String(e))
      })
      .finally(() => setAnalysisLoading(false))
  }, [selected, projectId, activeSegment])

  const loadAnalysis = (funnel: Funnel) => {
    if (!projectId) return
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
      alert('Failed to delete funnel: ' + String(e))
    } finally {
      setDeleting(false)
    }
  }

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
          onClose={() => setShowCreate(false)}
          onCreated={(f) => {
            setFunnels((prev) => [...prev, f])
            setShowCreate(false)
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
            // Re-trigger analysis by resetting and letting the effect pick it up.
            setAnalysis(null)
          }}
        />
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', margin: 0 }}>Funnels</h1>
        <button
          onClick={() => setShowCreate(true)}
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
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: '1.25rem', maxWidth: '100%', boxSizing: 'border-box' }}>
              {PRESET_SEGMENTS.map((seg) => (
                <button
                  key={seg.id}
                  onClick={() => setActiveSegment(seg.id)}
                  style={{
                    padding: '0.35rem 0.875rem',
                    borderRadius: 99,
                    border: `1px solid ${activeSegment === seg.id ? C.amber : C.border}`,
                    background: activeSegment === seg.id ? 'rgba(245,158,11,0.15)' : 'transparent',
                    color: activeSegment === seg.id ? C.amber : C.muted,
                    cursor: 'pointer',
                    fontSize: 13,
                    fontWeight: activeSegment === seg.id ? 600 : 400,
                    transition: 'all 0.15s',
                  }}
                >
                  {seg.label}
                </button>
              ))}
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
            <FunnelDetail analysis={analysis} activeSegment={activeSegment} apiKey={ingestKey} />
          )}
        </div>
      </div>
    </Shell>
  )
}
