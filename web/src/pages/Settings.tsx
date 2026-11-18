import { useEffect, useState } from 'react'
import { Globe, ShieldOff, Video } from 'lucide-react'
import { ToggleSwitch, PillOptions } from '../components/settings/SettingsControls'
import Shell from '../components/shell/Shell'
import { api, ApiKey } from '../lib/api'
import { useProjects, useEffectiveProjectId } from '../lib/projects'
import { CopyButton } from '../components/ui/CopyButton'
import { reportError } from '../lib/bugbarn'
import { ProjectSettings } from '../components/settings/ProjectSettings'
import { ApiKeySettings } from '../components/settings/ApiKeySettings'
import { McpSettings } from '../components/settings/McpSettings'
import { RetentionSettings } from '../components/settings/RetentionSettings'

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

function parseInstanceSettings(settings: Record<string, string> | undefined) {
  const rate = parseFloat(settings?.recording_sample_rate ?? '1')
  const retention = parseInt(settings?.recording_retention_days ?? '90', 10)
  return {
    geoEnabled: settings?.geo_enabled !== 'false',
    recordingEnabled: settings?.recording_enabled === 'true',
    sampleRatePct: isNaN(rate) ? 100 : Math.round(rate * 100),
    retentionDays: isNaN(retention) ? 90 : retention,
  }
}

function anonymizeParams(val: string): { ip: string } | { session_id: string } {
  const isIP = /^[\d.:a-fA-F]+$/.test(val) && !val.includes('-')
  return isIP ? { ip: val } : { session_id: val }
}

function anonymizeMessage(count: number): string {
  return `${count} session${count !== 1 ? 's' : ''} anonymized`
}

function AnonymizeForm() {
  const [anonymizeInput, setAnonymizeInput] = useState('')
  const [anonymizing, setAnonymizing] = useState(false)
  const [anonymizeResult, setAnonymizeResult] = useState<string | null>(null)
  const [anonymizeError, setAnonymizeError] = useState<string | null>(null)

  const handleAnonymize = async () => {
    const val = anonymizeInput.trim()
    if (!val) return
    setAnonymizing(true)
    setAnonymizeResult(null)
    setAnonymizeError(null)
    try {
      const res = await api.anonymizeGeo(anonymizeParams(val))
      setAnonymizeResult(anonymizeMessage(res.anonymized))
      setAnonymizeInput('')
    } catch (e) {
      setAnonymizeError(String(e))
    } finally {
      setAnonymizing(false)
    }
  }

  return (
    <div style={{ padding: '1.25rem 1.5rem' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
        <ShieldOff size={14} color={C.muted} />
        <div style={{ fontWeight: 600, fontSize: 14 }}>Anonymize geo data</div>
      </div>
      <div style={{ fontSize: 12, color: C.muted, marginBottom: 12, lineHeight: 1.5 }}>
        Enter a session ID or IP address to zero out the stored geo fields (IP, city, region, coordinates, ISP).
        Country code is preserved for aggregate analytics. Use this to fulfill GDPR right-to-erasure requests.
      </div>
      {anonymizeResult && (
        <div style={{ fontSize: 13, color: C.success, marginBottom: 10 }}>{anonymizeResult}</div>
      )}
      {anonymizeError && (
        <div style={{ fontSize: 13, color: C.error, marginBottom: 10 }}>{anonymizeError}</div>
      )}
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <input
          value={anonymizeInput}
          onChange={(e) => setAnonymizeInput(e.target.value)}
          placeholder="Session ID or IP address"
          style={{
            flex: '1 1 240px',
            background: C.bg,
            border: `1px solid ${C.border}`,
            borderRadius: 7,
            padding: '0.55rem 0.875rem',
            color: C.text,
            fontSize: 13,
            outline: 'none',
            fontFamily: '"SF Mono", "Fira Code", monospace',
          }}
          onFocus={(e) => (e.target.style.borderColor = C.amber)}
          onBlur={(e) => (e.target.style.borderColor = C.border)}
          onKeyDown={(e) => e.key === 'Enter' && handleAnonymize()}
        />
        <button
          onClick={handleAnonymize}
          disabled={anonymizing || !anonymizeInput.trim()}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            background: anonymizing ? C.surface : 'rgba(239,68,68,0.1)',
            border: '1px solid rgba(239,68,68,0.3)',
            borderRadius: 7,
            color: '#ef4444',
            padding: '0.55rem 1rem',
            cursor: (anonymizing || !anonymizeInput.trim()) ? 'not-allowed' : 'pointer',
            fontSize: 13,
            fontWeight: 700,
          }}
        >
          <ShieldOff size={13} />
          {anonymizing ? 'Anonymizing…' : 'Anonymize'}
        </button>
      </div>
    </div>
  )
}

export default function Settings() {
  const { projects, isLoading: projectsLoading, refetch: refetchProjects, defaultProjectId, setDefaultProjectId } = useProjects()
  // /settings carries no /:projectId, so resolve the same project the header
  // picker displays — otherwise this page acts on a project the user never
  // chose.
  const activeProjectId = useEffectiveProjectId()
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Data collection / geo settings
  const [geoEnabled, setGeoEnabled] = useState(true)
  const [savingGeo, setSavingGeo] = useState(false)

  // Recording settings
  const [recordingEnabled, setRecordingEnabled] = useState(false)
  const [recordingSampleRate, setRecordingSampleRate] = useState(100)
  const [recordingRetentionDays, setRecordingRetentionDays] = useState(90)
  const [savingRecording, setSavingRecording] = useState(false)

  useEffect(() => {
    if (!activeProjectId) {
      // No project to scope to: stop waiting once we know why (none exist),
      // rather than leaving the section on "Loading…" forever.
      if (!projectsLoading) { setApiKeys([]); setLoading(false) }
      return
    }
    let cancelled = false
    setLoading(true)
    api.listApiKeys(activeProjectId)
      .then((d) => { if (!cancelled) setApiKeys(d.api_keys || []) })
      .catch((e) => {
        if (cancelled) return
        reportError(e, { source: 'Settings.listApiKeys' })
        setApiKeys([])
      })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [activeProjectId, projectsLoading])

  useEffect(() => {
    api.getInstanceSettings()
      .then((d) => {
        const parsed = parseInstanceSettings(d.settings)
        setGeoEnabled(parsed.geoEnabled)
        setRecordingEnabled(parsed.recordingEnabled)
        setRecordingSampleRate(parsed.sampleRatePct)
        setRecordingRetentionDays(parsed.retentionDays)
      })
      .catch(() => {})
  }, [])

  const handleGeoToggle = async (enabled: boolean) => {
    setGeoEnabled(enabled)
    setSavingGeo(true)
    setError(null)
    try {
      await api.setInstanceSettings({ geo_enabled: enabled ? 'true' : 'false' })
    } catch (e) {
      setGeoEnabled(!enabled)
      setError(String(e))
    } finally {
      setSavingGeo(false)
    }
  }

  const handleSaveRecordingSettings = async () => {
    setSavingRecording(true)
    setError(null)
    try {
      await api.setInstanceSettings({
        recording_enabled: recordingEnabled ? 'true' : 'false',
        recording_sample_rate: String(recordingSampleRate / 100),
        recording_retention_days: String(recordingRetentionDays),
      })
    } catch (e) {
      setError(String(e))
    } finally {
      setSavingRecording(false)
    }
  }

  const publicURL = window.location.origin

  // Find first ingest key for embed snippet.
  // If we just created an ingest key, prefer the raw key value (shown once).
  // Otherwise fall back to the key ID from the list (a human-readable reference).
  const ingestKey = apiKeys.find((k) => k.scope === 'ingest')
  const snippetKey = ingestKey?.id ?? 'fb_your_key_here'
  const snippet = `<script src="${publicURL}/sdk.js"\n        data-api-key="${snippetKey}"></script>`

  return (
    <Shell projectId={activeProjectId}>
      <style>{`
        @media (max-width: 640px) {
          .api-keys-table { display: none !important; }
          .api-keys-cards { display: block !important; }
          .project-row { flex-wrap: wrap !important; }
          .project-row input { width: 100% !important; flex: none !important; }
          .project-row .project-actions { width: 100% !important; margin-top: 6px; }
          .project-save-btn { flex: 1 !important; justify-content: center !important; }
        }
        @media (min-width: 641px) {
          .api-keys-table { display: block !important; }
          .api-keys-cards { display: none !important; }
        }
      `}</style>
      <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', marginBottom: '2rem' }}>Settings</h1>

      {error && (
        <div style={{ fontSize: 13, color: C.error, marginBottom: '1rem' }}>{error}</div>
      )}

      <ProjectSettings
        projects={projects}
        refetchProjects={refetchProjects}
        defaultProjectId={defaultProjectId}
        setDefaultProjectId={setDefaultProjectId}
        onError={setError}
        publicURL={publicURL}
      />

      {/* Embed snippet */}
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 12,
        overflow: 'hidden',
        marginBottom: '2rem',
      }}>
        <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
          <div style={{ fontWeight: 700, fontSize: 15 }}>Tracking snippet</div>
          <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
            Paste this in the <code style={{ color: C.amber }}>&lt;head&gt;</code> of your site.
          </div>
        </div>
        <div style={{ padding: '1.25rem 1.5rem' }}>
          <div style={{
            background: C.bg,
            border: `1px solid ${C.border}`,
            borderRadius: 10,
            padding: '1rem',
            marginBottom: '0.75rem',
            fontFamily: '"SF Mono", "Fira Code", monospace',
            fontSize: 13,
            color: '#a5f3fc',
            whiteSpace: 'pre',
            overflowX: 'auto',
          }}>
            {snippet}
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <CopyButton value={snippet} />
          </div>

          {/* Segments reference */}
          <div style={{ marginTop: '1rem', paddingTop: '0.75rem', borderTop: `1px solid ${C.border}` }}>
            <div style={{ fontSize: 12, fontWeight: 600, color: C.muted, marginBottom: 6 }}>
              Segments reference
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <div style={{ fontSize: 12, color: C.muted }}>
                <span style={{ color: C.text }}>Mobile / Desktop / Tablet</span> — Detected automatically from the browser User-Agent.
              </div>
              <div style={{ fontSize: 12, color: C.muted }}>
                <span style={{ color: C.text }}>Logged in / Not logged in</span> — Call{' '}
                <code style={{ fontFamily: '"SF Mono", "Fira Code", monospace', color: C.amber, fontSize: 12 }}>
                  funnelbarn.identify('user-id')
                </code>{' '}
                after login.
              </div>
              <div style={{ fontSize: 12, color: C.muted }}>
                <span style={{ color: C.text }}>New visitors / Returning</span> — Tracked automatically via session in localStorage.
              </div>
            </div>
          </div>
        </div>
      </div>

      <ApiKeySettings
        apiKeys={apiKeys}
        setApiKeys={setApiKeys}
        loading={loading}
        projects={projects}
        projectId={activeProjectId}
        onError={setError}
      />

      <McpSettings projects={projects} projectId={activeProjectId} />

      <RetentionSettings />

      {/* Data Collection */}
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 12,
        overflow: 'hidden',
        marginBottom: '2rem',
      }}>
        <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Globe size={16} color={C.amber} />
            <div style={{ fontWeight: 700, fontSize: 15 }}>Data Collection</div>
          </div>
          <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
            Geo enrichment and anonymization controls. IP, city, and coordinates are resolved from visitor IPs at ingest time and stored on your server only.
          </div>
        </div>

        {/* Geo toggle */}
        <div style={{
          padding: '1.25rem 1.5rem',
          borderBottom: `1px solid ${C.border}`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 16,
          flexWrap: 'wrap',
        }}>
          <div style={{ flex: 1, minWidth: 200 }}>
            <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 3 }}>Geo enrichment</div>
            <div style={{ fontSize: 12, color: C.muted, lineHeight: 1.5 }}>
              Resolves visitor IPs to country, region, city, coordinates, timezone, and ISP.
              Requires{' '}
              <code style={{ color: C.amber, fontFamily: '"SF Mono", "Fira Code", monospace', fontSize: 11 }}>FUNNELBARN_GEOIP_CITY_DB</code>
              {' '}to point to a GeoLite2-City.mmdb file.
            </div>
          </div>
          <ToggleSwitch
            on={geoEnabled}
            disabled={savingGeo}
            label="Geo enrichment"
            onToggle={() => handleGeoToggle(!geoEnabled)}
          />
        </div>

        <AnonymizeForm />
      </div>

      {/* Session Recordings */}
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 12,
        overflow: 'hidden',
        marginBottom: '2rem',
      }}>
        <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Video size={16} color={C.amber} />
            <div style={{ fontWeight: 700, fontSize: 15 }}>Session Recordings</div>
          </div>
          <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
            Control dogfood self-recording on the FunnelBarn dashboard itself. Requires R2 credentials to be configured.
          </div>
        </div>

        {/* Enable toggle */}
        <div style={{
          padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}`,
          display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap',
        }}>
          <div style={{ flex: 1, minWidth: 200 }}>
            <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 3 }}>Enable recording</div>
            <div style={{ fontSize: 12, color: C.muted, lineHeight: 1.5 }}>
              When enabled, visitor sessions on the FunnelBarn dashboard are recorded using rrweb.
            </div>
          </div>
          <ToggleSwitch on={recordingEnabled} onToggle={() => setRecordingEnabled((v) => !v)} />
        </div>

        {/* Sample rate */}
        <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
          <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 3 }}>
            Sample rate: <span style={{ color: C.amber }}>{recordingSampleRate}%</span>
          </div>
          <div style={{ fontSize: 12, color: C.muted, marginBottom: 12, lineHeight: 1.5 }}>
            Percentage of sessions to record. 100% records all sessions; lower values reduce storage usage.
          </div>
          <PillOptions options={[10, 25, 50, 100]} value={recordingSampleRate} suffix="%" onSelect={setRecordingSampleRate} />
        </div>

        {/* Retention */}
        <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
          <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 3 }}>
            Retention: <span style={{ color: C.amber }}>{recordingRetentionDays} days</span>
          </div>
          <div style={{ fontSize: 12, color: C.muted, marginBottom: 12, lineHeight: 1.5 }}>
            Recordings older than this are automatically deleted from R2 and the database during nightly cleanup.
          </div>
          <PillOptions options={[7, 14, 30, 60, 90]} value={recordingRetentionDays} suffix="d" onSelect={setRecordingRetentionDays} />
        </div>

        {/* Save */}
        <div style={{ padding: '1rem 1.5rem', display: 'flex', justifyContent: 'flex-end' }}>
          <button
            onClick={handleSaveRecordingSettings}
            disabled={savingRecording}
            style={{
              background: savingRecording ? C.surface : 'rgba(245,158,11,0.15)',
              border: `1px solid rgba(245,158,11,0.4)`,
              borderRadius: 8, color: C.amber,
              padding: '0.55rem 1.25rem', cursor: savingRecording ? 'not-allowed' : 'pointer',
              fontSize: 13, fontWeight: 700,
            }}
          >
            {savingRecording ? 'Saving…' : 'Save recording settings'}
          </button>
        </div>
      </div>
    </Shell>
  )
}
