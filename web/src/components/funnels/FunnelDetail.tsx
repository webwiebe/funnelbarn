import { useState } from 'react'
import { Video } from 'lucide-react'
import { Funnel, FunnelAnalysis } from '../../lib/api'
import { C } from '../../lib/theme'
import { ImplementationSnippet } from './ImplementationSnippet'
import { PRESET_SEGMENTS, conversionColor } from './constants'

type Step = NonNullable<FunnelAnalysis['results']>[number]

function ImplToggle({ funnel, apiKey }: { funnel?: Funnel; apiKey?: string }) {
  const [showImpl, setShowImpl] = useState(false)
  return (
    <div style={{ marginTop: '1.5rem' }}>
      <button
        onClick={() => setShowImpl((v) => !v)}
        style={{
          background: 'transparent',
          border: `1px solid ${C.border}`,
          borderRadius: 8,
          color: C.muted,
          padding: '0.5rem 1rem',
          cursor: 'pointer',
          fontSize: 13,
          display: 'flex',
          alignItems: 'center',
          gap: 6,
        }}
      >
        <span>{showImpl ? '▲' : '▼'}</span>
        How to implement
      </button>
      {showImpl && funnel && <ImplementationSnippet funnel={funnel} apiKey={apiKey} />}
    </div>
  )
}

function EmptyFunnel({ analysis, apiKey }: { analysis: FunnelAnalysis; apiKey?: string }) {
  return (
    <div>
      <div style={{ padding: '2rem', textAlign: 'center', color: C.muted, maxWidth: '100%', overflow: 'hidden' }}>
        <div style={{ fontSize: 40, marginBottom: 16 }}>📭</div>
        <div style={{ fontWeight: 700, fontSize: 16, color: C.text, marginBottom: 8, wordBreak: 'break-word' }}>
          No data yet for "{analysis.funnel?.name}"
        </div>
        <div style={{ fontSize: 14, lineHeight: 1.6, maxWidth: '100%', wordBreak: 'break-word' }}>
          Start sending events using your tracking snippet.<br />
          This funnel tracks: {analysis.funnel?.steps?.map(s => s.event_name).filter(Boolean).join(' → ') || '(no steps defined)'}
        </div>
      </div>
      <ImplToggle funnel={analysis.funnel} apiKey={apiKey} />
    </div>
  )
}

function UnmatchedWarning({ funnel }: { funnel: Funnel }) {
  const unmatched = funnel.unmatched_steps ?? []
  if (unmatched.length === 0) return null
  return (
    <div
      style={{
        background: 'rgba(239,68,68,0.08)',
        border: '1px solid rgba(239,68,68,0.35)',
        borderRadius: 10,
        padding: '0.75rem 1rem',
        marginBottom: '1.25rem',
        fontSize: 13,
        color: C.text,
      }}
    >
      <div style={{ fontWeight: 700, color: C.error, marginBottom: 4 }}>
        This funnel is not receiving events
      </div>
      <div style={{ color: C.muted, lineHeight: 1.5 }}>
        {unmatched.length === funnel.steps?.length
          ? 'None of its steps match an event this project has sent, so it can never report a conversion. '
          : 'Some of its steps match no event this project has sent, so it stalls at those steps. '}
        Never received:{' '}
        {unmatched.map((name, i) => (
          <span key={name}>
            {i > 0 && ', '}
            <code style={{ color: C.error, fontFamily: 'ui-monospace, monospace' }}>{name}</code>
          </span>
        ))}
        . Check the event names your site sends, then edit the funnel to match them.
      </div>
    </div>
  )
}

function scopeLabel(scope?: string): string {
  if (scope === 'page_view') return 'page views'
  if (scope === 'user') return 'users'
  return 'sessions'
}

function StepRow({ step, index, maxCount, onWatchRecordings }: { step: Step; index: number; maxCount: number; onWatchRecordings?: (step: number) => void }) {
  return (
    <div style={{ marginBottom: '1.25rem' }}>
      {index > 0 && step.drop_off > 0 && (
        <div style={{
          fontSize: 12,
          color: C.error,
          marginBottom: 6,
          paddingLeft: 40,
          display: 'flex',
          alignItems: 'center',
          gap: 4,
        }}>
          <span>▼</span>
          {(step.drop_off * 100).toFixed(1)}% dropped off
        </div>
      )}

      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <div style={{
          width: 32,
          height: 32,
          background: conversionColor(step.conversion),
          color: '#0f1117',
          borderRadius: '50%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontWeight: 800,
          fontSize: 13,
          flexShrink: 0,
        }}>
          {index + 1}
        </div>
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: C.text, marginBottom: 6 }}>
            {step.event_name}
          </div>
          <div style={{ height: 20, background: '#2a2d3a', borderRadius: 4, overflow: 'hidden', position: 'relative' }}>
            <div style={{
              position: 'absolute',
              inset: 0,
              width: `${(step.count / maxCount) * 100}%`,
              background: conversionColor(step.conversion),
              borderRadius: 4,
              display: 'flex',
              alignItems: 'center',
              paddingLeft: 8,
              transition: 'width 0.5s',
            }}>
              <span style={{ fontSize: 11, fontWeight: 700, color: '#0f1117', whiteSpace: 'nowrap' }}>
                {(step.conversion * 100).toFixed(1)}%
              </span>
            </div>
          </div>
        </div>
        <div style={{ textAlign: 'right', minWidth: 70 }}>
          <div style={{ fontSize: 18, fontWeight: 800, color: C.text }}>{step.count.toLocaleString()}</div>
          <div style={{ fontSize: 12, color: C.muted }}>users</div>
        </div>
        {onWatchRecordings && (
          <button
            title="Watch recordings for this step"
            onClick={() => onWatchRecordings(step.step_order)}
            style={{
              background: 'none', border: `1px solid ${C.border}`, borderRadius: 6,
              color: C.muted, cursor: 'pointer', padding: '4px 6px',
              display: 'flex', alignItems: 'center',
            }}
          >
            <Video size={13} />
          </button>
        )}
      </div>
    </div>
  )
}

function SegmentBadge({ activeSegment }: { activeSegment: string }) {
  if (activeSegment === 'all') return null
  const label = PRESET_SEGMENTS.find((s) => s.id === activeSegment)?.label ?? activeSegment
  return (
    <span style={{
      fontSize: 12,
      fontWeight: 700,
      background: 'rgba(245,158,11,0.15)',
      color: C.amber,
      border: '1px solid rgba(245,158,11,0.35)',
      borderRadius: 99,
      padding: '0.15rem 0.6rem',
    }}>
      {label}
    </span>
  )
}

function hasData(analysis: FunnelAnalysis): boolean {
  return (analysis.results?.[0]?.count ?? 0) > 0
}

export function FunnelDetail({ analysis, activeSegment, apiKey, onWatchRecordings }: { analysis: FunnelAnalysis; activeSegment: string; apiKey?: string; onWatchRecordings?: (step: number) => void }) {
  if (!hasData(analysis)) return <EmptyFunnel analysis={analysis} apiKey={apiKey} />

  const maxCount = Math.max(...(analysis.results?.map((r) => r.count) ?? [1]))
  const entryCount = analysis.results?.[0]?.count ?? 0

  return (
    <div>
      <h2 style={{ fontSize: 20, fontWeight: 800, marginBottom: 4 }}>{analysis.funnel.name}</h2>
      <p style={{ color: C.muted, fontSize: 13, marginBottom: '0.75rem' }}>
        {analysis.from?.slice(0, 10)} → {analysis.to?.slice(0, 10)}
      </p>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: '1.25rem', flexWrap: 'wrap' }}>
        <span style={{ color: C.muted, fontSize: 13 }}>
          {entryCount.toLocaleString()} {scopeLabel(analysis.funnel?.scope)}
        </span>
        <SegmentBadge activeSegment={activeSegment} />
      </div>

      <UnmatchedWarning funnel={analysis.funnel} />

      {analysis.results?.map((step, i) => (
        <StepRow key={step.step_order} step={step} index={i} maxCount={maxCount} onWatchRecordings={onWatchRecordings} />
      ))}

      <ImplToggle funnel={analysis.funnel} apiKey={apiKey} />
    </div>
  )
}
