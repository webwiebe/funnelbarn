import { sankey, sankeyLinkHorizontal, SankeyNode, SankeyLink, SankeyGraph } from 'd3-sankey'
import { type FlowData, type FlowNode, type FlowLink } from '../../lib/api'
import { C, nodeColor } from './colors'

type SNode = SankeyNode<FlowNode, FlowLink>
type SLink = SankeyLink<FlowNode, FlowLink>

interface Props {
  data: FlowData
  onNodeClick: (page: string) => void
  width: number
}

// Strip the domain and show only the last meaningful path segment.
// For referrer domains and special nodes ("%drop-off)", "(direct)") keep as-is.
function pathLabel(label: string, maxLen = 14): string {
  if (label.startsWith('(')) return label
  try {
    const { pathname } = new URL(label)
    if (!pathname || pathname === '/') return '/'
    const parts = pathname.split('/').filter(Boolean)
    const segment = parts[parts.length - 1]
    return segment.length > maxLen ? segment.slice(0, maxLen - 1) + '…' : segment
  } catch {
    // Referrer domain or non-URL string
    return label.length > maxLen ? label.slice(0, maxLen - 1) + '…' : label
  }
}

function buildSankeyGraph(data: FlowData): {
  nodes: (FlowNode & { name: string })[]
  links: { source: number; target: number; value: number }[]
} | null {
  const nodeById = new Map<string, number>()
  const nodes: (FlowNode & { name: string })[] = data.nodes.map((n, i) => {
    nodeById.set(n.id, i)
    return { ...n, name: n.id }
  })

  const links = data.links
    .filter((l) => nodeById.has(l.source) && nodeById.has(l.target))
    .map((l) => ({
      ...l,
      source: nodeById.get(l.source)!,
      target: nodeById.get(l.target)!,
      value: Math.max(l.value, 1),
    }))

  if (nodes.length === 0 || links.length === 0) return null
  return { nodes, links }
}

function NodeRect({
  node,
  focusedPage,
  innerH,
  onNodeClick,
}: {
  node: SNode
  focusedPage: string
  innerH: number
  onNodeClick: (page: string) => void
}) {
  const nd = node as unknown as FlowNode
  const x0 = node.x0 ?? 0
  const x1 = node.x1 ?? 0
  const y0 = node.y0 ?? 0
  const y1 = node.y1 ?? 0
  const h = Math.max(4, y1 - y0)
  const isFocused = nd.depth === 0 && nd.label === focusedPage
  const color = nodeColor(nd.type, isFocused)
  const isClickable = nd.type !== 'exit'
  const cx = (x0 + x1) / 2

  return (
    <g
      onClick={() => isClickable && onNodeClick(nd.label)}
      style={{ cursor: isClickable ? 'pointer' : 'default' }}
    >
      {/* Full URL tooltip on hover */}
      <title>{nd.label}</title>

      {/* Node bar */}
      <rect x={x0} y={y0} width={x1 - x0} height={h} fill={color} opacity={isFocused ? 1 : 0.75} rx={2} />

      {/* Focused-page highlight ring */}
      {isFocused && (
        <rect
          x={x0 - 2} y={y0 - 2}
          width={x1 - x0 + 4} height={h + 4}
          fill="none" stroke={C.amber} strokeWidth={2} rx={3}
        />
      )}

      {/* Session count badge — top of bar so it's readable even on full-height nodes */}
      {h > 24 && (
        <text
          x={cx} y={y0 + 13}
          textAnchor="middle"
          fill="#fff"
          fontSize={10}
          fontWeight={600}
          style={{ userSelect: 'none' }}
        >
          {nd.sessions.toLocaleString()}
        </text>
      )}

      {/* Label below the chart area, rotated 45° so adjacent columns don't overlap */}
      <text
        transform={`translate(${cx},${innerH + 10}) rotate(45)`}
        textAnchor="start"
        dominantBaseline="hanging"
        fill={isFocused ? C.amber : C.text}
        fontSize={11}
        fontWeight={isFocused ? 700 : 400}
        style={{ userSelect: 'none' }}
      >
        {pathLabel(nd.label)}
      </text>
    </g>
  )
}

function LinkPath({ link, focusedPage }: { link: SLink; focusedPage: string }) {
  const linkPath = sankeyLinkHorizontal()
  const src = link.source as SNode
  const tgt = link.target as SNode
  const srcData = src as unknown as FlowNode
  const tgtData = tgt as unknown as FlowNode
  const isFocusedLink =
    (srcData.depth === 0 && srcData.label === focusedPage) ||
    (tgtData.depth === 0 && tgtData.label === focusedPage)

  return (
    <path
      d={linkPath(link) ?? ''}
      fill="none"
      stroke={isFocusedLink ? C.amber : C.muted}
      strokeOpacity={isFocusedLink ? 0.4 : 0.2}
      strokeWidth={Math.max(1, link.width ?? 1)}
    />
  )
}

export function SankeyChart({ data, onNodeClick, width }: Props) {
  // Small side margins — labels are below the chart now, not to the sides.
  const margin = { top: 20, right: 50, bottom: 130, left: 50 }
  // Height grows with node count but stays in a sensible range.
  const height = Math.max(320, Math.min(data.nodes.length * 28 + 200, 520))
  const innerW = width - margin.left - margin.right
  const innerH = height - margin.top - margin.bottom

  if (innerW <= 0) return null

  const graph = buildSankeyGraph(data)
  if (!graph) {
    return (
      <div style={{ color: C.muted, textAlign: 'center', padding: '2rem' }}>
        Not enough flow data to render a graph for this page in the selected period.
      </div>
    )
  }

  const sankeyLayout = sankey<FlowNode & { name: string }, FlowLink>()
    .nodeWidth(16)
    .nodePadding(10)
    .extent([[0, 0], [innerW, innerH]])

  let layout: SankeyGraph<FlowNode & { name: string }, FlowLink>
  try {
    layout = sankeyLayout(graph as SankeyGraph<FlowNode & { name: string }, FlowLink>)
  } catch {
    return (
      <div style={{ color: C.muted, textAlign: 'center', padding: '2rem' }}>
        Not enough flow data to render a graph for this page in the selected period.
      </div>
    )
  }

  return (
    <svg width={width} height={height} style={{ overflow: 'visible', display: 'block' }}>
      <g transform={`translate(${margin.left},${margin.top})`}>
        {layout.links.map((link, i) => (
          <LinkPath key={i} link={link as SLink} focusedPage={data.focused_page} />
        ))}
        {layout.nodes.map((node, i) => (
          <NodeRect
            key={i}
            node={node as SNode}
            focusedPage={data.focused_page}
            innerH={innerH}
            onNodeClick={onNodeClick}
          />
        ))}
      </g>
    </svg>
  )
}
