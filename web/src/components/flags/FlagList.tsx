import { useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import { Flag, Search } from 'lucide-react'
import type { FeatureFlag } from '../../lib/api'
import { Skeleton } from '../ui/Skeleton'
import { C, statusBadge } from './shared'
import {
  applyFlagListQuery, defaultFlagListQuery, isFiltered, statusCounts, STATUS_FILTERS,
  type FlagListQuery, type FlagSort, type KindFilter, type OriginFilter,
} from './flagListQuery'

// FlagList is the left column of the flags page: a count, search, filters and
// sort above the rows. On wide screens the rows scroll inside the column (see
// the .flags-rows rule in Flags.tsx), so a long list never pushes the detail
// pane out of view.
export function FlagList({ flags, error, onRetry, selectedId, onSelect, onCreate }: {
  /** null while the first load is in flight. */
  flags: FeatureFlag[] | null
  error: string | null
  onRetry: () => void
  selectedId: string | null
  onSelect: (f: FeatureFlag) => void
  onCreate: () => void
}) {
  const [query, setQuery] = useState<FlagListQuery>(defaultFlagListQuery)
  const all = useMemo(() => flags ?? [], [flags])
  const visible = useMemo(() => applyFlagListQuery(all, query), [all, query])
  const counts = useMemo(() => statusCounts(all, query), [all, query])
  const set = (patch: Partial<FlagListQuery>) => setQuery((q) => ({ ...q, ...patch }))

  if (error) return <ListError message={error} onRetry={onRetry} />
  if (flags === null) return <ListLoading />
  if (all.length === 0) return <ListEmpty onCreate={onCreate} />

  return (
    <>
      <div className="flags-toolbar" style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 10 }}>
        <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}>
          <span data-testid="flag-count" style={{ fontSize: 13, color: C.muted }}>
            {visible.length === all.length
              ? `${all.length} ${all.length === 1 ? 'flag' : 'flags'}`
              : `${visible.length} of ${all.length} flags`}
          </span>
          <SortSelect value={query.sort} onChange={(sort) => set({ sort })} />
        </div>
        <SearchBox value={query.search} onChange={(search) => set({ search })} />
        <div role="group" aria-label="Filter by status" style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {STATUS_FILTERS.map((s) => (
            <Chip key={s} active={query.status === s} onClick={() => set({ status: s })}>
              {s === 'all' ? 'All' : s[0].toUpperCase() + s.slice(1)} <span style={{ opacity: 0.7 }}>{counts[s]}</span>
            </Chip>
          ))}
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <FacetSelect label="Origin" value={query.origin} onChange={(v) => set({ origin: v as OriginFilter })}
            options={[['all', 'All origins'], ['manual', 'Created here'], ['auto', 'Auto-detected']]} />
          <FacetSelect label="Kind" value={query.kind} onChange={(v) => set({ kind: v as KindFilter })}
            options={[['all', 'All kinds'], ['experiment', 'Experiments'], ['config', 'Config values']]} />
        </div>
      </div>

      <div className="flags-rows" role="list" aria-label="Flags">
        {visible.length === 0 ? (
          <NoMatch onClear={() => setQuery((q) => ({ ...defaultFlagListQuery, sort: q.sort }))} />
        ) : visible.map((f) => (
          <FlagRow key={f.id} flag={f} selected={selectedId === f.id} onSelect={() => onSelect(f)} />
        ))}
      </div>
      {isFiltered(query) && visible.length > 0 && (
        <button onClick={() => setQuery((q) => ({ ...defaultFlagListQuery, sort: q.sort }))} style={linkButton}>
          Clear filters
        </button>
      )}
    </>
  )
}

function FlagRow({ flag, selected, onSelect }: { flag: FeatureFlag; selected: boolean; onSelect: () => void }) {
  return (
    <div role="listitem">
      <button onClick={onSelect} aria-current={selected ? 'true' : undefined} data-flag-key={flag.flag_key} style={{
        display: 'block', width: '100%', textAlign: 'left', boxSizing: 'border-box',
        padding: '0.55rem 0.75rem', marginBottom: 6, cursor: 'pointer', font: 'inherit',
        background: selected ? 'rgba(245,158,11,0.1)' : C.surface,
        border: `1px solid ${selected ? 'rgba(245,158,11,0.4)' : C.border}`,
        borderRadius: 8, color: C.text,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
          <span style={{ ...ellipsis, flex: 1, fontWeight: 700, fontSize: 14 }} title={flag.name}>{flag.name}</span>
          <span style={{ flexShrink: 0 }}>{statusBadge(flag.status)}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 2, minWidth: 0 }}>
          <code style={{ ...ellipsis, flex: 1, fontSize: 12, color: C.muted }} title={flag.flag_key}>{flag.flag_key}</code>
          {flag.origin === 'auto' && (
            <span title="Created automatically from an SDK evaluation" style={{ flexShrink: 0, fontSize: 10, fontWeight: 700, color: '#818cf8', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              auto
            </span>
          )}
          {flag.flag_kind === 'config' && (
            <span style={{ flexShrink: 0, fontSize: 10, fontWeight: 700, color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              config
            </span>
          )}
        </div>
      </button>
    </div>
  )
}

function SearchBox({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <label style={{ position: 'relative', display: 'block' }}>
      <Search size={14} style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', color: C.muted }} />
      <input type="search" value={value} onChange={(e) => onChange(e.target.value)}
        placeholder="Search name or key" aria-label="Search flags"
        style={{
          width: '100%', boxSizing: 'border-box', padding: '0.5rem 0.75rem 0.5rem 2rem',
          background: C.bg, border: `1px solid ${C.border}`, borderRadius: 8, color: C.text, fontSize: 14,
        }} />
    </label>
  )
}

function SortSelect({ value, onChange }: { value: FlagSort; onChange: (v: FlagSort) => void }) {
  return (
    <FacetSelect label="Sort" value={value} onChange={(v) => onChange(v as FlagSort)} compact
      options={[['newest', 'Newest first'], ['name', 'Name A-Z'], ['evaluated', 'Last evaluated']]} />
  )
}

function FacetSelect({ label, value, onChange, options, compact }: {
  label: string; value: string; onChange: (v: string) => void; options: [string, string][]; compact?: boolean
}) {
  return (
    <select aria-label={label} value={value} onChange={(e) => onChange(e.target.value)} style={{
      flex: compact ? undefined : 1, minWidth: 0, padding: compact ? '0.2rem 0.4rem' : '0.4rem 0.5rem',
      background: C.bg, border: `1px solid ${C.border}`, borderRadius: 6, color: compact ? C.muted : C.text, fontSize: 13,
    }}>
      {options.map(([v, text]) => <option key={v} value={v}>{text}</option>)}
    </select>
  )
}

function Chip({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button onClick={onClick} aria-pressed={active} style={{
      fontSize: 12, fontWeight: 600, padding: '0.25rem 0.6rem', borderRadius: 99, cursor: 'pointer',
      background: active ? 'rgba(245,158,11,0.12)' : 'transparent',
      border: `1px solid ${active ? 'rgba(245,158,11,0.45)' : C.border}`,
      color: active ? C.amber : C.muted,
    }}>
      {children}
    </button>
  )
}

function ListLoading() {
  return (
    <div aria-busy="true" aria-label="Loading flags" style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <Skeleton height={36} borderRadius={8} />
      {Array.from({ length: 6 }, (_, i) => <Skeleton key={i} height={52} borderRadius={8} />)}
    </div>
  )
}

function ListError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div role="alert" style={{ ...panel, borderColor: 'rgba(239,68,68,0.4)' }}>
      <div style={{ fontSize: 14, color: C.text, fontWeight: 600, marginBottom: 4 }}>Could not load flags</div>
      <div style={{ fontSize: 13, color: C.muted, overflowWrap: 'anywhere' }}>{message}</div>
      <button onClick={onRetry} style={{ ...outlineButton, marginTop: 12 }}>Retry</button>
    </div>
  )
}

function ListEmpty({ onCreate }: { onCreate: () => void }) {
  return (
    <div style={panel}>
      <Flag size={32} style={{ opacity: 0.3, marginBottom: 8 }} />
      <div style={{ fontSize: 14 }}>No flags yet</div>
      <div style={{ fontSize: 12, marginTop: 4 }}>Create one here, or evaluate a key from an SDK and it appears as auto-detected.</div>
      <button onClick={onCreate} style={{ ...outlineButton, marginTop: 12 }}>Create one</button>
    </div>
  )
}

function NoMatch({ onClear }: { onClear: () => void }) {
  return (
    <div style={panel}>
      <div style={{ fontSize: 14 }}>No flags match these filters</div>
      <button onClick={onClear} style={{ ...outlineButton, marginTop: 12 }}>Clear filters</button>
    </div>
  )
}

const ellipsis: CSSProperties = { minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }
const panel: CSSProperties = {
  background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12,
  padding: '2rem 1.25rem', textAlign: 'center', color: C.muted,
}
const outlineButton: CSSProperties = {
  background: 'transparent', border: `1px solid ${C.border}`, borderRadius: 7,
  color: C.amber, padding: '0.4rem 0.8rem', cursor: 'pointer', fontSize: 13, fontWeight: 600,
}
const linkButton: CSSProperties = {
  background: 'none', border: 'none', color: C.amber, cursor: 'pointer', fontSize: 13, fontWeight: 600, padding: '6px 0',
}
