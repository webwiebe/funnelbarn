import { useEffect, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { Flag, Plus } from 'lucide-react'
import Shell from '../components/shell/Shell'
import { api } from '../lib/api'
import type { FeatureFlag, TargetingRule } from '../lib/api'
import { useProjects } from '../lib/projects'
import { reportError } from '../lib/bugbarn'
import { C, FlagsTitle } from '../components/flags/shared'
import { FlagFormModal } from '../components/flags/FlagFormModal'
import { FlagList } from '../components/flags/FlagList'
import { FlagPlayground, TargetingRulesDisplay } from '../components/flags/FlagPlayground'
import { FlagEnvironments } from '../components/flags/FlagEnvironments'
import { ConfigCurrentValue } from '../components/flags/ConfigCurrentValue'
import { useFlagAnalysis } from '../components/flags/useFlagAnalysis'
import {
  AnalyticsPanel, FlagHeader, FlagMeta, SplitTiles, useFlagActions,
  alignBooleanSplit, DefaultToggle,
} from '../components/flags/FlagDetailParts'

export { alignBooleanSplit, DefaultToggle }

interface StringVariantRow {
  // The variant name is what's returned (and what targeting rules pick).
  // returnValue is only different from name when the user opts in to the
  // "use a different return value" disclosure — rare in practice.
  name: string
  returnValue: string
  splitPct: number
}

function parseJSONOr<T>(raw: string, fallback: T): T {
  try { return JSON.parse(raw) as T } catch { return fallback }
}

function defaultVariantRows(): StringVariantRow[] {
  return [
    { name: 'control', returnValue: 'control', splitPct: 50 },
    { name: 'treatment', returnValue: 'treatment', splitPct: 50 },
  ]
}

// parseFlagFormState converts a stored FeatureFlag back into the form state.
// Returns the values the modal needs to pre-fill when editing.
export function parseFlagFormState(flag: FeatureFlag): {
  defaultBool: 'on' | 'off'
  rolloutEnabled: boolean
  rolloutPct: number
  variants: StringVariantRow[]
  defaultVariant: string
  advancedReturnValues: boolean
  targetingRules: TargetingRule[]
} {
  const split = parseJSONOr<Record<string, number>>(flag.split, {})
  const variantsObj = parseJSONOr<Record<string, unknown>>(flag.variants, {})
  const rules = parseJSONOr<TargetingRule[]>(flag.targeting_rules || '[]', [])

  if (flag.flag_type === 'boolean') {
    const defaultBool: 'on' | 'off' = flag.default_variant === 'on' ? 'on' : 'off'
    const opposite = defaultBool === 'on' ? 'off' : 'on'
    const oppositePct = split[opposite] ?? 0
    const rolloutEnabled = oppositePct > 0 && oppositePct < 100
    return {
      defaultBool,
      rolloutEnabled,
      rolloutPct: rolloutEnabled ? oppositePct : 0,
      variants: defaultVariantRows(),
      defaultVariant: 'control',
      advancedReturnValues: false,
      targetingRules: rules,
    }
  }

  // String flag — rebuild variant rows from variants+split.
  const rows: StringVariantRow[] = Object.entries(variantsObj).map(([name, value]) => ({
    name,
    returnValue: typeof value === 'string' ? value : JSON.stringify(value),
    splitPct: split[name] ?? 0,
  }))
  const advanced = rows.some((r) => r.name !== r.returnValue)
  return {
    defaultBool: 'off',
    rolloutEnabled: false,
    rolloutPct: 0,
    variants: rows.length >= 2 ? rows : defaultVariantRows(),
    defaultVariant: flag.default_variant || (rows[0]?.name ?? 'control'),
    advancedReturnValues: advanced,
    targetingRules: rules,
  }
}

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      style={{
        background: 'transparent',
        border: 'none',
        borderBottom: `2px solid ${active ? C.amber : 'transparent'}`,
        color: active ? C.text : C.muted,
        fontWeight: active ? 600 : 500,
        cursor: 'pointer',
        padding: '8px 16px',
        fontSize: 14,
        marginBottom: -1,
      }}
    >
      {children}
    </button>
  )
}

export function FlagDetail({ flag, projectId, onUpdated, onDeleted }: {
  flag: FeatureFlag; projectId: string
  onUpdated: (f: FeatureFlag) => void
  onDeleted: () => void
}) {
  const { analysis, loading } = useFlagAnalysis(projectId, flag.id)
  const [tab, setTab] = useState<'analytics' | 'tryit'>('analytics')
  const [editing, setEditing] = useState(false)
  const actions = useFlagActions(flag, projectId, onUpdated, onDeleted)

  const variants = parseJSONOr<Record<string, unknown>>(flag.variants, {})
  const split = parseJSONOr<Record<string, number>>(flag.split, {})
  const isConfig = flag.flag_kind === 'config'
  const configValue = isConfig ? variants[flag.default_variant] : undefined

  return (
    <div>
      <FlagHeader flag={flag} isConfig={isConfig} configValue={configValue} actions={actions} onEdit={() => setEditing(true)} />

      <FlagMeta flag={flag} isConfig={isConfig} />

      {!isConfig && <SplitTiles flag={flag} variants={variants} split={split} />}

      {isConfig && <ConfigCurrentValue value={configValue} defaultVariant={flag.default_variant} />}

      <FlagEnvironments projectId={projectId} flag={flag} />

      <TargetingRulesDisplay rulesJSON={flag.targeting_rules} />

      <div style={{ display: 'flex', gap: 0, borderBottom: `1px solid ${C.border}`, margin: '12px 0 16px' }}>
        <TabButton active={tab === 'analytics'} onClick={() => setTab('analytics')}>Analytics</TabButton>
        <TabButton active={tab === 'tryit'} onClick={() => setTab('tryit')}>Try it</TabButton>
      </div>

      {tab === 'tryit'
        ? <FlagPlayground projectId={projectId} flag={flag} />
        : <AnalyticsPanel analysis={analysis} loading={loading} />}

      {editing && (
        <FlagFormModal
          projectId={projectId}
          flag={flag}
          onClose={() => setEditing(false)}
          onSaved={(f) => { onUpdated(f); setEditing(false) }}
        />
      )}
    </div>
  )
}

// Wide screens: both columns stick below the page header and fill the
// viewport, the rows scroll inside the list column and the detail pane stays
// in view whichever flag is picked. Phones: one column, the detail replaces
// the list and Back returns to the same scroll position.
const LAYOUT_CSS = `
  .flags-layout { display: grid; grid-template-columns: minmax(260px, 340px) minmax(0, 1fr); gap: 1.5rem; align-items: start; }
  @media (min-width: 768px) {
    .flags-list, .flags-detail { position: sticky; top: 1rem; max-height: calc(100vh - 2rem); box-sizing: border-box; }
    .flags-list { display: flex; flex-direction: column; min-width: 0; }
    .flags-rows { flex: 1 1 auto; min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding-right: 4px; }
    .flags-detail { overflow-y: auto; }
    .back-btn { display: none !important; }
  }
  @media (max-width: 767px) {
    .flags-layout { grid-template-columns: minmax(0, 1fr); }
    .flags-list.detail-open { display: none; }
    .flags-detail.no-selection { display: none; }
    .back-btn { display: block !important; }
  }
`

const isNarrow = () => typeof window !== 'undefined' && !!window.matchMedia?.('(max-width: 767px)').matches

// useProjectFlags loads a project's flags. flags is null until the first
// response; a failed load sets error instead of reading as an empty project.
function useProjectFlags(projectId: string | undefined) {
  const [flags, setFlags] = useState<FeatureFlag[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    setFlags(null)
    setError(null)
    api.listFlags(projectId)
      .then((d) => { if (!cancelled) setFlags(d.flags || []) })
      .catch((e) => {
        reportError(e instanceof Error ? e : new Error(String(e)), { source: 'Flags' })
        if (!cancelled) setError(e instanceof Error ? e.message : String(e))
      })
    return () => { cancelled = true }
  }, [projectId, attempt])

  return { flags, setFlags, error, retry: () => setAttempt((n) => n + 1) }
}

export default function Flags() {
  const { projectId } = useParams<{ projectId?: string }>()
  const { projects } = useProjects()
  const [searchParams, setSearchParams] = useSearchParams()
  const { flags, setFlags, error, retry } = useProjectFlags(projectId)
  const [showCreate, setShowCreate] = useState(false)
  const listScroll = useRef(0)

  const selectedKey = searchParams.get('flag')
  const selected = flags?.find((f) => f.flag_key === selectedKey) ?? null

  const select = (f: FeatureFlag) => {
    listScroll.current = window.scrollY
    setSearchParams({ flag: f.flag_key }, { replace: true })
    if (isNarrow()) window.scrollTo(0, 0)
  }
  const clearSelection = () => {
    setSearchParams({}, { replace: true })
    if (isNarrow()) requestAnimationFrame(() => window.scrollTo(0, listScroll.current))
  }

  if (!projectId) {
    return (
      <Shell>
        <div style={{ color: C.muted, padding: '2rem', textAlign: 'center' }}>Select a project to manage flags.</div>
      </Shell>
    )
  }

  const projectName = projects.find((p) => p.id === projectId)?.name

  return (
    <Shell projectId={projectId} projectName={projectName}>
      <style>{LAYOUT_CSS}</style>

      {showCreate && (
        <FlagFormModal projectId={projectId} onClose={() => setShowCreate(false)}
          onSaved={(f) => { setFlags((prev) => [f, ...(prev ?? [])]); setShowCreate(false); select(f) }} />
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: '1.25rem' }}>
        <FlagsTitle projectId={projectId} active="flags" />
        <button onClick={() => setShowCreate(true)} style={{
          display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0,
          background: C.amber, border: 'none', borderRadius: 8,
          color: '#0f1117', padding: '0.6rem 1.1rem', cursor: 'pointer', fontSize: 14, fontWeight: 700,
        }}>
          <Plus size={16} /> Create flag
        </button>
      </div>

      <div className="flags-layout">
        <div className={`flags-list${selected ? ' detail-open' : ''}`}>
          <FlagList flags={flags} error={error} onRetry={retry} selectedId={selected?.id ?? null}
            onSelect={select} onCreate={() => setShowCreate(true)} />
        </div>

        <div className={`flags-detail${!selected ? ' no-selection' : ''}`} style={{
          background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12,
          padding: '1.5rem', minHeight: 300,
        }}>
          <button className="back-btn" onClick={clearSelection} style={{
            display: 'none', background: 'none', border: 'none', color: C.amber,
            cursor: 'pointer', fontSize: 14, fontWeight: 600, padding: '0 0 1rem 0',
          }}>
            ← Back
          </button>
          {!selected ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: 300, color: C.muted, gap: 8 }}>
              <Flag size={40} opacity={0.3} />
              <div>Select a flag to view results</div>
            </div>
          ) : (
            <FlagDetail flag={selected} projectId={projectId}
              onUpdated={(f) => { setFlags((prev) => prev?.map((x) => x.id === f.id ? f : x) ?? prev); setSearchParams({ flag: f.flag_key }, { replace: true }) }}
              onDeleted={() => { setFlags((prev) => prev?.filter((x) => x.id !== selected.id) ?? prev); clearSelection() }}
            />
          )}
        </div>
      </div>
    </Shell>
  )
}
