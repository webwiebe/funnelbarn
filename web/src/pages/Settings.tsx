import { useEffect, useState } from 'react'
import { Trash2, Plus, Save, CheckCircle, Link } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api, ApiKey } from '../lib/api'
import { useProjects } from '../lib/projects'
import { CopyButton } from '../components/ui/CopyButton'

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

function ScopeBadge({ scope }: { scope: string }) {
  const isIngest = scope === 'ingest'
  return (
    <span style={{
      fontSize: 12,
      fontWeight: 600,
      background: isIngest ? 'rgba(245,158,11,0.1)' : 'rgba(16,185,129,0.1)',
      color: isIngest ? C.amber : C.success,
      border: `1px solid ${isIngest ? 'rgba(245,158,11,0.3)' : 'rgba(16,185,129,0.3)'}`,
      borderRadius: 99,
      padding: '0.15rem 0.6rem',
    }}>
      {scope}
    </span>
  )
}

export default function Settings() {
  const { projects, refetch: refetchProjects } = useProjects()
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [newKeyName, setNewKeyName] = useState('')
  const [newKeyScope, setNewKeyScope] = useState('ingest')
  const [creating, setCreating] = useState(false)
  const [newKeyValue, setNewKeyValue] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Project editing — initialise from shared context
  const [editedNames, setEditedNames] = useState<Record<string, string>>({})
  const [editedDomains, setEditedDomains] = useState<Record<string, string>>({})
  const [savingProject, setSavingProject] = useState<string | null>(null)
  const [projectSaveMsg, setProjectSaveMsg] = useState<string | null>(null)

  // Confirm delete (api key)
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)

  // Confirm delete project
  const [deleteProjectConfirm, setDeleteProjectConfirm] = useState<string | null>(null)
  const [deletingProject, setDeletingProject] = useState<string | null>(null)

  // Approving pending projects
  const [approvingProject, setApprovingProject] = useState<string | null>(null)

  // Sync editedNames when context projects change
  useEffect(() => {
    const names: Record<string, string> = {}
    for (const p of projects) names[p.id] = p.name
    setEditedNames(names)
  }, [projects])

  useEffect(() => {
    api.listApiKeys()
      .then((d) => setApiKeys(d.api_keys || []))
      .catch(() => setApiKeys([]))
      .finally(() => setLoading(false))
  }, [])

  const handleCreate = async () => {
    if (!newKeyName.trim()) { setError('Name is required'); return }
    setCreating(true)
    setError(null)
    try {
      // Pass the first project ID so the backend doesn't have to guess.
      const projectId = projects[0]?.id
      const res = await api.createApiKey(newKeyName, newKeyScope, projectId)
      setApiKeys((prev) => [...prev, res.api_key])
      setNewKeyValue(res.key)
      setNewKeyName('')
    } catch (e) {
      setError(String(e))
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
      setError(String(e))
    } finally {
      setDeleteConfirm(null)
    }
  }

  const handleDeleteProject = async (projectId: string) => {
    if (deleteProjectConfirm !== projectId) {
      setDeleteProjectConfirm(projectId)
      return
    }
    setDeletingProject(projectId)
    try {
      await api.deleteProject(projectId)
      setDeleteProjectConfirm(null)
      refetchProjects()
    } catch (e) {
      setError(String(e))
    } finally {
      setDeletingProject(null)
    }
  }

  const handleApproveProject = async (projectId: string) => {
    setApprovingProject(projectId)
    try {
      await api.approveProject(projectId)
      refetchProjects()
    } catch (e) {
      setError(String(e))
    } finally {
      setApprovingProject(null)
    }
  }

  const handleSaveProject = async (projectId: string) => {
    const name = editedNames[projectId]?.trim()
    if (!name) return
    const domain = editedDomains[projectId]?.trim() ?? ''
    setSavingProject(projectId)
    try {
      await api.updateProject(projectId, { name, domain })
      refetchProjects()
      setProjectSaveMsg('Saved!')
      setTimeout(() => setProjectSaveMsg(null), 2000)
    } catch (e) {
      setError(String(e))
    } finally {
      setSavingProject(null)
    }
  }

  const pendingProjects = projects.filter((p) => p.status === 'pending')
  const activeProjects = projects.filter((p) => p.status !== 'pending')

  const publicURL = window.location.origin

  // Find first ingest key for embed snippet.
  // If we just created an ingest key, prefer the raw key value (shown once).
  // Otherwise fall back to the key ID from the list (a human-readable reference).
  const ingestKey = apiKeys.find((k) => k.scope === 'ingest')
  const snippetKey = newKeyValue ?? ingestKey?.id ?? 'fb_your_key_here'
  const snippet = `<script src="https://funnelbarn.wiebe.xyz/sdk.js"\n        data-api-key="${snippetKey}"></script>`

  const inputStyle = {
    background: C.surface,
    border: `1px solid ${C.border}`,
    borderRadius: 7,
    padding: '0.55rem 0.875rem',
    color: C.text,
    fontSize: 14,
    outline: 'none',
  }

  return (
    <Shell>
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

      {/* Pending projects section — shown above active projects */}
      {pendingProjects.length > 0 && (
        <div style={{
          background: C.surface,
          border: `1px solid rgba(245,158,11,0.4)`,
          borderRadius: 12,
          overflow: 'hidden',
          marginBottom: '2rem',
        }}>
          <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid rgba(245,158,11,0.2)`, background: 'rgba(245,158,11,0.06)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <div style={{ fontWeight: 700, fontSize: 15 }}>Pending Projects</div>
              <span style={{
                fontSize: 11,
                fontWeight: 700,
                background: 'rgba(245,158,11,0.15)',
                color: C.amber,
                border: `1px solid rgba(245,158,11,0.35)`,
                borderRadius: 99,
                padding: '0.1rem 0.5rem',
                textTransform: 'uppercase',
                letterSpacing: '0.05em',
              }}>
                {pendingProjects.length}
              </span>
            </div>
            <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
              These projects were created via the setup page and are awaiting approval.
            </div>
          </div>
          {pendingProjects.map((p) => (
            <div key={p.id} style={{
              padding: '1rem 1.5rem',
              borderBottom: `1px solid ${C.border}`,
              display: 'flex',
              flexWrap: 'wrap',
              alignItems: 'center',
              gap: 12,
            }}>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span style={{ fontSize: 14, fontWeight: 600, color: C.text }}>{p.name}</span>
                  <span style={{
                    fontSize: 11,
                    fontWeight: 700,
                    background: 'rgba(245,158,11,0.1)',
                    color: C.amber,
                    border: `1px solid rgba(245,158,11,0.3)`,
                    borderRadius: 99,
                    padding: '0.1rem 0.5rem',
                  }}>
                    pending
                  </span>
                </div>
                <div style={{ fontSize: 12, color: C.muted, marginTop: 2 }}>{p.slug}</div>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                {/* Setup URL copy link */}
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{ fontSize: 12, color: C.muted, fontFamily: '"SF Mono", "Fira Code", monospace' }}>
                    /api/v1/setup/{p.slug}
                  </span>
                  <CopyButton value={`${publicURL}/api/v1/setup/${p.slug}`} />
                  <a
                    href={`/api/v1/setup/${p.slug}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    style={{
                      background: C.bg,
                      border: `1px solid ${C.border}`,
                      borderRadius: 7,
                      color: C.muted,
                      padding: '0.4rem 0.75rem',
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      gap: 5,
                      fontSize: 13,
                      textDecoration: 'none',
                    }}
                  >
                    <Link size={13} />
                    Open
                  </a>
                </div>
                {/* Approve button */}
                <button
                  onClick={() => handleApproveProject(p.id)}
                  disabled={approvingProject === p.id}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 5,
                    background: approvingProject === p.id ? '#1a4a2e' : 'rgba(16,185,129,0.1)',
                    border: `1px solid rgba(16,185,129,0.3)`,
                    borderRadius: 7,
                    color: C.success,
                    padding: '0.5rem 1rem',
                    cursor: approvingProject === p.id ? 'not-allowed' : 'pointer',
                    fontSize: 13,
                    fontWeight: 700,
                  }}
                >
                  <CheckCircle size={13} />
                  {approvingProject === p.id ? 'Approving…' : 'Approve'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Active projects section */}
      {activeProjects.length > 0 && (
        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          overflow: 'hidden',
          marginBottom: '2rem',
        }}>
          <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
            <div style={{ fontWeight: 700, fontSize: 15 }}>Projects</div>
            <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>Edit project names and domains.</div>
          </div>
          {activeProjects.map((p) => (
            <div key={p.id} className="project-row" style={{
              padding: '1rem 1.5rem',
              borderBottom: `1px solid ${C.border}`,
              display: 'flex',
              flexWrap: 'wrap',
              alignItems: 'center',
              gap: 12,
            }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6, flex: 1, minWidth: 0 }}>
                <input
                  value={editedNames[p.id] ?? p.name}
                  onChange={(e) => setEditedNames((prev) => ({ ...prev, [p.id]: e.target.value }))}
                  placeholder="Project name"
                  style={{ ...inputStyle, width: '100%', boxSizing: 'border-box' }}
                  onFocus={(e) => (e.target.style.borderColor = C.amber)}
                  onBlur={(e) => (e.target.style.borderColor = C.border)}
                  onKeyDown={(e) => e.key === 'Enter' && handleSaveProject(p.id)}
                />
                <input
                  value={editedDomains[p.id] ?? (p.domain ?? '')}
                  onChange={(e) => setEditedDomains((prev) => ({ ...prev, [p.id]: e.target.value }))}
                  placeholder="Domain (e.g. example.com)"
                  style={{ ...inputStyle, width: '100%', boxSizing: 'border-box' }}
                  onFocus={(e) => (e.target.style.borderColor = C.amber)}
                  onBlur={(e) => (e.target.style.borderColor = C.border)}
                  onKeyDown={(e) => e.key === 'Enter' && handleSaveProject(p.id)}
                />
              </div>
              <div className="project-actions" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <button
                  className="project-save-btn"
                  onClick={() => handleSaveProject(p.id)}
                  disabled={savingProject === p.id}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 5,
                    background: C.amber,
                    border: 'none',
                    borderRadius: 7,
                    color: '#0f1117',
                    padding: '0.5rem 1rem',
                    cursor: 'pointer',
                    fontSize: 13,
                    fontWeight: 700,
                  }}
                >
                  <Save size={13} />
                  {savingProject === p.id ? 'Saving…' : 'Save'}
                </button>
                {deleteProjectConfirm !== p.id && (
                  <button
                    onClick={() => setDeleteProjectConfirm(p.id)}
                    style={{
                      background: 'transparent',
                      border: 'none',
                      color: C.muted,
                      cursor: 'pointer',
                      padding: 4,
                    }}
                    title="Delete project"
                  >
                    <Trash2 size={14} />
                  </button>
                )}
              </div>
              {projectSaveMsg && savingProject === null && (
                <div style={{ width: '100%', fontSize: 13, color: C.success, paddingTop: 2 }}>
                  {projectSaveMsg}
                </div>
              )}
              {deleteProjectConfirm === p.id && (
                <div style={{
                  width: '100%',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '0.5rem 0',
                  flexWrap: 'wrap',
                }}>
                  <span style={{ fontSize: 13, color: C.error, flex: 1 }}>
                    Delete? This removes all data.
                  </span>
                  <button
                    onClick={() => handleDeleteProject(p.id)}
                    disabled={deletingProject === p.id}
                    style={{
                      background: 'rgba(239,68,68,0.1)',
                      border: `1px solid rgba(239,68,68,0.3)`,
                      borderRadius: 6,
                      color: C.error,
                      cursor: 'pointer',
                      padding: '0.25rem 0.6rem',
                      fontSize: 12,
                      fontWeight: 700,
                    }}
                  >
                    {deletingProject === p.id ? 'Deleting…' : 'Confirm'}
                  </button>
                  <button
                    onClick={() => setDeleteProjectConfirm(null)}
                    style={{
                      background: 'transparent',
                      border: 'none',
                      color: C.muted,
                      cursor: 'pointer',
                      fontSize: 12,
                    }}
                  >
                    Cancel
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

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
        </div>
      </div>

      {/* API Keys section */}
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
              Keys for sending events or full management access.
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
                    {['Name', 'Scope', 'Created', ''].map((h) => (
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
                      Created: {new Date(key.created_at).toLocaleDateString()}
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
              value={newKeyScope}
              onChange={(e) => setNewKeyScope(e.target.value)}
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
              <option value="ingest">ingest</option>
              <option value="full">full</option>
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

      {/* Event retention info */}
      <div style={{
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 12,
        overflow: 'hidden',
        marginBottom: '2rem',
      }}>
        <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
          <div style={{ fontWeight: 700, fontSize: 15 }}>Data Retention</div>
          <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
            How long raw event data is kept before being purged.
          </div>
        </div>
        <div style={{ padding: '1.25rem 1.5rem', display: 'flex', alignItems: 'center', gap: 12 }}>
          <div style={{
            background: 'rgba(245,158,11,0.08)',
            border: `1px solid rgba(245,158,11,0.2)`,
            borderRadius: 8,
            padding: '0.75rem 1rem',
            flex: 1,
          }}>
            <div style={{ fontSize: 14, color: C.text, fontWeight: 600, marginBottom: 4 }}>
              Event retention: 90 days
            </div>
            <div style={{ fontSize: 12, color: C.muted }}>
              Configured via{' '}
              <code style={{
                fontFamily: '"SF Mono", "Fira Code", monospace',
                color: C.amber,
                background: 'rgba(245,158,11,0.08)',
                padding: '0.1rem 0.3rem',
                borderRadius: 3,
              }}>
                FUNNELBARN_EVENT_RETENTION_DAYS
              </code>
              {' '}on the server. Events older than this window are automatically deleted.
            </div>
          </div>
        </div>
      </div>
    </Shell>
  )
}
