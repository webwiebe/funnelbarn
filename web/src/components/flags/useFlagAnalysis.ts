import { useEffect, useState } from 'react'
import { api } from '../../lib/api'
import type { FlagAnalysis } from '../../lib/api'
import { reportError } from '../../lib/bugbarn'

// useFlagAnalysis loads the analysis report of one flag. Switching flags drops
// the previous report first, so a failed request leaves an empty panel, and a
// late response for a flag that is no longer shown is ignored.
export function useFlagAnalysis(projectId: string, flagId: string) {
  const [analysis, setAnalysis] = useState<FlagAnalysis | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setAnalysis(null)
    setLoading(true)
    api.getFlagAnalysis(projectId, flagId)
      .then((a) => { if (!cancelled) setAnalysis(a) })
      .catch((e) => { if (!cancelled) { console.error(e); reportError(e, { source: 'Flags' }) } })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [projectId, flagId])

  return { analysis, loading }
}
