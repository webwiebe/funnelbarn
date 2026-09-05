import { useEffect, useState } from 'react'
import { Trash2, Plus } from 'lucide-react'
import { api, ApiKey, Project } from '../../lib/api'
import { CopyButton } from '../ui/CopyButton'
import { trackEvent } from '../../lib/analytics'
import { writeLastProjectId } from '../../lib/selectedProject'

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

// scopeTone colours a badge by how much the key can do: amber for
// send-or-read-only, green for anything that can change state. The point is
// that "this token can turn the outbound-email gate off" reads differently at
// a glance from "this token sends page views".
export function scopeTone(scope: string): 'read' | 'write' {
  return scope === 'full' || scope === 'flags:write' ? 'write' : 'read'
}

// scopeHint explains, in one line, what a key with this scope may do.
export function scopeHint(scope: string): string {
  switch (scope) {
    case 'ingest': return 'Send events. Cannot read or change anything.'
    case 'analytics:read': return "Read this project's events, funnels and conversion numbers. Cannot change anything."
    case 'flags:read': return "Read this project's flags. Cannot change them."
    case 'flags:write': return "Read and update this project's flags. Cannot create or delete them."
    case 'full': return 'Full API access to this project.'
    default: return 'Unknown scope — this key grants nothing.'
  }
}

function ScopeBadge({ scope }: { scope: string }) {
  const write = scopeTone(scope) === 'write'
  return (
    <span title={scopeHint(scope)} style={{
      fontSize: 12,
      fontWeight: 600,
      background: write ? 'rgba(16,185,129,0.1)' : 'rgba(245,158,11,0.1)',
      color: write ? C.success : C.amber,
      border: `1px solid ${write ? 'rgba(16,185,129,0.3)' : 'rgba(245,158,11,0.3)'}`,
      borderRadius: 99,
      padding: '0.15rem 0.6rem',
      whiteSpace: 'nowrap',
    }}>
      {scope}
    </span>
  )
}

// formatLastUsed distinguishes a token something depends on from one that was
// issued and forgotten — the question you actually have before revoking.
export function formatLastUsed(iso?: string): string {
  if (!iso) return 'never'
  const d = new Date(iso)
  return isNaN(d.getTime()) ? 'never' : d.toLocaleString()
}

interface ApiKeySettingsProps {
  apiKeys: ApiKey[]
  setApiKeys: React.Dispatch<React.SetStateAction<ApiKey[]>>
  loading: boolean
  projects: Project[]
  /** The project this section is showing keys for — the one the header names. */
  projectId?: string
  onError: (msg: string) => void
}

export function ApiKeySettings({
  apiKeys,
  setApiKeys,
  loading,
  projects,
  projectId,
  onError,
}: ApiKeySettingsProps) {
  const [newKeyName, setNewKeyName] = useState('')
  const [newKeyScope, setNewKeyScope] = useState('ingest')
  // Which project the key is minted for. Defaults to the project on screen, but
  // is shown and changeable, because a key silently bound to a project you
  // never chose is indistinguishable from a working one until it 403s.
  const [newKeyProjectId, setNewKeyProjectId] = useState(projectId ?? '')
  const [creating, setCreating] = useState(false)
  const [newKeyValue, setNewKeyValue] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)

  // Follow the header when the user switches project.
  useEffect(() => { if (projectId) setNewKeyProjectId(projectId) }, [projectId])

  const currentProject = projects.find((p) => p.id === projectId)

  const handleCreate = async () => {
    if (!newKeyName.trim()) { setError('Name is required'); return }
    if (!newKeyProjectId) { setError('Select a project for this key'); return }
    setCreating(true)
    setError(null)
    try {
      const res = await api.createApiKey(newKeyName, newKeyScope, newKeyProjectId)
      trackEvent('api_key_created', { scope: newKeyScope })
      setNewKeyValue(res.key)
      setNewKeyName('')
      if (newKeyProjectId === projectId) {
        setApiKeys((prev) => [...prev, res.api_key])
      } else {
        // The list below only holds the project on screen. Rather than show a
        // key it can never contain, follow the key: switching the selection
        // re-fetches the list and updates the header picker with it.
        writeLastProjectId(newKeyProjectId)
      }
    } catch (e) {
      setError(String(e))
      onError(String(e))
    } finally {
      setCreating(false)
    }
  }

  const handleDeleteKey = async (keyId: string) => {
    if (deleteConfirm !== keyId) {
      setDeleteConfirm(keyId)
      return
    }
    try {
      await api.deleteApiKey(keyId)
      setApiKeys((prev) => prev.filter((k) => k.id !== keyId))
    } catch (e) {
      onError(String(e))
    } finally {
      setDeleteConfirm(null)
    }
  }

  return (
    <div style={{
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
      overflow: 'hidden',
      marginBottom: '2rem',
    }}>
      <div style={{
        padding: '1.25rem 1.5rem',
        borderBottom: `1px solid ${C.border}`,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}>
        <div>
          <div style={{ fontWeight: 700, fontSize: 15 }}>API Keys</div>
          <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
            Keys for{' '}
            {currentProject
              ? <strong style={{ color: C.text, fontWeight: 600 }}>{currentProject.name}</strong>
              : 'this project'}
            {' '}— sending events, reading its analytics, reading or toggling
            its feature flags, or full management access. Every key works on one
            project only.
          </div>
        </div>
      </div>

      {/* New key revealed */}
      {newKeyValue && (
        <div style={{
          padding: '1rem 1.5rem',
          background: 'rgba(16,185,129,0.08)',
          borderBottom: `1px solid rgba(16,185,129,0.2)`,
        }}>
          <div style={{ fontSize: 13, color: C.success, fontWeight: 600, marginBottom: 8 }}>
            New API key created — copy it now, it won't be shown again.
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <code style={{
              flex: 1,
              background: C.bg,
              border: `1px solid ${C.border}`,
              borderRadius: 7,
              padding: '0.5rem 0.875rem',
              fontSize: 13,
              color: C.success,
              fontFamily: '"SF Mono", "Fira Code", monospace',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}>
              {newKeyValue}
            </code>
            <CopyButton value={newKeyValue} />
            <button
              onClick={() => setNewKeyValue(null)}
              style={{
                background: 'transparent',
                border: 'none',
                color: C.muted,
                cursor: 'pointer',
                padding: 4,
              }}
            >
              ✕
            </button>
          </div>
        </div>
      )}

      {/* Keys table */}
      {loading ? (
        <div style={{ padding: '2rem', color: C.muted, fontSize: 14 }}>Loading…</div>
      ) : apiKeys.length === 0 ? (
        <div style={{
          margin: '1.5rem',
          textAlign: 'center',
          padding: '2rem',
          color: C.muted,
          border: `1px dashed ${C.border}`,
          borderRadius: 8,
          fontSize: 14,
        }}>
          No API keys yet. Create one below to start sending events.
        </div>
      ) : (
        <>
          {/* Desktop table */}
          <div className="api-keys-table" style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${C.border}` }}>
                  {['Name', 'Scope', 'Created', 'Last used', ''].map((h) => (
                    <th key={h} style={{
                      textAlign: 'left',
                      padding: '0.75rem 1.5rem',
                      fontSize: 12,
                      fontWeight: 600,
                      color: C.muted,
                      textTransform: 'uppercase',
                      letterSpacing: '0.05em',
                      whiteSpace: 'nowrap',
                    }}>
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {apiKeys.map((key) => (
                  <tr key={key.id} style={{ borderBottom: `1px solid ${C.border}` }}>
                    <td style={{ padding: '0.875rem 1.5rem', fontSize: 14, color: C.text, fontWeight: 600, whiteSpace: 'nowrap' }}>
                      {key.name}
                    </td>
                    <td style={{ padding: '0.875rem 1.5rem', whiteSpace: 'nowrap' }}>
                      <ScopeBadge scope={key.scope} />
                    </td>
                    <td style={{ padding: '0.875rem 1.5rem', fontSize: 13, color: C.muted, whiteSpace: 'nowrap' }}>
                      {new Date(key.created_at).toLocaleDateString()}
                    </td>
                    <td style={{ padding: '0.875rem 1.5rem', fontSize: 13, color: key.last_used_at ? C.muted : '#4a4d5a', whiteSpace: 'nowrap' }}>
                      {formatLastUsed(key.last_used_at)}
                    </td>
                    <td style={{ padding: '0.875rem 1.5rem', textAlign: 'right', whiteSpace: 'nowrap' }}>
                      {deleteConfirm === key.id ? (
                        <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end', alignItems: 'center' }}>
                          <span style={{ fontSize: 12, color: C.error }}>Confirm?</span>
                          <button
                            onClick={() => handleDeleteKey(key.id)}
                            style={{
                              background: 'rgba(239,68,68,0.1)',
                              border: `1px solid rgba(239,68,68,0.3)`,
                              borderRadius: 6,
                              color: C.error,
                              cursor: 'pointer',
                              padding: '0.25rem 0.6rem',
                              fontSize: 12,
                              fontWeight: 700,
                              minHeight: 'unset',
                            }}
                          >
                            Delete
                          </button>
                          <button
                            onClick={() => setDeleteConfirm(null)}
                            style={{
                              background: 'transparent',
                              border: 'none',
                              color: C.muted,
                              cursor: 'pointer',
                              fontSize: 12,
                              minHeight: 'unset',
                            }}
                          >
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <button
                          onClick={() => handleDeleteKey(key.id)}
                          style={{
                            background: 'transparent',
                            border: 'none',
                            color: C.muted,
                            cursor: 'pointer',
                            padding: 4,
                            minHeight: 'unset',
                          }}
                          title="Delete key"
                        >
                          <Trash2 size={14} />
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile cards */}
          <div className="api-keys-cards" style={{ display: 'none' }}>
            {apiKeys.map((key) => (
              <div key={key.id} style={{
                padding: '1rem 1.25rem',
                borderBottom: `1px solid ${C.border}`,
              }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 4 }}>
                  <span style={{ fontWeight: 600, fontSize: 14, color: C.text }}>{key.name}</span>
                  <ScopeBadge scope={key.scope} />
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <span style={{ fontSize: 12, color: C.muted }}>
                    Created: {new Date(key.created_at).toLocaleDateString()} · Last used: {formatLastUsed(key.last_used_at)}
                  </span>
                  {deleteConfirm === key.id ? (
                    <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                      <span style={{ fontSize: 12, color: C.error }}>Confirm?</span>
                      <button
                        onClick={() => handleDeleteKey(key.id)}
                        style={{
                          background: 'rgba(239,68,68,0.1)',
                          border: `1px solid rgba(239,68,68,0.3)`,
                          borderRadius: 6,
                          color: C.error,
                          cursor: 'pointer',
                          padding: '0.2rem 0.5rem',
                          fontSize: 12,
                          fontWeight: 700,
                          minHeight: 'unset',
                        }}
                      >
                        Delete
                      </button>
                      <button
                        onClick={() => setDeleteConfirm(null)}
                        style={{
                          background: 'transparent',
                          border: 'none',
                          color: C.muted,
                          cursor: 'pointer',
                          fontSize: 12,
                          minHeight: 'unset',
                        }}
                      >
                        Cancel
                      </button>
                    </div>
                  ) : (
                    <button
                      onClick={() => handleDeleteKey(key.id)}
                      style={{
                        background: 'transparent',
                        border: `1px solid ${C.border}`,
                        borderRadius: 6,
                        color: C.error,
                        cursor: 'pointer',
                        padding: '0.25rem 0.5rem',
                        fontSize: 12,
                        display: 'flex',
                        alignItems: 'center',
                        gap: 4,
                        minHeight: 'unset',
                      }}
                    >
                      <Trash2 size={12} />
                      Delete
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </>
      )}

      {/* Create form */}
      <div style={{
        padding: '1.25rem 1.5rem',
        borderTop: `1px solid ${C.border}`,
        background: '#13151f',
      }}>
        <div style={{ fontSize: 13, fontWeight: 600, color: C.muted, marginBottom: 12 }}>
          Create new API key
        </div>

        {error && (
          <div style={{
            fontSize: 13,
            color: C.error,
            marginBottom: 10,
          }}>
            {error}
          </div>
        )}

        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <input
            value={newKeyName}
            onChange={(e) => setNewKeyName(e.target.value)}
            placeholder="Key name"
            style={{
              flex: '1 1 180px',
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 7,
              padding: '0.55rem 0.875rem',
              color: C.text,
              fontSize: 14,
              outline: 'none',
            }}
            onFocus={(e) => (e.target.style.borderColor = C.amber)}
            onBlur={(e) => (e.target.style.borderColor = C.border)}
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
          />
          <select
            value={newKeyProjectId}
            onChange={(e) => setNewKeyProjectId(e.target.value)}
            aria-label="Project for the new API key"
            style={{
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 7,
              padding: '0.55rem 0.875rem',
              color: C.text,
              fontSize: 14,
              cursor: 'pointer',
              outline: 'none',
            }}
          >
            {!newKeyProjectId && <option value="">Select a project…</option>}
            {projects.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
          <select
            value={newKeyScope}
            onChange={(e) => setNewKeyScope(e.target.value)}
            aria-label="Scope for the new API key"
            style={{
              background: C.surface,
              border: `1px solid ${C.border}`,
              borderRadius: 7,
              padding: '0.55rem 0.875rem',
              color: C.text,
              fontSize: 14,
              cursor: 'pointer',
              outline: 'none',
            }}
          >
            <option value="ingest">ingest — send events</option>
            <option value="analytics:read">analytics:read — read events, funnels and conversion</option>
            <option value="flags:read">flags:read — read this project&apos;s flags</option>
            <option value="flags:write">flags:write — read and toggle flags</option>
            <option value="full">full — everything</option>
          </select>
          <button
            onClick={handleCreate}
            disabled={creating}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              background: creating ? '#78481a' : C.amber,
              border: 'none',
              borderRadius: 7,
              color: '#0f1117',
              padding: '0.55rem 1rem',
              cursor: creating ? 'not-allowed' : 'pointer',
              fontSize: 14,
              fontWeight: 700,
            }}
          >
            <Plus size={14} />
            {creating ? 'Creating…' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  )
}
