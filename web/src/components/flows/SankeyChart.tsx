import { sankey, sankeyLinkHorizontal, SankeyNode, SankeyLink, SankeyGraph } from 'd3-sankey'
import { type FlowData, type FlowNode, type FlowLink } from '../../lib/api'
import { C, nodeColor, shortLabel } from './colors'

type SNode = SankeyNode<FlowNode, FlowLink>
type SLink = SankeyLink<FlowNode, FlowLink>

interface Props {
  data: FlowData
  onNodeClick: (page: string) => void
  width: number
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
  innerW,
  onNodeClick,
}: {
  node: SNode
  focusedPage: string
  innerW: number
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
  const onLeft = x0 < innerW / 2

  return (
    <g
      onClick={() => isClickable && onNodeClick(nd.label)}
      style={{ cursor: isClickable ? 'pointer' : 'default' }}
    >
      <rect x={x0} y={y0} width={x1 - x0} height={h} fill={color} opacity={isFocused ? 1 : 0.75} rx={2} />
      {isFocused && (
        <rect
          x={x0 - 1} y={y0 - 1}
          width={x1 - x0 + 2} height={h + 2}
          fill="none" stroke={C.amber} strokeWidth={2} rx={3}
        />
      )}
      <text
        x={onLeft ? x0 - 8 : x1 + 8}
        y={y0 + h / 2}
        textAnchor={onLeft ? 'end' : 'start'}
        dominantBaseline="middle"
        fill={isFocused ? C.amber : C.text}
        fontSize={11}
        fontWeight={isFocused ? 700 : 400}
        style={{ userSelect: 'none' }}
      >
        {shortLabel(nd.label)}
      </text>
      {h > 16 && (
        <text
          x={(x0 + x1) / 2}
          y={y0 + h / 2}
          textAnchor="middle"
          dominantBaseline="middle"
          fill="#fff"
          fontSize={9}
          fontWeight={600}
          style={{ userSelect: 'none' }}
        >
          {nd.sessions.toLocaleString()}
        </text>
      )}
    </g>
  )
}

function LinkPath({
  link,
  focusedPage,
}: {
  link: SLink
  focusedPage: string
}) {
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
      strokeOpacity={isFocusedLink ? 0.35 : 0.18}
      strokeWidth={Math.max(1, link.width ?? 1)}
    />
  )
}

export function SankeyChart({ data, onNodeClick, width }: Props) {
  const height = Math.max(500, data.nodes.length * 40)
  const margin = { top: 20, right: 160, bottom: 20, left: 160 }
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
    .nodeWidth(14)
    .nodePadding(12)
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
            innerW={innerW}
            onNodeClick={onNodeClick}
          />
        ))}
      </g>
    </svg>
  )
}
