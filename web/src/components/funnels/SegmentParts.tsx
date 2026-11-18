import { Trash2 } from 'lucide-react'
import { DistributionEntry, Segment, SegmentRule } from '../../lib/api'
import { C } from '../../lib/theme'
import { SEGMENT_FIELD_META, SEGMENT_OPERATORS } from './constants'

type SegmentOperator = SegmentRule['operator']

// ─── Distribution Chart ───────────────────────────────────────────────────────

function DistributionChart({ label, entries }: { label: string; entries: { value: string; count: number; pct: number }[] }) {
  if (!entries || entries.length === 0) return null
  const max = entries[0]?.pct ?? 1
  return (
    <div style={{ marginBottom: '1rem' }}>
      <div style={{ fontSize: 11, fontWeight: 600, color: '#94a3b8', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 6 }}>
        {label}
      </div>
      {entries.slice(0, 6).map((e) => (
        <div key={e.value} style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <div style={{ minWidth: 90, fontSize: 12, color: '#e2e8f0', textAlign: 'right', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
            {e.value}
          </div>
          <div style={{ flex: 1, height: 14, background: '#2a2d3a', borderRadius: 3, overflow: 'hidden' }}>
            <div style={{
              height: '100%',
              width: `${(e.pct / max) * 100}%`,
              background: '#f59e0b',
              opacity: 0.7,
              borderRadius: 3,
              transition: 'width 0.4s',
            }} />
          </div>
          <div style={{ fontSize: 11, color: '#94a3b8', minWidth: 32, textAlign: 'right' }}>{e.pct}%</div>
        </div>
      ))}
    </div>
  )
}

// ─── Segment Manager ──────────────────────────────────────────────────────────

export function valueHidden(op: SegmentOperator): boolean {
  return op === 'is_null' || op === 'is_not_null'
}

function SegmentRow({ seg, deleting, onDelete }: { seg: Segment; deleting: boolean; onDelete: () => void }) {
  return (
    <div
      style={{
        background: C.bg,
        border: `1px solid ${C.border}`,
        borderRadius: 8,
        padding: '0.75rem 1rem',
        marginBottom: 8,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ fontWeight: 700, fontSize: 14, color: C.text, marginBottom: 2 }}>{seg.name}</div>
          <div style={{
            fontFamily: 'ui-monospace, monospace',
            fontSize: 10,
            color: C.muted,
            marginBottom: 6,
            wordBreak: 'break-all',
          }}>
            ID: {seg.id}
          </div>
          {seg.rules.map((r, i) => (
            <div key={i} style={{ fontSize: 12, color: C.muted, marginBottom: 2 }}>
              <span style={{ color: C.text }}>{SEGMENT_FIELD_META[r.field]?.label ?? r.field}</span>
              {' '}
              <span style={{ color: C.amber }}>
                {SEGMENT_OPERATORS.find((o) => o.value === r.operator)?.label ?? r.operator}
              </span>
              {!valueHidden(r.operator) && (
                <> <span style={{ color: C.text, fontFamily: '"SF Mono","Fira Code",monospace' }}>{r.value}</span></>
              )}
            </div>
          ))}
        </div>
        <button
          onClick={onDelete}
          disabled={deleting}
          title="Delete segment"
          style={{
            background: 'none',
            border: `1px solid rgba(239,68,68,0.3)`,
            borderRadius: 6,
            color: C.error,
            cursor: deleting ? 'not-allowed' : 'pointer',
            padding: '0.3rem 0.5rem',
            fontSize: 12,
            flexShrink: 0,
            opacity: deleting ? 0.5 : 1,
          }}
        >
          <Trash2 size={12} />
        </button>
      </div>
    </div>
  )
}

const DISTRIBUTION_FIELDS = [
  { key: 'device_type', label: 'Device' },
  { key: 'country_code', label: 'Country' },
  { key: 'browser', label: 'Browser' },
  { key: 'connection_class', label: 'Connection' },
  { key: 'dark_mode', label: 'Dark mode' },
]

export function DistributionSection({ distributions }: { distributions: Record<string, DistributionEntry[]> }) {
  if (Object.keys(distributions).length === 0) return null
  return (
    <div style={{ marginBottom: '1.5rem' }}>
      <div style={{ fontSize: 12, color: C.muted, fontWeight: 600, marginBottom: 10 }}>
        Project-wide visitor breakdown
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: '1rem' }}>
        {DISTRIBUTION_FIELDS.filter(({ key }) => distributions[key]?.length).map(({ key, label }) => (
          <DistributionChart key={key} label={label} entries={distributions[key]} />
        ))}
      </div>
    </div>
  )
}

export function SegmentList({ segments, showCreate, deletingId, onDelete }: {
  segments: Segment[]
  showCreate: boolean
  deletingId: string | null
  onDelete: (seg: Segment) => void
}) {
  return (
    <>
      {segments.length === 0 && !showCreate && (
        <p style={{ color: C.muted, fontSize: 13, margin: '0 0 1rem' }}>No segments yet.</p>
      )}
      {segments.map((seg) => (
        <SegmentRow key={seg.id} seg={seg} deleting={deletingId === seg.id} onDelete={() => onDelete(seg)} />
      ))}
    </>
  )
}
