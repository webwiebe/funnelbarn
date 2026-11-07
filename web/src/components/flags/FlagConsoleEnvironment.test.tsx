import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import type { FeatureFlag } from '../../lib/api'
import { FlagDetail } from '../../pages/Flags'
import { FlagFormModal } from './FlagFormModal'
import { ruleEnvironments, servedText } from './environmentScope'
import { flipConfigBoolean } from './shape'

const mockUpdate = vi.fn()
const mockEvaluate = vi.fn()

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      updateFlag: (...a: unknown[]) => mockUpdate(...a),
      evaluateFlagPlayground: (...a: unknown[]) => mockEvaluate(...a),
      getEnvironments: () => Promise.resolve({ environments: ['test', 'staging', 'production'] }),
      getFlagAnalysis: () => Promise.resolve({ flag: {}, results: [], unavailable: 'config' }),
      getEventNames: () => Promise.resolve({ event_names: [] }),
      getContextKeySuggestions: () => Promise.resolve({ suggestions: [] }),
    },
  }
})

const rule = {
  name: 'docs', variant: 'on', match: 'all',
  conditions: [{ context_key: 'environment', operator: 'in', value: 'test, staging' }],
}

function configFlag(over: Partial<FeatureFlag> = {}): FeatureFlag {
  return {
    id: 'f1', project_id: 'p1', flag_key: 'docs_public', name: 'Docs public', flag_type: 'boolean',
    variants: '{"default":false,"on":true}', default_variant: 'default', split: '{"default":100}',
    targeting_rules: JSON.stringify([rule]), status: 'active', created_at: '2026-11-04 10:00:00',
    origin: 'manual', flag_kind: 'config', ...over,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mockEvaluate.mockImplementation((_p: string, body: { context: { environment: string } }) =>
    Promise.resolve({ flag_key: 'docs_public', variant: 'x', reason: 'STATIC', value: body.context.environment !== 'production' }))
})

describe('flag console: environments (#491)', () => {
  it('says saves apply to every environment and shows what each environment serves', async () => {
    render(<FlagDetail flag={configFlag()} projectId="p1" onUpdated={() => {}} onDeleted={() => {}} />)
    expect(screen.getByTestId('flag-environments')).toHaveTextContent('apply to every environment')
    await waitFor(() => expect(screen.getByTestId('flag-environments')).toHaveTextContent('test: true'))
    expect(screen.getByTestId('flag-environments')).toHaveTextContent('staging: true')
    expect(screen.getByTestId('flag-environments')).toHaveTextContent('production: false')
    expect(mockEvaluate.mock.calls.map((c) => c[1].context)).toContainEqual({ environment: 'staging' })
  })

  it('names the scope next to the Save button of the Edit Flag dialog', () => {
    render(<FlagFormModal projectId="p1" flag={configFlag()} onClose={() => {}} onSaved={() => {}} />)
    expect(screen.getByTestId('flag-scope-note')).toHaveTextContent('every environment of this project')
  })

  it('reads environment names out of eq and in rules only', () => {
    expect(ruleEnvironments(JSON.stringify([rule]))).toEqual(['test', 'staging'])
    expect(ruleEnvironments('not json')).toEqual([])
    expect(servedText(undefined)).toBe('…')
    expect(servedText({ flag_key: 'k', variant: '', reason: '', value: '' })).toBe('nothing')
  })
})

describe('flag console: boolean config value (#491)', () => {
  it('shows true or false as the current value', () => {
    const { rerender } = render(<FlagDetail flag={configFlag()} projectId="p1" onUpdated={() => {}} onDeleted={() => {}} />)
    expect(screen.getByText('false', { selector: 'code' })).toBeInTheDocument()
    rerender(<FlagDetail flag={configFlag({ variants: '{"default":true,"on":true}' })} projectId="p1" onUpdated={() => {}} onDeleted={() => {}} />)
    expect(screen.getByText('true', { selector: 'code' })).toBeInTheDocument()
  })

  it('says so when the default variant has no value, never a blank', () => {
    render(<FlagDetail flag={configFlag({ default_variant: 'off' })} projectId="p1" onUpdated={() => {}} onDeleted={() => {}} />)
    expect(screen.getByTestId('config-value-missing')).toHaveTextContent('"off" does not exist')
  })

  it('flips the value and keeps the default variant when the header toggle is used', async () => {
    mockUpdate.mockResolvedValue(configFlag())
    render(<FlagDetail flag={configFlag({ default_variant: 'off' })} projectId="p1" onUpdated={() => {}} onDeleted={() => {}} />)
    fireEvent.click(screen.getByText('off', { selector: 'span' }).closest('button')!)
    await waitFor(() => expect(mockUpdate).toHaveBeenCalledTimes(1))
    const body = mockUpdate.mock.calls[0][2]
    expect(body.default_variant).toBe('off')
    expect(JSON.parse(body.variants)).toEqual({ default: false, on: true, off: true })
  })

  it('flipConfigBoolean inverts, creates a missing value as true and survives bad JSON', () => {
    expect(JSON.parse(flipConfigBoolean('{"default":true}', 'default'))).toEqual({ default: false })
    expect(JSON.parse(flipConfigBoolean('{"default":false}', 'default'))).toEqual({ default: true })
    expect(JSON.parse(flipConfigBoolean('{}', 'default'))).toEqual({ default: true })
    expect(JSON.parse(flipConfigBoolean('nope', 'default'))).toEqual({ default: true })
  })

  it.each(['""', 'yes', '1', '{"a":1}'])('refuses %s as the value of a boolean config flag', async (value) => {
    render(<FlagFormModal projectId="p1" flag={configFlag()} onClose={() => {}} onSaved={() => {}} />)
    fireEvent.change(screen.getByLabelText('Value'), { target: { value } })
    fireEvent.click(screen.getByText('Save changes'))
    expect(await screen.findByText(/must be true or false|required/)).toBeInTheDocument()
    expect(mockUpdate).not.toHaveBeenCalled()
  })

  it('accepts false for a boolean config flag', async () => {
    mockUpdate.mockResolvedValue(configFlag())
    render(<FlagFormModal projectId="p1" flag={configFlag()} onClose={() => {}} onSaved={() => {}} />)
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'false' } })
    fireEvent.click(screen.getByText('Save changes'))
    await waitFor(() => expect(mockUpdate).toHaveBeenCalledTimes(1))
  })
})
