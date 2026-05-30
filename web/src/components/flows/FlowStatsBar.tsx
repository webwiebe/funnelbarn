import { C } from './colors'

interface Props {
  focusedPage: string
  totalSessions: number
  nodeCount: number
}

export function FlowStatsBar({ focusedPage, totalSessions, nodeCount }: Props) {
  return (
    <div style={{ display: 'flex', gap: 24, marginBottom: 20, flexWrap: 'wrap' }}>
      <div>
        <div style={{ fontSize: 11, color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          Focused page
        </div>
        <div style={{
          fontSize: 14, color: C.amber, fontWeight: 600, marginTop: 2,
          maxWidth: 320, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {focusedPage || '—'}
        </div>
      </div>
      <div>
        <div style={{ fontSize: 11, color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          Sessions
        </div>
        <div style={{ fontSize: 14, color: C.text, fontWeight: 600, marginTop: 2 }}>
          {totalSessions.toLocaleString()}
        </div>
      </div>
      <div>
        <div style={{ fontSize: 11, color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          Nodes
        </div>
        <div style={{ fontSize: 14, color: C.text, fontWeight: 600, marginTop: 2 }}>
          {nodeCount}
        </div>
      </div>
    </div>
  )
}
