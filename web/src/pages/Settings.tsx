import { useEffect, useState } from 'react'
import { Copy, Trash2, Plus, Check } from 'lucide-react'
import Shell from '../components/Shell'
import { api, ApiKey } from '../lib/api'

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

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <button
      onClick={handleCopy}
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

export default function Settings() {
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [newKeyName, setNewKeyName] = useState('')
  const [newKeyScope, setNewKeyScope] = useState('ingest')
  const [creating, setCreating] = useState(false)
  const [newKeyValue, setNewKeyValue] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

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
      const res = await api.createApiKey(newKeyName, newKeyScope)
      setApiKeys((prev) => [...prev, res.api_key])
      setNewKeyValue(res.key)
      setNewKeyName('')
    } catch (e) {
      setError(String(e))
    } finally {
      setCreating(false)
    }
  }

  return (
    <Shell>
      <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: '-0.5px', marginBottom: '2rem' }}>Settings</h1>

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
          <div style={{ padding: '2rem', color: C.muted, fontSize: 14, textAlign: 'center' }}>
            No API keys yet.
          </div>
        ) : (
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
                  }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {apiKeys.map((key) => (
                <tr key={key.id} style={{ borderBottom: `1px solid ${C.border}` }}>
                  <td style={{ padding: '0.875rem 1.5rem', fontSize: 14, color: C.text, fontWeight: 600 }}>
                    {key.name}
                  </td>
                  <td style={{ padding: '0.875rem 1.5rem' }}>
                    <span style={{
                      fontSize: 12,
                      fontWeight: 600,
                      background: key.scope === 'full' ? 'rgba(239,68,68,0.1)' : 'rgba(16,185,129,0.1)',
                      color: key.scope === 'full' ? C.error : C.success,
                      border: `1px solid ${key.scope === 'full' ? 'rgba(239,68,68,0.3)' : 'rgba(16,185,129,0.3)'}`,
                      borderRadius: 99,
                      padding: '0.15rem 0.6rem',
                    }}>
                      {key.scope}
                    </span>
                  </td>
                  <td style={{ padding: '0.875rem 1.5rem', fontSize: 13, color: C.muted }}>
                    {new Date(key.created_at).toLocaleDateString()}
                  </td>
                  <td style={{ padding: '0.875rem 1.5rem', textAlign: 'right' }}>
                    <button
                      style={{
                        background: 'transparent',
                        border: 'none',
                        color: C.muted,
                        cursor: 'pointer',
                        padding: 4,
                      }}
                      title="Delete key"
                    >
                      <Trash2 size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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
    </Shell>
  )
}
