import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useIambarnWidget, __resetIambarnWidgetLoaders } from './iambarn-widget'

// Ensure the custom element is defined so customElements.whenDefined resolves.
class FakeMenu extends HTMLElement {}
if (!customElements.get('iambarn-user-menu')) {
  customElements.define('iambarn-user-menu', FakeMenu)
}

function widgetScripts(): HTMLScriptElement[] {
  return Array.from(document.querySelectorAll('script[data-iambarn-widget]'))
}

describe('useIambarnWidget', () => {
  beforeEach(() => {
    __resetIambarnWidgetLoaders()
  })
  afterEach(() => {
    widgetScripts().forEach((s) => s.remove())
  })

  it('is a no-op and stays not-ready when url is falsy', () => {
    const { result } = renderHook(() => useIambarnWidget(undefined))
    expect(result.current).toBe(false)
    expect(widgetScripts()).toHaveLength(0)
  })

  it('injects the bundle script exactly once for a url', () => {
    const url = 'https://iam.test/widget/a.js'
    renderHook(() => useIambarnWidget(url))
    renderHook(() => useIambarnWidget(url))
    const scripts = widgetScripts().filter((s) => s.dataset.iambarnWidget === url)
    expect(scripts).toHaveLength(1)
    expect(scripts[0].src).toBe(url)
  })

  it('becomes ready after the script loads and the element upgrades', async () => {
    const url = 'https://iam.test/widget/b.js'
    const { result } = renderHook(() => useIambarnWidget(url))
    expect(result.current).toBe(false)

    const script = widgetScripts().find((s) => s.dataset.iambarnWidget === url)!
    act(() => {
      script.dispatchEvent(new Event('load'))
    })
    await waitFor(() => expect(result.current).toBe(true))
  })

  it('stays not-ready when the script fails to load', async () => {
    const url = 'https://iam.test/widget/c.js'
    const { result } = renderHook(() => useIambarnWidget(url))
    const script = widgetScripts().find((s) => s.dataset.iambarnWidget === url)!
    act(() => {
      script.dispatchEvent(new Event('error'))
    })
    // Give the resolved(false) promise a tick; readiness must remain false.
    await new Promise((r) => setTimeout(r, 0))
    expect(result.current).toBe(false)
  })
})
