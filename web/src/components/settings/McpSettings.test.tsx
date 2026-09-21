import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { McpSettings } from './McpSettings'
import type { Project } from '../../lib/api'

const mockGetClientConfig = vi.fn()

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return {
    ...actual,
    api: { ...actual.api, getClientConfig: (...args: unknown[]) => mockGetClientConfig(...args) },
  }
})

const projects: Project[] = [
  { id: 'p1', name: 'My Site', slug: 'my-site', status: 'active' },
  { id: 'p2', name: 'Other Site', slug: 'other-site', status: 'active' },
]

describe('McpSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing while OIDC status is unknown or config fails to load', async () => {
    mockGetClientConfig.mockRejectedValue(new Error('network error'))
    const { container } = render(<McpSettings projects={projects} projectId="p1" />)
    await waitFor(() => expect(mockGetClientConfig).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })

  it('is hidden when OIDC is not enabled', async () => {
    mockGetClientConfig.mockResolvedValue({ oidc: { enabled: false } })
    const { container } = render(<McpSettings projects={projects} projectId="p1" />)
    await waitFor(() => expect(mockGetClientConfig).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })

  it('is hidden when the client config has no oidc field at all', async () => {
    mockGetClientConfig.mockResolvedValue({ bugbarn_endpoint: '', bugbarn_ingest_key: '' })
    const { container } = render(<McpSettings projects={projects} projectId="p1" />)
    await waitFor(() => expect(mockGetClientConfig).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the claude mcp add command with the origin and the selected project slug', async () => {
    mockGetClientConfig.mockResolvedValue({ oidc: { enabled: true } })
    render(<McpSettings projects={projects} projectId="p2" />)

    await waitFor(() => screen.getByText('Connect an assistant'))

    const command = `claude mcp add --transport http funnelbarn ${window.location.origin}/api/v1/mcp --header "x-funnelbarn-project: other-site"`
    expect(screen.getByText(command)).toBeInTheDocument()
  })

  it('shows a pretty-printed .mcp.json snippet with the origin and slug', async () => {
    mockGetClientConfig.mockResolvedValue({ oidc: { enabled: true } })
    render(<McpSettings projects={projects} projectId="p1" />)

    await waitFor(() => screen.getByText('Connect an assistant'))

    const expected = JSON.stringify(
      {
        mcpServers: {
          funnelbarn: {
            type: 'http',
            url: `${window.location.origin}/api/v1/mcp`,
            headers: { 'x-funnelbarn-project': 'my-site' },
          },
        },
      },
      null,
      2,
    )
    expect(screen.getByTestId('mcp-json-snippet').textContent).toBe(expected)
  })

  it('names the selected project and mentions scopes and the no-secret sign-in', async () => {
    mockGetClientConfig.mockResolvedValue({ oidc: { enabled: true } })
    render(<McpSettings projects={projects} projectId="p1" />)

    await waitFor(() => screen.getByText('Connect an assistant'))

    expect(screen.getByText('My Site')).toBeInTheDocument()
    expect(screen.getByText(/holds no secret/i)).toBeInTheDocument()
    expect(screen.getByText(/IAMBarn/)).toBeInTheDocument()
    expect(screen.getByText(/reads stats, events, funnels, segments and flags/i)).toBeInTheDocument()
    expect(screen.getByText(/creates, updates and deletes/i)).toBeInTheDocument()
    expect(screen.getByText(/overrides the default project/i)).toBeInTheDocument()
  })
})
