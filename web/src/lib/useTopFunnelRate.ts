import { useEffect, useState } from 'react'
import { api } from './api'

// useTopFunnelRate is the completion rate (last step over first, in percent) of
// the project's first funnel, for the selected environment. null while loading
// or when there is nothing to show.
export function useTopFunnelRate(projectId: string | undefined, environment: string): number | null {
  const [rate, setRate] = useState<number | null>(null)

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    setRate(null)
    api.listFunnels(projectId)
      .then(async (d) => {
        const funnels = d.funnels || []
        if (funnels.length === 0) return
        const analysis = await api.getFunnelAnalysis(projectId, funnels[0].id, undefined, undefined, environment)
        if (cancelled) return
        const steps = analysis.results || []
        if (steps.length < 2) return
        const first = steps[0]?.count ?? 0
        const last = steps[steps.length - 1]?.count ?? 0
        if (first === 0) return
        setRate((last / first) * 100)
      })
      .catch(() => { if (!cancelled) setRate(null) })
    return () => { cancelled = true }
  }, [projectId, environment])

  return rate
}
