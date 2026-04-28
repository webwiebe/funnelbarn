import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'

interface FunnelStep {
  step_order: number
  event_name: string
}

interface Funnel {
  id: string
  name: string
  description?: string
  steps: FunnelStep[]
}

interface StepResult {
  StepOrder: number
  EventName: string
  Count: number
  Conversion: number
  DropOff: number
}

interface FunnelAnalysis {
  funnel: Funnel
  results: StepResult[]
  from: string
  to: string
}

export default function Funnels() {
  const { projectId } = useParams<{ projectId?: string }>()
  const [funnels, setFunnels] = useState<Funnel[]>([])
  const [selected, setSelected] = useState<Funnel | null>(null)
  const [analysis, setAnalysis] = useState<FunnelAnalysis | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!projectId) return
    fetch(`/api/v1/projects/${projectId}/funnels`)
      .then((r) => r.json())
      .then((d) => setFunnels(d.funnels || []))
      .catch(console.error)
  }, [projectId])

  const loadAnalysis = (funnel: Funnel) => {
    if (!projectId) return
    setSelected(funnel)
    setLoading(true)
    fetch(`/api/v1/projects/${projectId}/funnels/${funnel.id}/analysis`)
      .then((r) => r.json())
      .then(setAnalysis)
      .catch(console.error)
      .finally(() => setLoading(false))
  }

  return (
    <div style={{ fontFamily: 'sans-serif', padding: '2rem', maxWidth: 1200, margin: '0 auto' }}>
      <h1>Funnels</h1>

      {!projectId && <p style={{ color: '#6b7280' }}>Select a project first.</p>}

      {projectId && funnels.length === 0 && (
        <p style={{ color: '#6b7280' }}>No funnels yet. Create one via the API.</p>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '2rem' }}>
        <div>
          {funnels.map((f) => (
            <div
              key={f.id}
              onClick={() => loadAnalysis(f)}
              style={{
                padding: '0.75rem 1rem',
                marginBottom: '0.5rem',
                cursor: 'pointer',
                background: selected?.id === f.id ? '#eff6ff' : '#f9fafb',
                border: `1px solid ${selected?.id === f.id ? '#bfdbfe' : '#e5e7eb'}`,
                borderRadius: 6,
              }}
            >
              <div style={{ fontWeight: 600 }}>{f.name}</div>
              <div style={{ fontSize: 12, color: '#6b7280' }}>{f.steps?.length ?? 0} steps</div>
            </div>
          ))}
        </div>

        <div>
          {loading && <p>Loading analysis…</p>}
          {analysis && (
            <>
              <h2>{analysis.funnel.name}</h2>
              <p style={{ color: '#6b7280', fontSize: 13 }}>
                {analysis.from.slice(0, 10)} → {analysis.to.slice(0, 10)}
              </p>

              <div>
                {analysis.results?.map((step, i) => (
                  <div
                    key={step.StepOrder}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: '1rem',
                      marginBottom: '1rem',
                    }}
                  >
                    <div
                      style={{
                        width: 32,
                        height: 32,
                        background: '#2563eb',
                        color: 'white',
                        borderRadius: '50%',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        fontWeight: 700,
                        flexShrink: 0,
                      }}
                    >
                      {i + 1}
                    </div>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontWeight: 600 }}>{step.EventName}</div>
                      <div
                        style={{
                          height: 8,
                          background: '#e5e7eb',
                          borderRadius: 4,
                          overflow: 'hidden',
                          marginTop: 4,
                        }}
                      >
                        <div
                          style={{
                            height: '100%',
                            width: `${(step.Conversion * 100).toFixed(1)}%`,
                            background: '#2563eb',
                            borderRadius: 4,
                          }}
                        />
                      </div>
                    </div>
                    <div style={{ textAlign: 'right', minWidth: 80 }}>
                      <div style={{ fontWeight: 700 }}>{step.Count.toLocaleString()}</div>
                      <div style={{ fontSize: 12, color: '#6b7280' }}>
                        {(step.Conversion * 100).toFixed(1)}%
                      </div>
                    </div>
                    {i > 0 && step.DropOff > 0 && (
                      <div style={{ fontSize: 12, color: '#ef4444', minWidth: 60 }}>
                        -{(step.DropOff * 100).toFixed(1)}%
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
