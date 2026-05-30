import { useParams } from 'react-router-dom'
import { useState, useEffect, useRef, useCallback } from 'react'
import { ChevronRight, Home, RotateCcw } from 'lucide-react'
import { sankey, sankeyLinkHorizontal, SankeyNode, SankeyLink, SankeyGraph } from 'd3-sankey'
import { api, type FlowData, type FlowNode, type FlowLink } from '../lib/api'
import Shell from '../components/shell/Shell'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  amberDim: 'rgba(245,158,11,0.15)',
  text: '#e2e8f0',
  muted: '#94a3b8',
  blue: '#3b82f6',
  green: '#10b981',
  red: '#ef4444',
}

const RANGE_OPTIONS = ['24h', '7d', '30d'] as const
type Range = (typeof RANGE_OPTIONS)[number]

// d3-sankey node/link types augmented with our data
type SNode = SankeyNode<FlowNode, FlowLink>
type SLink = SankeyLink<FlowNode, FlowLink>

function nodeColor(node: FlowNode, focusedPage: string): string {
  if (node.label === focusedPage && node.depth === 0) return C.amber
  if (node.type === 'referrer') return C.blue
  if (node.type === 'exit') return C.red
  if (node.depth < 0) return '#6366f1' // indigo for inbound pages
  return C.green // outbound pages
}

function shortLabel(label: string, maxLen = 28): string {
  if (label.length <= maxLen) return label
  return '…' + label.slice(-(maxLen - 1))
}

interface SankeyChartProps {
  data: FlowData
  onNodeClick: (page: string) => void
  width: number
}

function SankeyChart({ data, onNodeClick, width }: SankeyChartProps) {
  const height = Math.max(500, data.nodes.length * 40)
  const margin = { top: 20, right: 160, bottom: 20, left: 160 }
  const innerW = width - margin.left - margin.right
  const innerH = height - margin.top - margin.bottom

  if (innerW <= 0 || data.nodes.length === 0) return null

  // Build d3-sankey graph
  // Nodes need to be keyed by id for the link source/target lookup
  const nodeById = new Map<string, number>()
  const sankeyNodes: (FlowNode & { name: string })[] = data.nodes.map((n, i) => {
    nodeById.set(n.id, i)
    return { ...n, name: n.id }
  })

  const sankeyLinks = data.links
    .filter((l) => nodeById.has(l.source) && nodeById.has(l.target))
    .map((l) => ({
      ...l,
      source: nodeById.get(l.source)!,
      target: nodeById.get(l.target)!,
      value: Math.max(l.value, 1),
    }))

  const graph: SankeyGraph<FlowNode & { name: string }, FlowLink> = {
    nodes: sankeyNodes,
    links: sankeyLinks as unknown as SankeyLink<FlowNode & { name: string }, FlowLink>[],
  }

  const sankeyLayout = sankey<FlowNode & { name: string }, FlowLink>()
    .nodeWidth(14)
    .nodePadding(12)
    .extent([[0, 0], [innerW, innerH]])

  let layout: SankeyGraph<FlowNode & { name: string }, FlowLink>
  try {
    layout = sankeyLayout(graph)
  } catch {
    return (
      <div style={{ color: C.muted, textAlign: 'center', padding: '2rem' }}>
        Not enough flow data to render a graph for this page in the selected period.
      </div>
    )
  }

  const linkPath = sankeyLinkHorizontal()

  return (
    <svg width={width} height={height} style={{ overflow: 'visible', display: 'block' }}>
      <g transform={`translate(${margin.left},${margin.top})`}>
        {/* Links */}
        {layout.links.map((link, i) => {
          const srcNode = link.source as SNode
          const tgtNode = link.target as SNode
          const srcData = srcNode as unknown as FlowNode
          const tgtData = tgtNode as unknown as FlowNode
          const isFocusedLink =
            (srcData.depth === 0 && srcData.label === data.focused_page) ||
            (tgtData.depth === 0 && tgtData.label === data.focused_page)
          return (
            <path
              key={i}
              d={linkPath(link as SLink) ?? ''}
              fill="none"
              stroke={isFocusedLink ? C.amber : '#94a3b8'}
              strokeOpacity={isFocusedLink ? 0.35 : 0.18}
              strokeWidth={Math.max(1, (link as SLink).width ?? 1)}
            />
          )
        })}

        {/* Nodes */}
        {layout.nodes.map((node, i) => {
          const nd = node as unknown as FlowNode
          const x0 = node.x0 ?? 0
          const x1 = node.x1 ?? 0
          const y0 = node.y0 ?? 0
          const y1 = node.y1 ?? 0
          const nodeH = Math.max(4, y1 - y0)
          const color = nodeColor(nd, data.focused_page)
          const isClickable = nd.type !== 'exit'
          const isFocused = nd.depth === 0 && nd.label === data.focused_page

          return (
            <g
              key={i}
              onClick={() => isClickable && onNodeClick(nd.label)}
              style={{ cursor: isClickable ? 'pointer' : 'default' }}
            >
              <rect
                x={x0}
                y={y0}
                width={x1 - x0}
                height={nodeH}
                fill={color}
                opacity={isFocused ? 1 : 0.75}
                rx={2}
              />
              {isFocused && (
                <rect
                  x={x0 - 1}
                  y={y0 - 1}
                  width={x1 - x0 + 2}
                  height={nodeH + 2}
                  fill="none"
                  stroke={C.amber}
                  strokeWidth={2}
                  rx={3}
                />
              )}
              {/* Label — left side (inbound) or right side (outbound/focused) */}
              {x0 < innerW / 2 ? (
                // Node is on the left half: label goes to the left
                <text
                  x={x0 - 8}
                  y={y0 + nodeH / 2}
                  textAnchor="end"
                  dominantBaseline="middle"
                  fill={isFocused ? C.amber : C.text}
                  fontSize={11}
                  fontWeight={isFocused ? 700 : 400}
                  style={{ userSelect: 'none' }}
                >
                  {shortLabel(nd.label)}
                </text>
              ) : (
                // Node is on the right half: label goes to the right
                <text
                  x={x1 + 8}
                  y={y0 + nodeH / 2}
                  textAnchor="start"
                  dominantBaseline="middle"
                  fill={isFocused ? C.amber : C.text}
                  fontSize={11}
                  fontWeight={isFocused ? 700 : 400}
                  style={{ userSelect: 'none' }}
                >
                  {shortLabel(nd.label)}
                </text>
              )}
              {/* Session count badge */}
              <text
                x={(x0 + x1) / 2}
                y={y0 + nodeH / 2}
                textAnchor="middle"
                dominantBaseline="middle"
                fill="#fff"
                fontSize={9}
                fontWeight={600}
                opacity={nodeH > 16 ? 0.9 : 0}
                style={{ userSelect: 'none' }}
              >
                {nd.sessions.toLocaleString()}
              </text>
            </g>
          )
        })}
      </g>
    </svg>
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

  // Current focused page (derived from data)
  const focusedPage = data?.focused_page ?? ''

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

  // Initial load and range change: reset history, use default page
  useEffect(() => {
    setHistory([])
    fetchFlows(undefined)
  }, [fetchFlows])

  // Observe container width for responsive SVG
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
    if (page === focusedPage) return
    setHistory((h) => [...h, focusedPage])
    fetchFlows(page)
  }

  const handleBreadcrumbClick = (index: number) => {
    const page = history[index]
    setHistory((h) => h.slice(0, index))
    fetchFlows(page || undefined)
  }

  const handleReset = () => {
    setHistory([])
    fetchFlows(undefined)
  }

  const breadcrumbs = [...history, focusedPage].filter(Boolean)

  return (
    <Shell projectId={projectId}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12 }}>
          <div>
            <h1 style={{ fontSize: 22, fontWeight: 700, color: C.text, margin: 0 }}>Traffic Flows</h1>
            <p style={{ fontSize: 13, color: C.muted, margin: '4px 0 0' }}>
              Click any page node to drill into its flow. Path depth: 5 hops.
            </p>
          </div>

          {/* Range selector */}
          <div style={{ display: 'flex', gap: 4 }}>
            {RANGE_OPTIONS.map((r) => (
              <button
                key={r}
                onClick={() => setRange(r)}
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
        </div>

        {/* Breadcrumb trail */}
        {breadcrumbs.length > 0 && (
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            flexWrap: 'wrap',
            background: C.surface,
            border: `1px solid ${C.border}`,
            borderRadius: 8,
            padding: '0.5rem 0.75rem',
          }}>
            <button
              onClick={handleReset}
              title="Reset to top page"
              style={{
                background: 'none', border: 'none', cursor: 'pointer',
                color: C.muted, display: 'flex', alignItems: 'center', padding: 0, minHeight: 'unset',
              }}
            >
              <Home size={14} />
            </button>
            {breadcrumbs.map((crumb, i) => (
              <span key={i} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                <ChevronRight size={12} color={C.muted} />
                <button
                  onClick={() => handleBreadcrumbClick(i)}
                  style={{
                    background: 'none',
                    border: 'none',
                    cursor: i < breadcrumbs.length - 1 ? 'pointer' : 'default',
                    color: i === breadcrumbs.length - 1 ? C.amber : C.muted,
                    fontSize: 12,
                    fontWeight: i === breadcrumbs.length - 1 ? 600 : 400,
                    padding: 0,
                    minHeight: 'unset',
                    maxWidth: 240,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {crumb}
                </button>
              </span>
            ))}
            {history.length > 0 && (
              <button
                onClick={handleReset}
                style={{
                  marginLeft: 'auto',
                  background: 'none',
                  border: `1px solid ${C.border}`,
                  borderRadius: 4,
                  cursor: 'pointer',
                  color: C.muted,
                  fontSize: 11,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 4,
                  padding: '0.2rem 0.5rem',
                  minHeight: 'unset',
                }}
              >
                <RotateCcw size={11} />
                Reset
              </button>
            )}
          </div>
        )}

        {/* Chart card */}
        <div style={{
          background: C.surface,
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          padding: '1.5rem',
          overflowX: 'auto',
        }}>
          {/* Stats bar */}
          {data && !loading && (
            <div style={{ display: 'flex', gap: 24, marginBottom: 20, flexWrap: 'wrap' }}>
              <div>
                <div style={{ fontSize: 11, color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>Focused page</div>
                <div style={{ fontSize: 14, color: C.amber, fontWeight: 600, marginTop: 2, maxWidth: 320, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {data.focused_page || '—'}
                </div>
              </div>
              <div>
                <div style={{ fontSize: 11, color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>Sessions</div>
                <div style={{ fontSize: 14, color: C.text, fontWeight: 600, marginTop: 2 }}>
                  {data.total_sessions.toLocaleString()}
                </div>
              </div>
              <div>
                <div style={{ fontSize: 11, color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>Nodes</div>
                <div style={{ fontSize: 14, color: C.text, fontWeight: 600, marginTop: 2 }}>
                  {data.nodes.length}
                </div>
              </div>
            </div>
          )}

          {/* Legend */}
          {data && !loading && data.nodes.length > 0 && (
            <div style={{ display: 'flex', gap: 16, marginBottom: 16, flexWrap: 'wrap' }}>
              {[
                { color: C.amber, label: 'Focused page' },
                { color: '#6366f1', label: 'Inbound pages' },
                { color: C.green, label: 'Outbound pages' },
                { color: C.blue, label: 'Entry referrers' },
                { color: C.red, label: 'Drop-off' },
              ].map(({ color, label }) => (
                <div key={label} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: C.muted }}>
                  <div style={{ width: 10, height: 10, borderRadius: 2, background: color }} />
                  {label}
                </div>
              ))}
            </div>
          )}

          <div ref={containerRef} style={{ minWidth: 600 }}>
            {loading && (
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
            )}

            {error && !loading && (
              <div style={{ color: C.red, textAlign: 'center', padding: '2rem', fontSize: 14 }}>{error}</div>
            )}

            {!loading && !error && data && data.nodes.length === 0 && (
              <div style={{ color: C.muted, textAlign: 'center', padding: '3rem', fontSize: 14 }}>
                No page view data found for the selected period.
              </div>
            )}

            {!loading && !error && data && data.nodes.length > 0 && (
              <SankeyChart
                data={data}
                onNodeClick={handleNodeClick}
                width={chartWidth}
              />
            )}
          </div>
        </div>

        {/* Hint */}
        {data && !loading && data.nodes.length > 0 && (
          <p style={{ fontSize: 12, color: C.muted, margin: 0, textAlign: 'center' }}>
            Click any page node to focus on it and explore its traffic paths.
          </p>
        )}
      </div>
    </Shell>
  )
}
