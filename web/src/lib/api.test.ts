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
