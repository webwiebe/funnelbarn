import { describe, it, expect, vi, beforeAll } from 'vitest'

// ---------------------------------------------------------------------------
// main.tsx is an entry module with top-level side effects: it installs global
// error handlers and mounts the React app. We mock everything it touches so
// importing it neither renders the real app nor performs real error reporting,
// then exercise the handlers it registered on `window`.
// ---------------------------------------------------------------------------

const mockReportError = vi.hoisted(() => vi.fn())
const mockRender = vi.hoisted(() => vi.fn())
const mockCreateRoot = vi.hoisted(() => vi.fn((_el: HTMLElement) => ({ render: mockRender })))

vi.mock('./lib/bugbarn', () => ({ reportError: mockReportError }))

vi.mock('./App', () => ({ default: () => null }))

vi.mock('./index.css', () => ({}))

vi.mock('react-dom/client', () => ({
  default: { createRoot: mockCreateRoot },
  createRoot: mockCreateRoot,
}))

beforeAll(async () => {
  document.body.innerHTML = '<div id="root"></div>'
  await import('./main')
})

describe('main entry module', () => {
  it('mounts the app by creating a root on the #root element', () => {
    expect(mockCreateRoot).toHaveBeenCalledTimes(1)
    const rootArg = mockCreateRoot.mock.calls[0][0]
    expect(rootArg).toBe(document.getElementById('root'))
    expect(mockRender).toHaveBeenCalledTimes(1)
  })

  it('registers a global window.onerror handler', () => {
    expect(typeof window.onerror).toBe('function')
  })

  it('registers a global window.onunhandledrejection handler', () => {
    expect(typeof window.onunhandledrejection).toBe('function')
  })

  describe('window.onerror', () => {
    it('reports the provided error object', () => {
      mockReportError.mockClear()
      const err = new Error('kaboom')
      const result = (window.onerror as OnErrorEventHandler)!(
        'kaboom',
        'app.js',
        12,
        34,
        err,
      )
      expect(mockReportError).toHaveBeenCalledTimes(1)
      expect(mockReportError.mock.calls[0][0]).toBe(err)
      expect(mockReportError.mock.calls[0][1]).toMatchObject({
        source: 'window.onerror',
        file: 'app.js',
        line: 12,
        col: 34,
      })
      // Returns false so the browser still logs to the console.
      expect(result).toBe(false)
    })

    it('synthesizes an Error when none is provided', () => {
      mockReportError.mockClear()
      ;(window.onerror as OnErrorEventHandler)!('just a message')
      expect(mockReportError).toHaveBeenCalledTimes(1)
      const reported = mockReportError.mock.calls[0][0]
      expect(reported).toBeInstanceOf(Error)
      expect((reported as Error).message).toBe('just a message')
    })
  })

  describe('window.onunhandledrejection', () => {
    const fire = (reason: unknown) =>
      (window.onunhandledrejection as (e: PromiseRejectionEvent) => void)!(
        { reason } as PromiseRejectionEvent,
      )

    it('reports a genuine unhandled rejection', () => {
      mockReportError.mockClear()
      const err = new Error('real bug')
      fire(err)
      expect(mockReportError).toHaveBeenCalledTimes(1)
      expect(mockReportError.mock.calls[0][0]).toBe(err)
      expect(mockReportError.mock.calls[0][1]).toMatchObject({
        source: 'window.onunhandledrejection',
      })
    })

    it('filters out ServiceWorker registration failures (by message)', () => {
      mockReportError.mockClear()
      fire(new Error('Failed to register a ServiceWorker for scope ...'))
      expect(mockReportError).not.toHaveBeenCalled()
    })

    it('filters out rejections whose stack points at registerSW.js', () => {
      mockReportError.mockClear()
      const err = new Error('Rejected')
      err.stack = 'Error: Rejected\n    at registerSW.js:1:1'
      fire(err)
      expect(mockReportError).not.toHaveBeenCalled()
    })

    it('filters out TypeError: Failed to fetch network blips', () => {
      mockReportError.mockClear()
      fire(new TypeError('Failed to fetch'))
      expect(mockReportError).not.toHaveBeenCalled()
    })

    it('still reports a non-TypeError "Failed to fetch" reason', () => {
      mockReportError.mockClear()
      fire(new Error('Failed to fetch'))
      expect(mockReportError).toHaveBeenCalledTimes(1)
    })
  })
})
