import { useEffect, useMemo, useState } from 'react'
import { Plus, Trash2, Save } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api, CanonicalEvent } from '../lib/api'
import { useProjects } from '../lib/projects'
import { OverviewTabs } from '../components/OverviewTabs'
import { C } from '../lib/theme'

// A single editable mapping row: a project's raw event name → a canonical key.
interface Row {
  raw_name: string
  canonical_key: string // "" = unmapped/ignore
}

export default function EventMapping() {
  const { projects } = useProjects()
  const [catalog, setCatalog] = useState<CanonicalEvent[]>([])
  const [projectId, setProjectId] = useState('')
  const [rows, setRows] = useState<Row[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  // New-canonical form.
  const [newKey, setNewKey] = useState('')
  const [newLabel, setNewLabel] = useState('')

  const loadCatalog = () => api.listCanonicalEvents().then((r) => setCatalog(r.canonical_events))

  useEffect(() => {
    loadCatalog().catch((e) => setError(e.message))
  }, [])

  // Default to the first project once loaded.
  useEffect(() => {
    if (!projectId && projects.length) setProjectId(projects[0].id)
  }, [projects, projectId])

  // Load existing mappings + unmapped suggestions for the selected project and
  // merge them into one editable list.
  useEffect(() => {
    if (!projectId) return
    setLoading(true)
    setMsg(null)
    setError(null)
    Promise.all([api.listMappings(projectId), api.getMappingSuggestions(projectId)])
      .then(([m, s]) => {
        const merged: Row[] = [
          ...m.mappings.map((x) => ({ raw_name: x.raw_name, canonical_key: x.canonical_key })),
          ...s.suggestions.map((x) => ({ raw_name: x.raw_name, canonical_key: x.suggested_key })),
        ]
        // De-dupe by raw_name, existing mappings win.
        const seen = new Set<string>()
        const dedup = merged.filter((r) => (seen.has(r.raw_name) ? false : (seen.add(r.raw_name), true)))
        dedup.sort((a, b) => a.raw_name.localeCompare(b.raw_name))
        setRows(dedup)
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [projectId])

  const suggestedCount = useMemo(() => rows.filter((r) => r.canonical_key).length, [rows])

  const setRowKey = (raw: string, key: string) =>
    setRows((rs) => rs.map((r) => (r.raw_name === raw ? { ...r, canonical_key: key } : r)))

  const save = () => {
    setSaving(true)
    setMsg(null)
    setError(null)
    const mappings = rows.filter((r) => r.canonical_key).map((r) => ({ raw_name: r.raw_name, canonical_key: r.canonical_key }))
    api.setMappings(projectId, mappings)
      .then(() => setMsg(`Saved ${mappings.length} mapping${mappings.length === 1 ? '' : 's'}.`))
      .catch((e) => setError(e.message || 'save failed'))
      .finally(() => setSaving(false))
  }

  const addCanonical = () => {
    const key = newKey.trim()
    if (!key) return
    api.createCanonicalEvent({ key, label: newLabel.trim() || key, sort_order: (catalog.length + 1) * 10 })
      .then(() => { setNewKey(''); setNewLabel(''); return loadCatalog() })
      .catch((e) => setError(e.message || 'could not add canonical event'))
  }

  const removeCanonical = (key: string) => {
    api.deleteCanonicalEvent(key)
      .then(() => loadCatalog())
      .catch((e) => setError(e.message || 'could not delete (still used by a funnel?)'))
  }

  const inputStyle: React.CSSProperties = {
    background: C.bg, border: `1px solid ${C.border}`, color: C.text,
    borderRadius: 8, padding: '0.4rem 0.6rem', fontSize: 13,
  }

  return (
    <Shell>
      <OverviewTabs />
      <h1 style={{ fontSize: 22, fontWeight: 700, color: C.text, margin: '0 0 0.25rem' }}>Event Normalization</h1>
      <div style={{ color: C.muted, fontSize: 13, marginBottom: '1.25rem' }}>
        Map each site's raw event names onto a shared vocabulary so they can be compared and funneled across projects.
      </div>

      {error && <div style={{ background: 'rgba(239,68,68,0.1)', border: `1px solid ${C.error}`, color: C.error, borderRadius: 8, padding: '0.6rem 0.9rem', marginBottom: '1rem' }}>{error}</div>}
      {msg && <div style={{ background: 'rgba(16,185,129,0.1)', border: `1px solid ${C.success}`, color: C.success, borderRadius: 8, padding: '0.6rem 0.9rem', marginBottom: '1rem' }}>{msg}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(240px, 1fr) 2fr', gap: 16, alignItems: 'start' }}>
        {/* Canonical catalog */}
        <div style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, padding: '1.25rem' }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: C.text, marginBottom: '0.75rem' }}>Canonical events</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginBottom: '1rem' }}>
            {catalog.map((c) => (
              <div key={c.key} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, fontSize: 13, color: C.text }}>
                <span><strong>{c.label}</strong> <span style={{ color: C.muted }}>({c.key})</span></span>
                <button onClick={() => removeCanonical(c.key)} title="Delete" style={{ background: 'transparent', border: 'none', color: C.muted, cursor: 'pointer', display: 'inline-flex' }}>
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <input placeholder="key (e.g. contact_form)" value={newKey} onChange={(e) => setNewKey(e.target.value)} style={inputStyle} />
            <input placeholder="Label (e.g. Contact Form)" value={newLabel} onChange={(e) => setNewLabel(e.target.value)} style={inputStyle} />
            <button onClick={addCanonical} style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: 6, background: 'transparent', border: `1px solid ${C.border}`, color: C.text, borderRadius: 8, padding: '0.4rem', fontSize: 13, cursor: 'pointer' }}>
              <Plus size={14} /> Add canonical event
            </button>
          </div>
        </div>

        {/* Per-project mapping table */}
        <div style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, padding: '1.25rem' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, marginBottom: '0.75rem', flexWrap: 'wrap' }}>
            <div style={{ fontSize: 14, fontWeight: 600, color: C.text }}>Mappings</div>
            <select value={projectId} onChange={(e) => setProjectId(e.target.value)} style={inputStyle}>
              {projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </div>

          {loading ? (
            <div style={{ color: C.muted, fontSize: 13 }}>Loading…</div>
          ) : rows.length === 0 ? (
            <div style={{ color: C.muted, fontSize: 13 }}>No events seen for this site yet.</div>
          ) : (
            <div style={{ overflowX: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
                <thead>
                  <tr style={{ color: C.muted, textAlign: 'left' }}>
                    <th style={{ padding: '0.4rem 0.5rem' }}>Raw event name</th>
                    <th style={{ padding: '0.4rem 0.5rem' }}>Canonical event</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.raw_name} style={{ borderTop: `1px solid ${C.border}`, color: C.text }}>
                      <td style={{ padding: '0.5rem' }}>{r.raw_name}</td>
                      <td style={{ padding: '0.5rem' }}>
                        <select value={r.canonical_key} onChange={(e) => setRowKey(r.raw_name, e.target.value)} style={{ ...inputStyle, width: '100%' }}>
                          <option value="">— ignore —</option>
                          {catalog.map((c) => <option key={c.key} value={c.key}>{c.label}</option>)}
                        </select>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: '1rem', gap: 8 }}>
            <span style={{ color: C.muted, fontSize: 12 }}>{suggestedCount} of {rows.length} mapped</span>
            <button onClick={save} disabled={saving || !projectId} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, background: C.amber, color: '#1a1d27', border: 'none', borderRadius: 8, padding: '0.45rem 1rem', fontSize: 13, fontWeight: 600, cursor: saving ? 'default' : 'pointer' }}>
              <Save size={14} /> {saving ? 'Saving…' : 'Save mappings'}
            </button>
          </div>
        </div>
      </div>
    </Shell>
  )
}
