import { describe, it, expect } from 'vitest'
import { recordedSize, replayScale, FALLBACK_RECORDED_SIZE } from './replayScale'

const meta = (data: Record<string, unknown>) => ({ type: 4, data, timestamp: 1 })

describe('recordedSize', () => {
  it('reads the width and height from the Meta event', () => {
    expect(recordedSize([meta({ width: 1000, height: 600 }), { type: 2 }])).toEqual({
      width: 1000, height: 600, fallback: false,
    })
  })

  it.each([
    ['a 0 width', { width: 0, height: 600 }],
    ['a 0 height', { width: 1000, height: 0 }],
    ['a negative width', { width: -5, height: 600 }],
    ['a width that is not a number', { width: '1000', height: 600 }],
    ['no size at all', {}],
  ])('falls back for %s', (_name, data) => {
    expect(recordedSize([meta(data)])).toEqual({ ...FALLBACK_RECORDED_SIZE, fallback: true })
  })

  it('falls back when there is no Meta event', () => {
    expect(recordedSize([{ type: 2 }])).toEqual({ ...FALLBACK_RECORDED_SIZE, fallback: true })
  })
})

describe('replayScale', () => {
  const size = { width: 1000, height: 500, fallback: false }

  it('fits the recorded viewport by its tighter side', () => {
    expect(replayScale(500, 500, size)).toBe(0.5)
    expect(replayScale(2000, 250, size)).toBe(0.5)
  })

  it('stays finite and positive for a container with no size yet', () => {
    expect(replayScale(0, 0, size)).toBe(1)
  })

  it('stays finite for a 0 recorded size, which recordedSize never returns', () => {
    expect(replayScale(800, 600, { width: 0, height: 0, fallback: false })).toBe(1)
  })
})
