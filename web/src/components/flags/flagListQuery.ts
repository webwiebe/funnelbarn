import type { FeatureFlag } from '../../lib/api'

// The flag list's search, filters and sort. Kept free of React so the rules
// that decide which flags a user sees are tested on their own.

export type StatusFilter = 'all' | 'active' | 'paused' | 'inactive'
export type OriginFilter = 'all' | 'manual' | 'auto'
export type KindFilter = 'all' | 'experiment' | 'config'
export type FlagSort = 'newest' | 'name' | 'evaluated'

export interface FlagListQuery {
  search: string
  status: StatusFilter
  origin: OriginFilter
  kind: KindFilter
  sort: FlagSort
}

export const defaultFlagListQuery: FlagListQuery = {
  search: '', status: 'all', origin: 'all', kind: 'all', sort: 'newest',
}

export const STATUS_FILTERS: StatusFilter[] = ['all', 'active', 'paused', 'inactive']

// A flag without flag_kind predates the column and behaves as an experiment.
function flagKind(f: FeatureFlag): 'experiment' | 'config' {
  return f.flag_kind === 'config' ? 'config' : 'experiment'
}

function flagOrigin(f: FeatureFlag): 'manual' | 'auto' {
  return f.origin === 'auto' ? 'auto' : 'manual'
}

function matchesSearch(f: FeatureFlag, search: string): boolean {
  const needle = search.trim().toLowerCase()
  if (!needle) return true
  return f.name.toLowerCase().includes(needle) || f.flag_key.toLowerCase().includes(needle)
}

// matchesFacets checks everything except status, so the status chips can show
// how many flags each status would leave once the other filters apply.
function matchesFacets(f: FeatureFlag, q: FlagListQuery): boolean {
  if (q.origin !== 'all' && flagOrigin(f) !== q.origin) return false
  if (q.kind !== 'all' && flagKind(f) !== q.kind) return false
  return matchesSearch(f, q.search)
}

function timeOf(value: string | undefined): number {
  if (!value) return 0
  // SQLite returns "YYYY-MM-DD HH:MM:SS" in UTC; Date.parse wants a T and a zone.
  const t = Date.parse(/^\d{4}-\d{2}-\d{2} /.test(value) ? `${value.replace(' ', 'T')}Z` : value)
  return Number.isNaN(t) ? 0 : t
}

// flagDate renders a flag timestamp as "YYYY-MM-DD HH:MM" UTC, or null when
// there is none.
export function flagDate(value: string | undefined): string | null {
  const t = timeOf(value)
  return t === 0 ? null : new Date(t).toISOString().slice(0, 16).replace('T', ' ')
}

const comparators: Record<FlagSort, (a: FeatureFlag, b: FeatureFlag) => number> = {
  newest: (a, b) => timeOf(b.created_at) - timeOf(a.created_at),
  name: (a, b) => (a.name || a.flag_key).localeCompare(b.name || b.flag_key, undefined, { sensitivity: 'base' }),
  // Never-evaluated flags (time 0) sink to the bottom.
  evaluated: (a, b) => timeOf(b.last_evaluated_at) - timeOf(a.last_evaluated_at),
}

// applyFlagListQuery returns the flags to show, in display order. The sort is
// stable, so ties keep the order the API returned them in.
export function applyFlagListQuery(flags: FeatureFlag[], q: FlagListQuery): FeatureFlag[] {
  return flags
    .filter((f) => matchesFacets(f, q) && (q.status === 'all' || f.status === q.status))
    .sort(comparators[q.sort])
}

export function statusCounts(flags: FeatureFlag[], q: FlagListQuery): Record<StatusFilter, number> {
  const counts: Record<StatusFilter, number> = { all: 0, active: 0, paused: 0, inactive: 0 }
  for (const f of flags) {
    if (!matchesFacets(f, q)) continue
    counts.all++
    if (f.status in counts) counts[f.status as StatusFilter]++
  }
  return counts
}

export function isFiltered(q: FlagListQuery): boolean {
  return q.search.trim() !== '' || q.status !== 'all' || q.origin !== 'all' || q.kind !== 'all'
}
