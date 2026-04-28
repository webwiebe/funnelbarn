import { useState } from 'react'
import { api, Project, ApiKey } from '../lib/api'
import { Check, Copy } from 'lucide-react'

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

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  const handle = async () => {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <button
      onClick={handle}
      style={{
        background: copied ? 'rgba(16,185,129,0.1)' : C.bg,
        border: `1px solid ${copied ? 'rgba(16,185,129,0.4)' : C.border}`,
        borderRadius: 7,
        color: copied ? C.success : C.muted,
        padding: '0.4rem 0.75rem',
        cursor: 'pointer',
        display: 'flex',
        alignItems: 'center',
        gap: 5,
        fontSize: 13,
        transition: 'all 0.2s',
      }}
    >
      {copied ? <Check size={13} /> : <Copy size={13} />}
      {copied ? 'Copied!' : 'Copy'}
    </button>
  )
}

interface FirstRunWizardProps {
  onComplete: () => void
}

export default function FirstRunWizard({ onComplete }: FirstRunWizardProps) {
  const [step, setStep] = useState(1)
  const [projectName, setProjectName] = useState('')
  const [domain, setDomain] = useState('')
  const [createdProject, setCreatedProject] = useState<Project | null>(null)
  const [ingestKey, setIngestKey] = useState<string | null>(null)
  const [_createdKey, setCreatedKey] = useState<ApiKey | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCreateProject = async () => {
    if (!projectName.trim()) { setError('Project name is required'); return }
    setSaving(true)
    setError(null)
    try {
      const p = await api.createProject(projectName, domain)
      setCreatedProject(p)
      // Also create an ingest key automatically
      try {
        const keyRes = await api.createApiKey(`${projectName} ingest`, 'ingest')
        setIngestKey(keyRes.key)
        setCreatedKey(keyRes.api_key)
      } catch {
        // non-fatal
      }
      setStep(3)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  const snippet = ingestKey
    ? `<script src="https://funnelbarn.wiebe.xyz/sdk.js"\n        data-api-key="${ingestKey}"></script>`
    : `<script src="https://funnelbarn.wiebe.xyz/sdk.js"\n        data-api-key="fb_xxxxx"></script>`

  const totalSteps = 4

  const stepDots = Array.from({ length: totalSteps }, (_, i) => (
    <div
      key={i}
      style={{
        width: 8,
        height: 8,
        borderRadius: '50%',
        background: i + 1 <= step ? C.amber : C.border,
        transition: 'background 0.3s',
      }}
    />
  ))

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      background: 'rgba(0,0,0,0.85)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 1000,
      padding: '1rem',
    }}>
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 16,
        padding: '2.5rem',
        width: '100%',
        maxWidth: 520,
        boxShadow: '0 30px 80px rgba(0,0,0,0.6)',
      }}>
        {/* Step dots */}
        <div style={{ display: 'flex', gap: 6, justifyContent: 'center', marginBottom: '2rem' }}>
          {stepDots}
        </div>

        {/* Step 1: Welcome */}
        {step === 1 && (
          <div style={{ textAlign: 'center' }}>
            <div style={{ fontSize: 52, marginBottom: 12 }}>🐄</div>
            <h2 style={{ fontSize: 24, fontWeight: 800, margin: '0 0 12px', color: C.text }}>
              Welcome to FunnelBarn
            </h2>
            <p style={{ color: C.muted, fontSize: 15, lineHeight: 1.6, margin: '0 0 2rem' }}>
              Track user journeys, visualize drop-offs, and run A/B tests — all in one place.
              Let's get your barn set up in just a few steps.
            </p>
            <button
              onClick={() => setStep(2)}
              style={{
                background: C.amber,
                border: 'none',
                borderRadius: 10,
                color: '#0f1117',
                padding: '0.75rem 2rem',
                cursor: 'pointer',
                fontSize: 15,
                fontWeight: 700,
                width: '100%',
              }}
            >
              Let's go →
            </button>
          </div>
        )}

        {/* Step 2: Create project */}
        {step === 2 && (
          <div>
            <h2 style={{ fontSize: 20, fontWeight: 800, margin: '0 0 6px', color: C.text }}>
              Create your first project
            </h2>
            <p style={{ color: C.muted, fontSize: 14, margin: '0 0 1.5rem' }}>
              A project groups all analytics for one website or app.
            </p>

            {error && (
              <div style={{
                background: 'rgba(239,68,68,0.1)',
                border: `1px solid rgba(239,68,68,0.3)`,
                borderRadius: 8,
                padding: '0.6rem 0.875rem',
                color: C.error,
                fontSize: 13,
                marginBottom: '1rem',
              }}>
                {error}
              </div>
            )}

            <div style={{ marginBottom: '1rem' }}>
              <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>
                Project name *
              </label>
              <input
                value={projectName}
                onChange={(e) => setProjectName(e.target.value)}
                placeholder="e.g. My SaaS App"
                autoFocus
                style={{
                  width: '100%',
                  boxSizing: 'border-box',
                  background: C.bg,
                  border: `1px solid ${C.border}`,
                  borderRadius: 8,
                  padding: '0.65rem 0.875rem',
                  color: C.text,
                  fontSize: 14,
                  outline: 'none',
                }}
                onFocus={(e) => (e.target.style.borderColor = C.amber)}
                onBlur={(e) => (e.target.style.borderColor = C.border)}
                onKeyDown={(e) => e.key === 'Enter' && handleCreateProject()}
              />
            </div>

            <div style={{ marginBottom: '1.5rem' }}>
              <label style={{ display: 'block', fontSize: 13, color: C.muted, marginBottom: 6 }}>
                Domain (optional)
              </label>
              <input
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                placeholder="e.g. myapp.com"
                style={{
                  width: '100%',
                  boxSizing: 'border-box',
                  background: C.bg,
                  border: `1px solid ${C.border}`,
                  borderRadius: 8,
                  padding: '0.65rem 0.875rem',
                  color: C.text,
                  fontSize: 14,
                  outline: 'none',
                }}
                onFocus={(e) => (e.target.style.borderColor = C.amber)}
                onBlur={(e) => (e.target.style.borderColor = C.border)}
              />
            </div>

            <div style={{ display: 'flex', gap: 8 }}>
              <button
                onClick={() => setStep(1)}
                style={{
                  flex: 1,
                  background: 'transparent',
                  border: `1px solid ${C.border}`,
                  borderRadius: 8,
                  color: C.muted,
                  padding: '0.65rem',
                  cursor: 'pointer',
                  fontSize: 14,
                }}
              >
                Back
              </button>
              <button
                onClick={handleCreateProject}
                disabled={saving}
                style={{
                  flex: 2,
                  background: saving ? '#78481a' : C.amber,
                  border: 'none',
                  borderRadius: 8,
                  color: '#0f1117',
                  padding: '0.65rem',
                  cursor: saving ? 'not-allowed' : 'pointer',
                  fontSize: 14,
                  fontWeight: 700,
                }}
              >
                {saving ? 'Creating…' : 'Create project →'}
              </button>
            </div>
          </div>
        )}

        {/* Step 3: API key / snippet */}
        {step === 3 && (
          <div>
            <h2 style={{ fontSize: 20, fontWeight: 800, margin: '0 0 6px', color: C.text }}>
              Add the tracking snippet
            </h2>
            <p style={{ color: C.muted, fontSize: 14, margin: '0 0 1.25rem' }}>
              Paste this in the <code style={{ color: C.amber }}>&lt;head&gt;</code> of your site to start collecting events.
            </p>

            <div style={{
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 10,
              padding: '1rem',
              marginBottom: '1rem',
              fontFamily: '"SF Mono", "Fira Code", monospace',
              fontSize: 13,
              color: '#a5f3fc',
              whiteSpace: 'pre',
              overflowX: 'auto',
            }}>
              {snippet}
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '1.5rem' }}>
              <CopyButton value={snippet} />
            </div>

            <div style={{ display: 'flex', gap: 8 }}>
              <button
                onClick={() => setStep(4)}
                style={{
                  flex: 1,
                  background: 'transparent',
                  border: `1px solid ${C.border}`,
                  borderRadius: 8,
                  color: C.muted,
                  padding: '0.65rem',
                  cursor: 'pointer',
                  fontSize: 13,
                }}
              >
                I'll do this later
              </button>
              <button
                onClick={() => setStep(4)}
                style={{
                  flex: 2,
                  background: C.amber,
                  border: 'none',
                  borderRadius: 8,
                  color: '#0f1117',
                  padding: '0.65rem',
                  cursor: 'pointer',
                  fontSize: 14,
                  fontWeight: 700,
                }}
              >
                Done, continue →
              </button>
            </div>
          </div>
        )}

        {/* Step 4: Done */}
        {step === 4 && (
          <div style={{ textAlign: 'center' }}>
            <div style={{ fontSize: 52, marginBottom: 12 }}>🎉</div>
            <h2 style={{ fontSize: 22, fontWeight: 800, margin: '0 0 10px', color: C.text }}>
              Your barn is ready!
            </h2>
            <p style={{ color: C.muted, fontSize: 15, lineHeight: 1.6, margin: '0 0 2rem' }}>
              {createdProject
                ? `"${createdProject.name}" is all set up. Head to your dashboard to start exploring.`
                : 'All set up. Head to your dashboard to start exploring.'}
            </p>
            <button
              onClick={onComplete}
              style={{
                background: C.amber,
                border: 'none',
                borderRadius: 10,
                color: '#0f1117',
                padding: '0.75rem 2rem',
                cursor: 'pointer',
                fontSize: 15,
                fontWeight: 700,
                width: '100%',
              }}
            >
              Go to dashboard →
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
