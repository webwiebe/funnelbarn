import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import type { FeatureFlag } from '../../lib/api'
import { FlagFormModal } from './FlagFormModal'

const mockUpdate = vi.fn()
const mockCreate = vi.fn()

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      updateFlag: (...args: unknown[]) => mockUpdate(...args),
      createFlag: (...args: unknown[]) => mockCreate(...args),
      getEventNames: () => Promise.resolve({ event_names: [] }),
      getContextKeySuggestions: () => Promise.resolve({ suggestions: [] }),
    },
  }
})

const testingRule = {
  name: 'testing', variant: 'on', match: 'all',
  conditions: [{ context_key: 'environment', operator: 'eq', value: 'testing' }],
}

// The case from #459: professionals docs_public, a config flag whose rule serves
// "on" in testing while everyone else gets the default.
const docsPublic: FeatureFlag = {
  id: 'f1', project_id: 'p1', flag_key: 'docs_public', name: 'Docs public', flag_type: 'boolean',
  variants: '{"default":false,"on":true}', default_variant: 'default', split: '{"default":100}',
  targeting_rules: JSON.stringify([testingRule]),
  status: 'active', created_at: '2026-11-04 10:00:00', origin: 'manual', flag_kind: 'config',
}

describe('FlagFormModal, config flags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps the rule and the variant it serves when the default value changes', async () => {
    mockUpdate.mockResolvedValue(docsPublic)
    render(<FlagFormModal projectId="p1" flag={docsPublic} onClose={() => {}} onSaved={() => {}} />)

    expect(screen.getByLabelText('Variant 1 name')).toHaveValue('on')
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'true' } })
    fireEvent.click(screen.getByText('Save changes'))

    await waitFor(() => expect(mockUpdate).toHaveBeenCalledTimes(1))
    const [, , body] = mockUpdate.mock.calls[0]
    expect(JSON.parse(body.variants)).toEqual({ default: true, on: true })
    expect(body.default_variant).toBe('default')
    expect(JSON.parse(body.split)).toEqual({ default: 100 })
    expect(JSON.parse(body.targeting_rules)).toEqual([testingRule])
    expect(body.flag_kind).toBe('config')
  })

  it('builds a new config flag with a rule that serves a named value', async () => {
    mockCreate.mockResolvedValue(docsPublic)
    render(<FlagFormModal projectId="p1" onClose={() => {}} onSaved={() => {}} />)

    fireEvent.change(screen.getByPlaceholderText('e.g. Checkout Redesign'), { target: { value: 'Docs public' } })
    fireEvent.click(screen.getByText('Config value'))
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'false' } })
    fireEvent.click(screen.getByText('Add rule value'))
    fireEvent.change(screen.getByLabelText('Variant 1 name'), { target: { value: 'on' } })
    fireEvent.change(screen.getByLabelText('Variant 1 value'), { target: { value: 'true' } })
    fireEvent.click(screen.getByText(/Targeting Rules/))
    fireEvent.click(screen.getByText('+ Add rule'))
    fireEvent.click(screen.getByRole('button', { name: 'Create Flag' }))

    await waitFor(() => expect(mockCreate).toHaveBeenCalledTimes(1))
    const [, body] = mockCreate.mock.calls[0]
    expect(JSON.parse(body.variants)).toEqual({ default: false, on: true })
    const rules = JSON.parse(body.targeting_rules)
    expect(rules).toHaveLength(1)
    expect(rules[0].variant).toBe('on')
  })

  it('refuses a named value that repeats the default name', async () => {
    render(<FlagFormModal projectId="p1" flag={docsPublic} onClose={() => {}} onSaved={() => {}} />)

    fireEvent.change(screen.getByLabelText('Variant 1 name'), { target: { value: 'default' } })
    fireEvent.click(screen.getByText('Save changes'))

    expect(await screen.findByText('Variant names must be unique ("default")')).toBeInTheDocument()
    expect(mockUpdate).not.toHaveBeenCalled()
  })
})
