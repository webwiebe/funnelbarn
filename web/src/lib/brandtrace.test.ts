import { afterEach, describe, expect, it } from 'vitest'
import { loadBrandTrace } from './brandtrace'

const SELECTOR = 'script[src="https://brandtrace.net/sdk.js"]'

describe('loadBrandTrace', () => {
  afterEach(() => {
    document.head.innerHTML = ''
  })

  it('adds the SDK script with the site key', () => {
    loadBrandTrace('abc123')
    const script = document.querySelector<HTMLScriptElement>(SELECTOR)
    expect(script?.dataset.siteKey).toBe('abc123')
    expect(script?.defer).toBe(true)
  })

  it('stays off for an empty or missing key', () => {
    loadBrandTrace('')
    loadBrandTrace(undefined)
    expect(document.querySelector(SELECTOR)).toBeNull()
  })

  it('adds the script once', () => {
    loadBrandTrace('abc123')
    loadBrandTrace('abc123')
    expect(document.querySelectorAll(SELECTOR)).toHaveLength(1)
  })
})
