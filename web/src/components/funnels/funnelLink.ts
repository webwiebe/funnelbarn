import { useCallback, useEffect, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { Funnel } from '../../lib/api'

// Deep link to a funnel view: ?funnel=<id>&breakdown=<property>&group=<value>.
// Other services (BugBarn's sign-up surface rows) link straight to one group of
// a breakdown, and the page writes its own state back so a copied URL
// reproduces the view.
export type FunnelLink = { funnel: string; breakdown: string; group: string }

// Same rule as the API applies to the breakdown property.
const propertyName = /^[A-Za-z0-9_]+$/

// A breakdown name the API would reject is dropped, and with it the group,
// since a group only means something within its breakdown.
export function parseFunnelLink(params: URLSearchParams): FunnelLink {
  const breakdown = params.get('breakdown') ?? ''
  const valid = propertyName.test(breakdown)
  return {
    funnel: params.get('funnel') ?? '',
    breakdown: valid ? breakdown : '',
    group: valid ? (params.get('group') ?? '') : '',
  }
}

// Sets the given link fields and removes the empty ones; other parameters stay.
export function withFunnelLink(params: URLSearchParams, link: Partial<FunnelLink>): URLSearchParams {
  const next = new URLSearchParams(params)
  for (const [key, value] of Object.entries(link)) {
    if (value) next.set(key, value)
    else next.delete(key)
  }
  return next
}

// The link fields of the current URL, and a setter that replaces the history
// entry so stepping through breakdowns does not fill the back button.
export function useFunnelLink(): [FunnelLink, (link: Partial<FunnelLink>) => void] {
  const [params, setParams] = useSearchParams()
  const update = useCallback(
    (link: Partial<FunnelLink>) => setParams((current) => withFunnelLink(current, link), { replace: true }),
    [setParams],
  )
  return [parseFunnelLink(params), update]
}

// Funnels page side: opens the linked funnel once the list has loaded, then
// keeps ?funnel= in step with the selection. Choosing another funnel drops the
// breakdown and group, which belong to the previous one.
export function useFunnelDeepLink(funnels: Funnel[], selected: Funnel | null, open: (f: Funnel) => void) {
  const [link, update] = useFunnelLink()
  const applied = useRef(false)
  useEffect(() => {
    if (applied.current || selected || funnels.length === 0) return
    applied.current = true
    const linked = funnels.find((f) => f.id === link.funnel)
    if (linked) open(linked)
  }, [funnels, selected, link.funnel, open])
  useEffect(() => {
    if (!selected || selected.id === link.funnel) return
    update({ funnel: selected.id, breakdown: '', group: '' })
  }, [selected, link.funnel, update])
}
