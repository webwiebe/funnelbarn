import { describe, it, expect } from 'vitest'
import { parseFunnelLink, withFunnelLink } from './funnelLink'

const parse = (search: string) => parseFunnelLink(new URLSearchParams(search))

describe('parseFunnelLink', () => {
  it('reads funnel, breakdown and group', () => {
    expect(parse('?funnel=f1&breakdown=signup_surface&group=wordpress')).toEqual({
      funnel: 'f1', breakdown: 'signup_surface', group: 'wordpress',
    })
  })

  it('returns empty fields for a bare URL', () => {
    expect(parse('')).toEqual({ funnel: '', breakdown: '', group: '' })
  })

  it('drops a breakdown name the API would reject, and its group', () => {
    for (const bad of ['signup-surface', 'a b', '$.x', "x')--"]) {
      const search = new URLSearchParams({ funnel: 'f1', breakdown: bad, group: 'wordpress' })
      expect(parseFunnelLink(search)).toEqual({ funnel: 'f1', breakdown: '', group: '' })
    }
  })

  it('keeps a group value as written', () => {
    expect(parse('?breakdown=ref&group=site%20badge%2F2').group).toBe('site badge/2')
  })
})

describe('withFunnelLink', () => {
  it('sets the given fields and keeps other parameters', () => {
    const next = withFunnelLink(new URLSearchParams('?tab=x&funnel=f1'), { breakdown: 'plan' })
    expect(next.toString()).toBe('tab=x&funnel=f1&breakdown=plan')
  })

  it('removes fields set to an empty value', () => {
    const next = withFunnelLink(new URLSearchParams('?funnel=f1&breakdown=plan&group=pro'), { breakdown: '', group: '' })
    expect(next.toString()).toBe('funnel=f1')
  })

  it('round-trips a link through the URL', () => {
    const link = { funnel: 'f1', breakdown: 'signup_surface', group: 'wordpress plugin' }
    expect(parseFunnelLink(withFunnelLink(new URLSearchParams(), link))).toEqual(link)
  })
})
