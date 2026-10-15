import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { EnvironmentSwitch } from './EnvironmentSwitch'

const mockApi = vi.hoisted(() => ({ getEnvironments: vi.fn() }))
vi.mock('../../lib/api', () => ({ api: mockApi }))

describe('EnvironmentSwitch', () => {
  beforeEach(() => {
    mockApi.getEnvironments.mockReset()
    mockApi.getEnvironments.mockResolvedValue({ environments: ['staging', 'production', 'test'] })
  })

  it('offers All, Production and the other recorded environments once each', async () => {
    render(<EnvironmentSwitch projectId="p1" value="" onChange={vi.fn()} />)
    await screen.findByRole('button', { name: 'Staging' })
    const names = screen.getAllByRole('button').map((b) => b.textContent)
    expect(names).toEqual(['All', 'Production', 'Staging', 'Test'])
  })

  it('offers Production even when nothing is recorded yet', async () => {
    mockApi.getEnvironments.mockResolvedValue({ environments: [] })
    render(<EnvironmentSwitch projectId="p1" value="" onChange={vi.fn()} />)
    await waitFor(() => expect(mockApi.getEnvironments).toHaveBeenCalledWith('p1'))
    expect(screen.getByRole('button', { name: 'Production' })).toBeInTheDocument()
  })

  it('selects an environment per click and All with an empty value', async () => {
    const onChange = vi.fn()
    render(<EnvironmentSwitch projectId="p1" value="production" onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'All' }))
    expect(onChange).toHaveBeenLastCalledWith('')
    fireEvent.click(await screen.findByRole('button', { name: 'Staging' }))
    expect(onChange).toHaveBeenLastCalledWith('staging')
    expect(screen.getByRole('button', { name: 'Production' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('explains where events without an environment show up', () => {
    const { rerender } = render(<EnvironmentSwitch projectId="p1" value="" onChange={vi.fn()} />)
    expect(screen.queryByText(/appear only under All/)).toBeNull()
    rerender(<EnvironmentSwitch projectId="p1" value="production" onChange={vi.fn()} />)
    expect(screen.getByText(/appear only under All/)).toBeInTheDocument()
  })
})
