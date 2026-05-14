import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { api, ApiError } from './api'

function mockFetch(status: number, body: unknown, headers: Record<string, string> = {}) {
  const responseBody = typeof body === 'string' ? body : JSON.stringify(body)
  globalThis.fetch = vi.fn().mockResolvedValue({
    status,
    ok: status >= 200 && status < 300,
    text: () => Promise.resolve(responseBody),
    json: () => Promise.resolve(body),
    headers: new Headers(headers),
  })
}

beforeEach(() => {
  Object.defineProperty(window, 'location', {
    writable: true,
    value: { pathname: '/dashboard', href: '/dashboard' },
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ApiError', () => {
  it('has the correct message', () => {
    const err = new ApiError(404, 'Not found')
    expect(err.message).toBe('Not found')
  })

  it('has the correct status code', () => {
    const err = new ApiError(403, 'Forbidden')
    expect(err.status).toBe(403)
  })

  it('is an instance of Error', () => {
    const err = new ApiError(500, 'Server error')
    expect(err).toBeInstanceOf(Error)
  })

  it('has name set to ApiError', () => {
    const err = new ApiError(401, 'Unauthorized')
    expect(err.name).toBe('ApiError')
  })
})

describe('api — request headers', () => {
  it('sends Content-Type: application/json by default', async () => {
    mockFetch(200, { id: '1', username: 'alice' })
    await api.me()
    const [, options] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit]
    expect((options.headers as Record<string, string>)['Content-Type']).toBe('application/json')
  })

  it('sends credentials: include', async () => {
    mockFetch(200, { id: '1', username: 'alice' })
    await api.me()
    const [, options] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit]
    expect(options.credentials).toBe('include')
  })

  it('sends POST body as JSON string', async () => {
    mockFetch(200, { id: '1', username: 'alice' })
    await api.login({ username: 'alice', password: 'secret' })
    const [, options] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit]
    expect(options.method).toBe('POST')
    expect(options.body).toBe(JSON.stringify({ username: 'alice', password: 'secret' }))
  })
})

describe('api — error handling', () => {
  it('throws ApiError with status 404 on not-found response', async () => {
    mockFetch(404, { error: 'not found' })
    await expect(api.me()).rejects.toMatchObject({ status: 404 })
  })

  it('throws ApiError with status 500 on server error', async () => {
    mockFetch(500, { error: 'internal server error' })
    await expect(api.me()).rejects.toBeInstanceOf(ApiError)
  })

  it('uses error field from JSON body as message', async () => {
    mockFetch(400, { error: 'bad credentials' })
    let thrown: ApiError | null = null
    try {
      await api.login({ username: 'x', password: 'y' })
    } catch (e) {
      thrown = e as ApiError
    }
    expect(thrown).not.toBeNull()
    expect(thrown!.message).toBe('bad credentials')
    expect(thrown!.status).toBe(400)
  })

  it('throws ApiError(401) on unauthorized response', async () => {
    mockFetch(401, { error: 'Unauthorized' })
    await expect(api.me()).rejects.toMatchObject({ status: 401, name: 'ApiError' })
  })
})

describe('api — JSON parsing', () => {
  it('returns parsed JSON on success', async () => {
    const user = { id: 'u1', username: 'alice' }
    mockFetch(200, user)
    const result = await api.me()
    expect(result).toEqual(user)
  })

  it('returns undefined when response body is empty (e.g. DELETE)', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      text: () => Promise.resolve(''),
    })
    const result = await api.deleteFunnel('proj1', 'funnel1')
    expect(result).toBeUndefined()
  })
})

// ---------------------------------------------------------------------------
// Response shape contract tests
// ---------------------------------------------------------------------------

describe('api.listProjects', () => {
  it('returns the projects array from { projects: [...] }', async () => {
    mockFetch(200, { projects: [{ id: '1', name: 'Test', slug: 'test', status: 'active' }] })
    const result = await api.listProjects()
    expect(result.projects).toHaveLength(1)
    expect(result.projects[0].name).toBe('Test')
  })

  it('throws ApiError when response is not ok', async () => {
    mockFetch(401, { error: 'unauthorized' })
    await expect(api.listProjects()).rejects.toBeInstanceOf(ApiError)
  })

  it('throws ApiError with status 403 on forbidden', async () => {
    mockFetch(403, { error: 'forbidden' })
    await expect(api.listProjects()).rejects.toMatchObject({ status: 403 })
  })
})

describe('api.createProject', () => {
  it('sends name and domain in POST body', async () => {
    mockFetch(200, { id: '1', name: 'New', slug: 'new', status: 'active' })
    await api.createProject('New', 'new.example.com')
    const [, options] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit]
    const body = JSON.parse(options.body as string)
    expect(body.name).toBe('New')
    expect(body.domain).toBe('new.example.com')
  })

  it('returns created project on success', async () => {
    const project = { id: '1', name: 'New', slug: 'new', status: 'active' }
    mockFetch(200, project)
    const result = await api.createProject('New', 'new.example.com')
    expect(result).toEqual(project)
  })

  it('throws ApiError with status 409 on conflict', async () => {
    mockFetch(409, { error: 'already exists' })
    await expect(api.createProject('X', 'x.com')).rejects.toMatchObject({ status: 409 })
  })

  it('throws ApiError with status 422 on validation error', async () => {
    mockFetch(422, { error: 'name: required' })
    await expect(api.createProject('', '')).rejects.toMatchObject({ status: 422 })
  })
})

describe('api.login', () => {
  it('returns user object on success', async () => {
    mockFetch(200, { id: 'u1', username: 'alice' })
    const user = await api.login({ username: 'alice', password: 'secret' })
    expect(user.username).toBe('alice')
  })

  it('throws ApiError with status 401 on bad credentials', async () => {
    mockFetch(401, { error: 'invalid credentials' })
    await expect(api.login({ username: 'x', password: 'y' })).rejects.toMatchObject({ status: 401 })
  })
})

describe('api.me', () => {
  it('returns current user on success', async () => {
    mockFetch(200, { id: 'u1', username: 'bob' })
    const result = await api.me()
    expect(result).toMatchObject({ id: 'u1', username: 'bob' })
  })
})

describe('api.listFunnels', () => {
  it('returns funnels array from { funnels: [...] }', async () => {
    const funnel = { id: 'f1', name: 'Signup', steps: [] }
    mockFetch(200, { funnels: [funnel] })
    const result = await api.listFunnels('proj1')
    expect(result.funnels).toHaveLength(1)
    expect(result.funnels[0].name).toBe('Signup')
  })

  it('throws ApiError on non-ok response', async () => {
    mockFetch(404, { error: 'project not found' })
    await expect(api.listFunnels('missing')).rejects.toBeInstanceOf(ApiError)
  })
})

describe('api.createFunnel', () => {
  it('sends name and steps in POST body', async () => {
    const steps = [{ event_name: 'page_view' }]
    mockFetch(200, { id: 'f1', name: 'Checkout', steps: [] })
    await api.createFunnel('proj1', 'Checkout', steps)
    const [, options] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit]
    const body = JSON.parse(options.body as string)
    expect(body.name).toBe('Checkout')
    expect(body.steps).toEqual(steps)
  })
})

describe('api.getEventProperties', () => {
  it('calls the correct URL with encoded event_name', async () => {
    mockFetch(200, { properties: ['plan', 'source'] })
    const result = await api.getEventProperties('proj1', 'page view')
    expect(result.properties).toEqual(['plan', 'source'])
    const [url] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string]
    expect(url).toContain('/event-properties?event_name=page%20view')
  })

  it('returns empty array from server', async () => {
    mockFetch(200, { properties: [] })
    const result = await api.getEventProperties('proj1', 'signup')
    expect(result.properties).toEqual([])
  })
})

describe('api.getEventPropertyValues', () => {
  it('calls the correct URL with encoded params', async () => {
    mockFetch(200, { values: ['pro', 'free'] })
    const result = await api.getEventPropertyValues('proj1', 'signup', 'plan')
    expect(result.values).toEqual(['pro', 'free'])
    const [url] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string]
    expect(url).toContain('/event-property-values?event_name=signup&property=plan')
  })

  it('encodes special characters in property name', async () => {
    mockFetch(200, { values: [] })
    await api.getEventPropertyValues('proj1', 'click', 'button name')
    const [url] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string]
    expect(url).toContain('property=button%20name')
  })
})

describe('api — transient 5xx retry', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  function mockFetchSequence(...responses: Array<{ status: number; body?: unknown }>) {
    const fn = vi.fn()
    for (const r of responses) {
      const body = r.body ?? {}
      fn.mockResolvedValueOnce({
        status: r.status,
        ok: r.status >= 200 && r.status < 300,
        text: () => Promise.resolve(JSON.stringify(body)),
        json: () => Promise.resolve(body),
        headers: new Headers(),
      })
    }
    globalThis.fetch = fn
    return fn
  }

  it('retries a GET once on 503 and succeeds on second attempt', async () => {
    const fetchFn = mockFetchSequence({ status: 503 }, { status: 200, body: { ok: true } })
    const promise = api.me()
    await vi.advanceTimersByTimeAsync(800)
    const result = await promise
    expect(result).toEqual({ ok: true })
    expect(fetchFn).toHaveBeenCalledTimes(2)
  })

  it('does not retry POST on 503 (mutation might have partially applied)', async () => {
    const fetchFn = mockFetchSequence({ status: 503, body: { error: 'unavailable' } })
    await expect(api.login({ username: 'a', password: 'b' })).rejects.toMatchObject({
      status: 503,
    })
    expect(fetchFn).toHaveBeenCalledTimes(1)
  })

  it('throws ApiError(0) when fetch itself rejects (network failure)', async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    await expect(api.me()).rejects.toMatchObject({ status: 0 })
  })
})
