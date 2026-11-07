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

// The owner's case: registered by the app as a boolean with the variants on and
// off and the default off.
const storedOff: FeatureFlag = {
  ...docsPublic, variants: '{"on":true,"off":false}', default_variant: 'off', split: '{"off":100}', targeting_rules: '[]',
}

const textFlag: FeatureFlag = {
  ...docsPublic, flag_type: 'string', variants: '{"base":"a","fast":"b"}', default_variant: 'base', split: '{"base":100}', targeting_rules: '[]',
}

describe('FlagFormModal, config flags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows a boolean flag as one On/Off control with no value field and no rule-value rows', () => {
    render(<FlagFormModal projectId="p1" flag={docsPublic} onClose={() => {}} onSaved={() => {}} />)
    expect(screen.getByRole('button', { name: 'On' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Off' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.queryByLabelText('Value')).toBeNull()
    expect(screen.queryByLabelText('Variant 1 name')).toBeNull()
    expect(screen.queryByText('Add rule value')).toBeNull()
  })

  it('opens a flag stored with the default off, switches it to On and saves without an error', async () => {
    mockUpdate.mockResolvedValue(storedOff)
    render(<FlagFormModal projectId="p1" flag={storedOff} onClose={() => {}} onSaved={() => {}} />)

    fireEvent.click(screen.getByRole('button', { name: 'On' }))
    fireEvent.click(screen.getByText('Save changes'))

    await waitFor(() => expect(mockUpdate).toHaveBeenCalledTimes(1))
    const [, , body] = mockUpdate.mock.calls[0]
    expect(JSON.parse(body.variants)).toEqual({ on: true, off: false })
    expect(body.default_variant).toBe('on')
    expect(body.flag_type).toBe('boolean')
    expect(screen.queryByText(/must be unique|repeats the name/)).toBeNull()
  })

  it('keeps the rule, the fixed variants and a default stored under another name', async () => {
    mockUpdate.mockResolvedValue(docsPublic)
    render(<FlagFormModal projectId="p1" flag={docsPublic} onClose={() => {}} onSaved={() => {}} />)

    fireEvent.click(screen.getByRole('button', { name: 'On' }))
    fireEvent.click(screen.getByText('Save changes'))

    await waitFor(() => expect(mockUpdate).toHaveBeenCalledTimes(1))
    const [, , body] = mockUpdate.mock.calls[0]
    expect(JSON.parse(body.variants)).toEqual({ on: true, off: false, default: true })
    expect(body.default_variant).toBe('default')
    expect(JSON.parse(body.targeting_rules)).toEqual([testingRule])
    expect(body.flag_kind).toBe('config')
  })

  it('lets a targeting rule choose On or Off', () => {
    render(<FlagFormModal projectId="p1" flag={storedOff} onClose={() => {}} onSaved={() => {}} />)
    fireEvent.click(screen.getByText(/Targeting Rules/))
    fireEvent.click(screen.getByText('+ Add rule'))
    const options = Array.from(screen.getAllByRole('option')).map((o) => o.textContent)
    expect(options).toEqual(expect.arrayContaining(['on', 'off']))
  })

  it('asks for the type of a new config flag before it shows value fields', async () => {
    mockCreate.mockResolvedValue(docsPublic)
    render(<FlagFormModal projectId="p1" onClose={() => {}} onSaved={() => {}} />)

    fireEvent.change(screen.getByPlaceholderText('e.g. Checkout Redesign'), { target: { value: 'Docs public' } })
    fireEvent.click(screen.getByText('Config value'))
    expect(screen.getByRole('group', { name: 'Value type' })).toBeInTheDocument()
    expect(screen.queryByLabelText('Value')).toBeNull()
    expect(screen.queryByRole('button', { name: 'On' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Boolean' }))
    fireEvent.click(screen.getByRole('button', { name: 'On' }))
    fireEvent.click(screen.getByText(/Targeting Rules/))
    fireEvent.click(screen.getByText('+ Add rule'))
    fireEvent.click(screen.getByRole('button', { name: 'Create Flag' }))

    await waitFor(() => expect(mockCreate).toHaveBeenCalledTimes(1))
    const [, body] = mockCreate.mock.calls[0]
    expect(JSON.parse(body.variants)).toEqual({ on: true, off: false })
    expect(body.default_variant).toBe('on')
    expect(JSON.parse(body.targeting_rules)[0].variant).toBe('off')
  })

  it('refuses to create a config flag before its type is chosen', async () => {
    render(<FlagFormModal projectId="p1" onClose={() => {}} onSaved={() => {}} />)
    fireEvent.change(screen.getByPlaceholderText('e.g. Checkout Redesign'), { target: { value: 'Speed' } })
    fireEvent.click(screen.getByText('Config value'))
    fireEvent.click(screen.getByRole('button', { name: 'Create Flag' }))
    expect(await screen.findByText('Choose the value type first')).toBeInTheDocument()
    expect(mockCreate).not.toHaveBeenCalled()
  })

  it('lists the default of a text flag as the first row with a read-only name', () => {
    render(<FlagFormModal projectId="p1" flag={textFlag} onClose={() => {}} onSaved={() => {}} />)
    const name = screen.getByLabelText('Default name')
    expect(name).toHaveValue('base')
    expect(name).toHaveAttribute('readonly')
    expect(screen.getByLabelText('Value')).toHaveValue('a')
    expect(screen.getByLabelText('Variant 1 name')).toHaveValue('fast')
  })

  it('names the row a duplicate collides with', async () => {
    render(<FlagFormModal projectId="p1" flag={textFlag} onClose={() => {}} onSaved={() => {}} />)

    fireEvent.change(screen.getByLabelText('Variant 1 name'), { target: { value: 'base' } })
    fireEvent.click(screen.getByText('Save changes'))

    expect(await screen.findByText('Row 2 repeats the name "base" of row 1 (the default value)')).toBeInTheDocument()
    expect(mockUpdate).not.toHaveBeenCalled()
  })

  it('keeps a text value as typed and refuses a non-number in a number flag', async () => {
    mockUpdate.mockResolvedValue(textFlag)
    const { unmount } = render(<FlagFormModal projectId="p1" flag={textFlag} onClose={() => {}} onSaved={() => {}} />)
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'true' } })
    fireEvent.click(screen.getByText('Save changes'))
    await waitFor(() => expect(mockUpdate).toHaveBeenCalledTimes(1))
    expect(JSON.parse(mockUpdate.mock.calls[0][2].variants).base).toBe('true')
    unmount()

    const numberFlag = { ...textFlag, flag_type: 'number', variants: '{"base":5}' }
    render(<FlagFormModal projectId="p1" flag={numberFlag} onClose={() => {}} onSaved={() => {}} />)
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'abc' } })
    fireEvent.click(screen.getByText('Save changes'))
    expect(await screen.findByText('Value must be a number')).toBeInTheDocument()
    expect(mockUpdate).toHaveBeenCalledTimes(1)
  })
})
