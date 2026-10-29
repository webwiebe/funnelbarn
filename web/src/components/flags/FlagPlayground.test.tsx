import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import type { FeatureFlag } from '../../lib/api'
import { FlagPlayground, TargetingRulesDisplay } from './FlagPlayground'

const mockEvaluate = vi.fn()

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return {
    ...actual,
    api: { ...actual.api, evaluateFlagPlayground: (...args: unknown[]) => mockEvaluate(...args) },
  }
})

const flag: FeatureFlag = {
  id: 'f1', project_id: 'p1', flag_key: 'checkout_redesign', name: 'Checkout redesign', flag_type: 'boolean',
  variants: '{"on":true,"off":false}', default_variant: 'off', split: '{"on":50,"off":50}', targeting_rules: '[]',
  status: 'active', created_at: '2026-10-01 10:00:00', origin: 'manual', flag_kind: 'experiment',
}

describe('FlagPlayground', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('evaluates the flag with a typed context and shows the result', async () => {
    mockEvaluate.mockResolvedValue({ flag_key: 'checkout_redesign', variant: 'on', value: true, reason: 'SPLIT' })
    render(<FlagPlayground projectId="p1" flag={flag} />)

    fireEvent.change(screen.getByPlaceholderText('value'), { target: { value: '42' } })
    fireEvent.click(screen.getByRole('button', { name: /Add field/ }))
    const keys = screen.getAllByPlaceholderText('key')
    const values = screen.getAllByPlaceholderText('value')
    fireEvent.change(keys[1], { target: { value: 'beta' } })
    fireEvent.change(values[1], { target: { value: 'true' } })

    await waitFor(() => expect(screen.getByText('SPLIT')).toBeInTheDocument())
    expect(mockEvaluate).toHaveBeenLastCalledWith('p1', {
      flag_key: 'checkout_redesign',
      default_value: undefined,
      context: { targeting_key: 42, beta: true },
    })
    expect(screen.getByText('on')).toBeInTheDocument()
    expect(screen.getByText('true')).toBeInTheDocument()
  })

  it('shows the request error when the evaluation fails', async () => {
    mockEvaluate.mockRejectedValue(new Error('HTTP 500'))
    render(<FlagPlayground projectId="p1" flag={flag} />)
    await waitFor(() => expect(screen.getByText('HTTP 500')).toBeInTheDocument())
  })

  it('asks for a key instead of evaluating an empty one', () => {
    render(<FlagPlayground projectId="p1" flag={{ ...flag, flag_key: '' }} />)
    expect(screen.getByText('Enter a flag key to evaluate.')).toBeInTheDocument()
    expect(mockEvaluate).not.toHaveBeenCalled()
  })
})

describe('TargetingRulesDisplay', () => {
  it('lists each rule with its conditions', () => {
    const rules = [{
      name: 'Beta users', variant: 'on', match: 'all',
      conditions: [
        { context_key: 'plan', operator: 'eq', value: 'pro' },
        { context_key: 'email', operator: 'present', value: '' },
      ],
    }]
    render(<TargetingRulesDisplay rulesJSON={JSON.stringify(rules)} />)
    expect(screen.getByText('Targeting Rules (1)')).toBeInTheDocument()
    expect(screen.getByText('Beta users')).toBeInTheDocument()
    expect(screen.getByText('"pro"')).toBeInTheDocument()
  })

  it('renders nothing for no rules or unreadable JSON', () => {
    const { container } = render(<TargetingRulesDisplay rulesJSON="not json" />)
    expect(container).toBeEmptyDOMElement()
  })
})
