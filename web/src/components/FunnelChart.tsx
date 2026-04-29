import { useEffect, useState } from 'react'
import { StepResult } from '../lib/api'

const C = {
  bg: '#0f1117',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
  success: '#10b981',
  error: '#ef4444',
}

function barColor(conversion: number) {
  if (conversion >= 0.6) return C.success
  if (conversion >= 0.3) return C.amber
  return C.error
}

interface FunnelChartProps {
  results: StepResult[]
}

export default function FunnelChart({ results }: FunnelChartProps) {
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    const t = setTimeout(() => setMounted(true), 50)
    return () => clearTimeout(t)
  }, [])

  if (!results || results.length === 0) {
    return <div style={{ color: C.muted, fontSize: 14 }}>No step data.</div>
  }

  const first = results[0]?.count ?? 1

  return (
    <div style={{ width: '100%' }}>
      {results.map((step, i) => {
        const widthPct = first > 0 ? (step.count / first) * 100 : 0
        const color = barColor(i === 0 ? 1 : step.conversion)
        const prevStep = i > 0 ? results[i - 1] : null
        const droppedAbsolute = prevStep ? prevStep.count - step.count : 0

        return (
          <div key={step.step_order}>
            {/* Drop arrow between steps */}
            {i > 0 && (
              <div style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                padding: '4px 0',
                paddingLeft: '5%',
                fontSize: 12,
                color: C.error,
              }}>
                <span>↓</span>
                <span style={{ color: C.muted }}>
                  {droppedAbsolute.toLocaleString()} dropped off
                  {' '}
                  ({prevStep && prevStep.count > 0
                    ? ((step.drop_off) * 100).toFixed(1)
                    : '0.0'}%)
                </span>
              </div>
            )}

            {/* Step bar */}
            <div style={{ marginBottom: 4 }}>
              <div style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                marginBottom: 4,
              }}>
                <div style={{
                  width: 22,
                  height: 22,
                  borderRadius: '50%',
                  background: color,
                  color: '#0f1117',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 11,
                  fontWeight: 800,
                  flexShrink: 0,
                }}>
                  {i + 1}
                </div>
                <span style={{ fontSize: 13, color: C.text, fontWeight: 600, minWidth: 140 }}>
                  {step.event_name}
                </span>
                <span style={{ fontSize: 13, color: C.muted, marginLeft: 'auto' }}>
                  {step.count.toLocaleString()} users
                  {' · '}
                  <span style={{ color, fontWeight: 700 }}>
                    {i === 0 ? '100' : (step.conversion * 100).toFixed(1)}%
                  </span>
                </span>
              </div>

              {/* The bar */}
              <div style={{
                height: 28,
                background: '#2a2d3a',
                borderRadius: 6,
                overflow: 'hidden',
                position: 'relative',
              }}>
                <div style={{
                  height: '100%',
                  width: mounted ? `${widthPct}%` : '0%',
                  background: color,
                  borderRadius: 6,
                  transition: 'width 0.6s cubic-bezier(0.4,0,0.2,1)',
                  opacity: 0.85,
                }} />
              </div>
            </div>
          </div>
        )
      })}
    </div>
  )
}
