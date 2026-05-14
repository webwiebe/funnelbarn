import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { DefaultToggle, alignBooleanSplit, parseFlagFormState } from './Flags'
import type { FeatureFlag } from '../lib/api'

function makeFlag(overrides: Partial<FeatureFlag> = {}): FeatureFlag {
  return {
    id: 'flag-1',
    project_id: 'p',
    flag_key: 'k',
    name: 'n',
    flag_type: 'boolean',
    variants: '{"on":true,"off":false}',
    default_variant: 'off',
    split: '{"on":0,"off":100}',
    conversion_event: '',
    targeting_rules: '[]',
    status: 'active',
    created_at: '',
    ...overrides,
  } as FeatureFlag
}

describe('alignBooleanSplit', () => {
  it('self-heals legacy state where split contradicted default', () => {
    // Legacy: stored default=off but split says 100% on (the bug we shipped
    // from the old modal). Toggling to default=on should produce a
    // consistent state, not flip the split to {on:0, off:100} via mirror.
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 100, off: 0 }), 'on')))
      .toEqual({ on: 100, off: 0 })
  })

  it('preserves rollout percentage when swapping default', () => {
    // 80% on default, 20% rolled out → toggling default flips which side is
    // the rollout but keeps the percentage.
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 80, off: 20 }), 'off')))
      .toEqual({ off: 80, on: 20 })
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 20, off: 80 }), 'on')))
      .toEqual({ on: 80, off: 20 })
  })

  it('handles the trivial 100/0 case', () => {
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 0, off: 100 }), 'on')))
      .toEqual({ on: 100, off: 0 })
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 100, off: 0 }), 'off')))
      .toEqual({ off: 100, on: 0 })
  })

  it('handles 50/50 splits (no clear majority)', () => {
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 50, off: 50 }), 'on')))
      .toEqual({ on: 50, off: 50 })
  })

  it('treats missing keys as 0', () => {
    expect(JSON.parse(alignBooleanSplit(JSON.stringify({ on: 30 }), 'on')))
      .toEqual({ on: 30, off: 0 })
  })

  it('falls back to all-zero on invalid JSON instead of throwing', () => {
    expect(JSON.parse(alignBooleanSplit('', 'on'))).toEqual({ on: 0, off: 0 })
    expect(JSON.parse(alignBooleanSplit('not json', 'off'))).toEqual({ off: 0, on: 0 })
  })
})

describe('DefaultToggle', () => {
  it('renders "on" label when value is true', () => {
    render(<DefaultToggle value={true} onChange={() => {}} />)
    expect(screen.getByText('on')).toBeInTheDocument()
  })

  it('renders "off" label when value is false', () => {
    render(<DefaultToggle value={false} onChange={() => {}} />)
    expect(screen.getByText('off')).toBeInTheDocument()
  })

  it('calls onChange when clicked', () => {
    const onChange = vi.fn()
    render(<DefaultToggle value={false} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button'))
    expect(onChange).toHaveBeenCalledTimes(1)
  })

  it('does not call onChange when disabled', () => {
    const onChange = vi.fn()
    render(<DefaultToggle value={false} onChange={onChange} disabled />)
    fireEvent.click(screen.getByRole('button'))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('hint title reflects current state and proposed flip direction', () => {
    const { rerender } = render(<DefaultToggle value={true} onChange={() => {}} />)
    expect(screen.getByRole('button')).toHaveAttribute('title', expect.stringContaining('flip to off'))
    rerender(<DefaultToggle value={false} onChange={() => {}} />)
    expect(screen.getByRole('button')).toHaveAttribute('title', expect.stringContaining('flip to on'))
  })
})

describe('parseFlagFormState', () => {
  it('parses a boolean flag with no rollout', () => {
    const state = parseFlagFormState(makeFlag({
      flag_type: 'boolean',
      default_variant: 'off',
      split: '{"on":0,"off":100}',
    }))
    expect(state.defaultBool).toBe('off')
    expect(state.rolloutEnabled).toBe(false)
    expect(state.rolloutPct).toBe(0)
  })

  it('parses a boolean flag with a partial rollout', () => {
    const state = parseFlagFormState(makeFlag({
      flag_type: 'boolean',
      default_variant: 'off',
      split: '{"on":20,"off":80}', // 20% rolled out to on
    }))
    expect(state.defaultBool).toBe('off')
    expect(state.rolloutEnabled).toBe(true)
    expect(state.rolloutPct).toBe(20)
  })

  it('treats fully-flipped boolean (default=off but 100% on) as no rollout', () => {
    // Legacy bad state — the editor shouldn't pre-fill rollout=100. The
    // toggle/align fix in the detail view is the right path for these.
    const state = parseFlagFormState(makeFlag({
      flag_type: 'boolean',
      default_variant: 'off',
      split: '{"on":100,"off":0}',
    }))
    expect(state.rolloutEnabled).toBe(false)
  })

  it('parses targeting rules from JSON', () => {
    const rules = [{ name: 'Beta', variant: 'on', match: 'all', conditions: [{ context_key: 'plan', operator: 'eq', value: 'pro' }] }]
    const state = parseFlagFormState(makeFlag({ targeting_rules: JSON.stringify(rules) }))
    expect(state.targetingRules).toHaveLength(1)
    expect(state.targetingRules[0].name).toBe('Beta')
  })

  it('rebuilds string variant rows from variants+split', () => {
    const state = parseFlagFormState(makeFlag({
      flag_type: 'string',
      variants: '{"a":"a","b":"b","c":"c"}',
      split: '{"a":33,"b":33,"c":34}',
      default_variant: 'b',
    }))
    expect(state.variants).toHaveLength(3)
    expect(state.variants.map((v) => v.name)).toEqual(['a', 'b', 'c'])
    expect(state.variants.map((v) => v.splitPct)).toEqual([33, 33, 34])
    expect(state.defaultVariant).toBe('b')
    expect(state.advancedReturnValues).toBe(false)
  })

  it('detects advanced return values when variant name !== value', () => {
    const state = parseFlagFormState(makeFlag({
      flag_type: 'string',
      variants: '{"a":"alpha","b":"beta"}',
      split: '{"a":50,"b":50}',
      default_variant: 'a',
    }))
    expect(state.advancedReturnValues).toBe(true)
    expect(state.variants[0].returnValue).toBe('alpha')
  })

  it('falls back to defaults on invalid JSON instead of throwing', () => {
    const state = parseFlagFormState(makeFlag({
      flag_type: 'string',
      variants: 'not json',
      split: 'also not json',
      targeting_rules: 'still not json',
    }))
    expect(state.variants.length).toBeGreaterThanOrEqual(2)
    expect(state.targetingRules).toEqual([])
  })
})
