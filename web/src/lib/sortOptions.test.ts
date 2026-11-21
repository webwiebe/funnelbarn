import { describe, expect, it } from 'vitest'
import { sortByLabel, sortByLabelPinned, sortStrings } from './sortOptions'

describe('sortOptions', () => {
  it('sorts case-insensitively', () => {
    expect(sortStrings(['banana', 'Apple', 'cherry'])).toEqual(['Apple', 'banana', 'cherry'])
  })

  it('sorts digits in natural order', () => {
    expect(sortStrings(['step 10', 'step 2', 'step 1'])).toEqual(['step 1', 'step 2', 'step 10'])
  })

  it('sorts by the label and leaves the input untouched', () => {
    const input = [{ n: 'b' }, { n: 'a' }]
    expect(sortByLabel(input, (i) => i.n)).toEqual([{ n: 'a' }, { n: 'b' }])
    expect(input[0].n).toBe('b')
  })

  it('keeps the pinned entry first', () => {
    const out = sortByLabelPinned(['Mobile', 'All visitors', 'Anonymous'], (s) => s, (s) => s.startsWith('All'))
    expect(out).toEqual(['All visitors', 'Anonymous', 'Mobile'])
  })
})
