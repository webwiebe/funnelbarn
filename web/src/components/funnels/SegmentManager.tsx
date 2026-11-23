import { useEffect, useState } from 'react'
import { Plus, X, ChevronDown, ChevronRight } from 'lucide-react'
import { api, DistributionEntry, Segment, SegmentRule } from '../../lib/api'
import { trackEvent } from '../../lib/analytics'
import { C } from '../../lib/theme'
import {
  SEGMENT_FIELD_META,
  SEGMENT_FIELDS,
  SEGMENT_OPERATORS,
  SEGMENT_TEMPLATES,
} from './constants'

type SegmentOperator = SegmentRule['operator']

import { DistributionSection, SegmentList, valueHidden } from './SegmentParts'
import { sortByLabel } from '../../lib/sortOptions'

export function SegmentManager({
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
  const [distributions, setDistributions] = useState<Record<string, DistributionEntry[]>>({})

  useEffect(() => {
    api.getSessionDistributions(projectId)
      .then((d) => setDistributions(d.distributions ?? {}))
      .catch(() => {})
  }, [projectId])

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
      trackEvent('segment_created', { segment_name: seg.name, rule_count: seg.rules.length })
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
      trackEvent('segment_deleted', { segment_name: seg.name, segment_id: seg.id })
      onSegmentsChange(segments.filter((s) => s.id !== seg.id))
    } catch (e) {
      alert('Failed to delete: ' + String(e))
    } finally {
      setDeletingId(null)
    }
  }

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
          <SegmentList
            segments={segments}
            showCreate={showCreate}
            deletingId={deletingId}
            onDelete={handleDelete}
          />

          <DistributionSection distributions={distributions} />

          {/* Starter templates */}
          {!showCreate && (
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
                          {sortByLabel(SEGMENT_FIELDS, (f) => SEGMENT_FIELD_META[f]?.label ?? f).map((f) => (
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
                          {sortByLabel(SEGMENT_OPERATORS, (op) => op.label).map((op) => (
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
