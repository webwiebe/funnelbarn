import { beforeEach, describe, expect, it } from 'vitest'
import { FAVOURITE_PROJECTS_KEY, readFavouriteProjectIds, toggleFavouriteId, writeFavouriteProjectIds } from './favouriteProjects'

describe('favouriteProjects', () => {
  beforeEach(() => window.localStorage.clear())

  it('reads an empty list when nothing is stored', () => {
    expect(readFavouriteProjectIds()).toEqual([])
  })

  it('round-trips ids', () => {
    writeFavouriteProjectIds(['a', 'b'])
    expect(readFavouriteProjectIds()).toEqual(['a', 'b'])
  })

  it('ignores malformed storage', () => {
    window.localStorage.setItem(FAVOURITE_PROJECTS_KEY, '{oops')
    expect(readFavouriteProjectIds()).toEqual([])
    window.localStorage.setItem(FAVOURITE_PROJECTS_KEY, '{"a":1}')
    expect(readFavouriteProjectIds()).toEqual([])
  })

  it('toggles an id on and off without mutating the input', () => {
    const start = ['a']
    expect(toggleFavouriteId(start, 'b')).toEqual(['a', 'b'])
    expect(toggleFavouriteId(start, 'a')).toEqual([])
    expect(start).toEqual(['a'])
  })
})
