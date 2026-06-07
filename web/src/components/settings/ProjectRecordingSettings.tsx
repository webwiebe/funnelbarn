import { useEffect, useState } from 'react'
import { Plus, Trash2, ChevronDown, ChevronUp } from 'lucide-react'
import { api, type RecordingRule, type ProjectRecordingSettings as ProjectRecordingSettingsData } from '../../lib/api'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  error: '#ef4444',
  success: '#10b981',
}

interface Props {
  projectId: string
}

export function ProjectRecordingSettings({ projectId }: Props) {
  const [open, setOpen] = useState(false)
  const [settings, setSettings] = useState<ProjectRecordingSettingsData | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [savedMsg, setSavedMsg] = useState(false)

  // Editable state
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [sampleRate, setSampleRate] = useState<number | null>(null)
  const [rules, setRules] = useState<RecordingRule[]>([])

  useEffect(() => {
    if (!open || settings) return
    setLoading(true)
    api.getProjectRecordingSettings(projectId)
      .then((s) => {
        setSettings(s)
        setEnabled(s.enabled)
        setSampleRate(s.sample_rate)
        setRules(s.rules ?? [])
      })
      .finally(() => setLoading(false))
  }, [open, projectId, settings])

  const handleSave = async () => {
    setSaving(true)
    try {
      await api.updateProjectRecordingSettings(projectId, {
        enabled,
        sample_rate: sampleRate,
        rules,
      })
      setSettings(null) // force refresh on next open
      setSavedMsg(true)
      setTimeout(() => setSavedMsg(false), 2000)
    } finally {
      setSaving(false)
    }
  }

  const addRule = () => setRules((r) => [...r, { pattern: '/', action: 'capture' }])
  const removeRule = (i: number) => setRules((r) => r.filter((_, idx) => idx !== i))
  const updateRule = (i: number, field: keyof RecordingRule, value: string) =>
    setRules((r) => r.map((rule, idx) => idx === i ? { ...rule, [field]: value } : rule))

  const buttonBase: React.CSSProperties = {
    border: `1px solid ${C.border}`,
    borderRadius: 6,
    padding: '0.3rem 0.75rem',
    fontSize: 12,
    fontWeight: 600,
    cursor: 'pointer',
    background: 'transparent',
    color: C.muted,
  }

  return (
    <div style={{ borderTop: `1px solid ${C.border}`, marginTop: 8 }}>
      <button
        onClick={() => setOpen((v) => !v)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          width: '100%',
          background: 'none',
          border: 'none',
          padding: '0.6rem 0',
          color: C.muted,
          fontSize: 13,
          cursor: 'pointer',
          textAlign: 'left',
        }}
      >
        {open ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        Recording settings
      </button>

      {open && (
        <div style={{ paddingBottom: '1rem' }}>
          {loading && (
            <div style={{ color: C.muted, fontSize: 13 }}>Loading…</div>
          )}

          {!loading && (
            <>
              {/* Enable override */}
              <div style={{ marginBottom: '1rem' }}>
                <div style={{ fontSize: 12, color: C.muted, marginBottom: 6 }}>Recording</div>
                <div style={{ display: 'flex', gap: 6 }}>
                  {([null, true, false] as const).map((val) => {
                    const label = val === null ? 'Inherit' : val ? 'Force on' : 'Force off'
                    const active = enabled === val
                    return (
                      <button
                        key={String(val)}
                        onClick={() => setEnabled(val)}
                        style={{
                          ...buttonBase,
                          background: active ? 'rgba(245,158,11,0.12)' : 'transparent',
                          borderColor: active ? C.amber : C.border,
                          color: active ? C.amber : C.muted,
                        }}
                      >
                        {label}
                      </button>
                    )
                  })}
                </div>
                {settings && (
                  <div style={{ fontSize: 11, color: C.muted, marginTop: 4 }}>
                    Effective: {settings.effective_enabled ? 'on' : 'off'} at {Math.round((settings.effective_rate ?? 1) * 100)}%
                  </div>
                )}
              </div>

              {/* Sample rate override */}
              <div style={{ marginBottom: '1rem' }}>
                <div style={{ fontSize: 12, color: C.muted, marginBottom: 6 }}>Sample rate override</div>
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                  <button
                    onClick={() => setSampleRate(null)}
                    style={{
                      ...buttonBase,
                      background: sampleRate === null ? 'rgba(245,158,11,0.12)' : 'transparent',
                      borderColor: sampleRate === null ? C.amber : C.border,
                      color: sampleRate === null ? C.amber : C.muted,
                    }}
                  >
                    Inherit
                  </button>
                  {[0.1, 0.25, 0.5, 1].map((rate) => (
                    <button
                      key={rate}
                      onClick={() => setSampleRate(rate)}
                      style={{
                        ...buttonBase,
                        background: sampleRate === rate ? 'rgba(245,158,11,0.12)' : 'transparent',
                        borderColor: sampleRate === rate ? C.amber : C.border,
                        color: sampleRate === rate ? C.amber : C.muted,
                      }}
                    >
                      {Math.round(rate * 100)}%
                    </button>
                  ))}
                </div>
              </div>

              {/* URL rules */}
              <div style={{ marginBottom: '1rem' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
                  <div style={{ fontSize: 12, color: C.muted }}>URL rules <span style={{ color: '#4a5568' }}>(first match wins)</span></div>
                  <button
                    onClick={addRule}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 4,
                      background: 'transparent',
                      border: `1px solid ${C.border}`,
                      borderRadius: 5,
                      padding: '0.2rem 0.5rem',
                      color: C.muted,
                      fontSize: 12,
                      cursor: 'pointer',
                    }}
                  >
                    <Plus size={11} />
                    Add rule
                  </button>
                </div>

                {rules.length === 0 && (
                  <div style={{ fontSize: 12, color: '#4a5568' }}>
                    No rules — all pages captured (when recording is on).
                  </div>
                )}

                {rules.map((rule, i) => (
                  <div key={i} style={{ display: 'flex', gap: 6, marginBottom: 6, alignItems: 'center' }}>
                    <input
                      value={rule.pattern}
                      onChange={(e) => updateRule(i, 'pattern', e.target.value)}
                      placeholder="/path/* or /exact"
                      style={{
                        flex: 1,
                        background: C.bg,
                        border: `1px solid ${C.border}`,
                        borderRadius: 6,
                        padding: '0.35rem 0.6rem',
                        color: C.text,
                        fontSize: 12,
                        fontFamily: '"SF Mono", "Fira Code", monospace',
                        outline: 'none',
                      }}
                      onFocus={(e) => (e.target.style.borderColor = C.amber)}
                      onBlur={(e) => (e.target.style.borderColor = C.border)}
                    />
                    <select
                      value={rule.action}
                      onChange={(e) => updateRule(i, 'action', e.target.value)}
                      style={{
                        background: C.bg,
                        border: `1px solid ${C.border}`,
                        borderRadius: 6,
                        padding: '0.35rem 0.5rem',
                        color: rule.action === 'capture' ? C.success : C.error,
                        fontSize: 12,
                        outline: 'none',
                        cursor: 'pointer',
                      }}
                    >
                      <option value="capture">Capture</option>
                      <option value="ignore">Ignore</option>
                    </select>
                    <button
                      onClick={() => removeRule(i)}
                      style={{
                        background: 'transparent',
                        border: 'none',
                        color: C.muted,
                        cursor: 'pointer',
                        padding: 2,
                        display: 'flex',
                      }}
                    >
                      <Trash2 size={13} />
                    </button>
                  </div>
                ))}

                {rules.length > 0 && (
                  <div style={{ fontSize: 11, color: '#4a5568', marginTop: 4 }}>
                    Use <code style={{ fontFamily: 'monospace' }}>*</code> for a single path segment, <code style={{ fontFamily: 'monospace' }}>**</code> for any depth.
                  </div>
                )}
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <button
                  onClick={handleSave}
                  disabled={saving}
                  style={{
                    background: C.amber,
                    border: 'none',
                    borderRadius: 6,
                    padding: '0.4rem 1rem',
                    color: '#0f1117',
                    fontSize: 13,
                    fontWeight: 700,
                    cursor: saving ? 'not-allowed' : 'pointer',
                  }}
                >
                  {saving ? 'Saving…' : 'Save'}
                </button>
                {savedMsg && (
                  <span style={{ fontSize: 13, color: C.success }}>Saved!</span>
                )}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}
