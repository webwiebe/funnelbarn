import { useEffect, useState } from 'react'
import { Trash2, Save, CheckCircle, Link, X } from 'lucide-react'
import { api, Project } from '../../lib/api'
import { CopyButton } from '../ui/CopyButton'
import { ProjectRecordingSettings } from './ProjectRecordingSettings'

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

interface ProjectSettingsProps {
  projects: Project[]
  refetchProjects: () => void
  defaultProjectId: string | null
  setDefaultProjectId: (id: string) => void
  onError: (msg: string) => void
  publicURL: string
}

export function ProjectSettings({
  projects,
  refetchProjects,
  defaultProjectId,
  setDefaultProjectId,
  onError,
  publicURL,
}: ProjectSettingsProps) {
  const [editedNames, setEditedNames] = useState<Record<string, string>>({})
  const [editedDomains, setEditedDomains] = useState<Record<string, string>>({})
  const [savingProject, setSavingProject] = useState<string | null>(null)
  const [projectSaveMsg, setProjectSaveMsg] = useState<string | null>(null)

  const [deleteProjectConfirm, setDeleteProjectConfirm] = useState<string | null>(null)
  const [deletingProject, setDeletingProject] = useState<string | null>(null)

  const [approvingProject, setApprovingProject] = useState<string | null>(null)
  const [rejectingProject, setRejectingProject] = useState<string | null>(null)

  // Sync editedNames when projects change
  useEffect(() => {
    const names: Record<string, string> = {}
    for (const p of projects) names[p.id] = p.name
    setEditedNames(names)
  }, [projects])

  const handleRejectProject = async (projectId: string) => {
    setRejectingProject(projectId)
    try {
      await api.deleteProject(projectId)
      refetchProjects()
    } finally {
      setRejectingProject(null)
    }
  }

  const handleApproveProject = async (projectId: string) => {
    setApprovingProject(projectId)
    try {
      await api.approveProject(projectId)
      refetchProjects()
    } catch (e) {
      onError(String(e))
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
      setProjectSaveMsg(projectId)
      setTimeout(() => setProjectSaveMsg(null), 2000)
    } catch (e) {
      onError(String(e))
    } finally {
      setSavingProject(null)
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
      onError(String(e))
    } finally {
      setDeletingProject(null)
    }
  }

  const inputStyle = {
    background: C.surface,
    border: `1px solid ${C.border}`,
    borderRadius: 7,
    padding: '0.55rem 0.875rem',
    color: C.text,
    fontSize: 14,
    outline: 'none',
  }

  const pendingProjects = projects.filter((p) => p.status === 'pending')
  const activeProjects = projects.filter((p) => p.status !== 'pending')

  return (
    <>
      {/* Pending projects section */}
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
                {/* Reject button */}
                <button
                  onClick={() => handleRejectProject(p.id)}
                  disabled={rejectingProject === p.id}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 5,
                    background: 'rgba(239,68,68,0.08)',
                    border: '1px solid rgba(239,68,68,0.25)',
                    borderRadius: 7,
                    color: '#ef4444',
                    padding: '0.5rem 1rem',
                    cursor: rejectingProject === p.id ? 'not-allowed' : 'pointer',
                    fontSize: 13,
                    fontWeight: 700,
                  }}
                >
                  <X size={13} />
                  {rejectingProject === p.id ? 'Rejecting…' : 'Reject'}
                </button>
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

      {/* Default project selector */}
      {activeProjects.length > 1 && (
        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          overflow: 'hidden',
          marginBottom: '2rem',
        }}>
          <div style={{ padding: '1.25rem 1.5rem', borderBottom: `1px solid ${C.border}` }}>
            <div style={{ fontWeight: 700, fontSize: 15 }}>Default project</div>
            <div style={{ fontSize: 13, color: C.muted, marginTop: 2 }}>
              Shown first on login and when navigating to a page without a project in the URL.
            </div>
          </div>
          <div style={{ padding: '1rem 1.5rem', display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {activeProjects.map((p) => {
              const isDefault = (defaultProjectId ?? activeProjects[0]?.id) === p.id
              return (
                <button
                  key={p.id}
                  onClick={() => setDefaultProjectId(p.id)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                    padding: '0.45rem 0.875rem',
                    borderRadius: 8,
                    border: `1px solid ${isDefault ? C.amber : C.border}`,
                    background: isDefault ? 'rgba(245,158,11,0.1)' : 'transparent',
                    color: isDefault ? C.amber : C.muted,
                    fontSize: 13,
                    fontWeight: isDefault ? 600 : 400,
                    cursor: isDefault ? 'default' : 'pointer',
                    transition: 'all 0.15s',
                    minHeight: 'unset',
                  }}
                >
                  {isDefault && <CheckCircle size={13} />}
                  {p.name}
                </button>
              )
            })}
          </div>
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
              {projectSaveMsg === p.id && savingProject === null && (
                <div style={{ width: '100%', fontSize: 13, color: C.success, paddingTop: 2 }}>
                  Saved!
                </div>
              )}
              <ProjectRecordingSettings projectId={p.id} />
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
    </>
  )
}
