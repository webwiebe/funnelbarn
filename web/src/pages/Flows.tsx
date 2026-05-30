import { useParams } from 'react-router-dom'
import { useState, useEffect, useRef, useCallback } from 'react'
import { api, type FlowData } from '../lib/api'
import Shell from '../components/shell/Shell'
import { SankeyChart } from '../components/flows/SankeyChart'
import { FlowBreadcrumb } from '../components/flows/FlowBreadcrumb'
import { FlowLegend } from '../components/flows/FlowLegend'
import { FlowStatsBar } from '../components/flows/FlowStatsBar'
import { C } from '../components/flows/colors'

const RANGE_OPTIONS = ['24h', '7d', '30d'] as const
type Range = (typeof RANGE_OPTIONS)[number]

function RangeSelector({ range, onChange }: { range: Range; onChange: (r: Range) => void }) {
  return (
    <div style={{ display: 'flex', gap: 4 }}>
      {RANGE_OPTIONS.map((r) => (
        <button
          key={r}
          onClick={() => onChange(r)}
          style={{
            padding: '0.35rem 0.75rem',
            borderRadius: 6,
            border: `1px solid ${range === r ? C.amber : C.border}`,
            background: range === r ? C.amberDim : 'transparent',
            color: range === r ? C.amber : C.muted,
            fontSize: 13,
            fontWeight: 500,
            cursor: 'pointer',
            minHeight: 'unset',
          }}
        >
          {r}
        </button>
      ))}
    </div>
  )
}

function ChartPlaceholder({ loading, error }: { loading: boolean; error: string | null }) {
  if (loading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 300, color: C.muted }}>
        <div style={{
          width: 28, height: 28,
          border: `3px solid ${C.border}`,
          borderTopColor: C.amber,
          borderRadius: '50%',
          animation: 'spin 0.7s linear infinite',
          marginRight: 12,
        }} />
        Loading flows…
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      </div>
    )
  }
  if (error) {
    return <div style={{ color: C.red, textAlign: 'center', padding: '2rem', fontSize: 14 }}>{error}</div>
  }
  return (
    <div style={{ color: C.muted, textAlign: 'center', padding: '3rem', fontSize: 14 }}>
      No page view data found for the selected period.
    </div>
  )
}

export default function Flows() {
  const { projectId } = useParams<{ projectId: string }>()
  const [range, setRange] = useState<Range>('7d')
  const [data, setData] = useState<FlowData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [history, setHistory] = useState<string[]>([])
  const [chartWidth, setChartWidth] = useState(900)
  const containerRef = useRef<HTMLDivElement>(null)

  const fetchFlows = useCallback(async (page?: string) => {
    if (!projectId) return
    setLoading(true)
    setError(null)
    try {
      const result = await api.getPageFlows(projectId, { range, page, depth: 5 })
      setData(result)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load flow data')
    } finally {
      setLoading(false)
    }
  }, [projectId, range])

  useEffect(() => {
    setHistory([])
    fetchFlows(undefined)
  }, [fetchFlows])

  useEffect(() => {
    if (!containerRef.current) return
    const obs = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (w && w > 0) setChartWidth(w)
    })
    obs.observe(containerRef.current)
    return () => obs.disconnect()
  }, [])

  const handleNodeClick = (page: string) => {
    if (!data || page === data.focused_page) return
    setHistory((h) => [...h, data.focused_page])
    fetchFlows(page)
  }

  const handleCrumbClick = (index: number) => {
    const page = [...history, data?.focused_page ?? ''][index]
    setHistory((h) => h.slice(0, index))
    fetchFlows(page || undefined)
  }

  const handleReset = () => {
    setHistory([])
    fetchFlows(undefined)
  }

  const handleRangeChange = (r: Range) => {
    setRange(r)
    setHistory([])
  }

  const crumbs = [...history, data?.focused_page ?? ''].filter(Boolean)
  const hasData = !loading && !error && data && data.nodes.length > 0

  return (
    <Shell projectId={projectId}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12 }}>
          <div>
            <h1 style={{ fontSize: 22, fontWeight: 700, color: C.text, margin: 0 }}>Traffic Flows</h1>
            <p style={{ fontSize: 13, color: C.muted, margin: '4px 0 0' }}>
              Click any page node to drill into its flow. Path depth: 5 hops.
            </p>
          </div>
          <RangeSelector range={range} onChange={handleRangeChange} />
        </div>

        <FlowBreadcrumb crumbs={crumbs} onCrumbClick={handleCrumbClick} onReset={handleReset} />

        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '1.5rem',
          overflowX: 'auto',
        }}>
          {hasData && data && (
            <>
              <FlowStatsBar
                focusedPage={data.focused_page}
                totalSessions={data.total_sessions}
                nodeCount={data.nodes.length}
              />
              <FlowLegend />
              <div style={{ marginBottom: 16 }} />
            </>
          )}

          <div ref={containerRef} style={{ minWidth: 600 }}>
            {hasData && data ? (
              <SankeyChart data={data} onNodeClick={handleNodeClick} width={chartWidth} />
            ) : (
              <ChartPlaceholder loading={loading} error={error} />
            )}
          </div>
        </div>

        {hasData && (
          <p style={{ fontSize: 12, color: C.muted, margin: 0, textAlign: 'center' }}>
            Click any page node to focus on it and explore its traffic paths.
          </p>
        )}
      </div>
    </Shell>
  )
}
